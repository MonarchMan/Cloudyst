package workflows

import (
	pb "api/api/file/common/v1"
	pbexplorer "api/api/file/workflow/v1"
	"api/external/trans"
	"archive/zip"
	"common/hashid"
	"common/util"
	"context"
	"encoding/json"
	"file/ent"
	"file/ent/task"
	"file/internal/biz/cluster"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/manager"
	"file/internal/biz/filemanager/manager/entitysource"
	"file/internal/biz/queue"
	"file/internal/data/types"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/gofrs/uuid"
)

type (
	CreateArchiveTask struct {
		*queue.DBTask

		l        *log.Helper
		state    *CreateArchiveTaskState
		progress *pbexplorer.TaskPhaseProgressResponse
		node     cluster.Node
	}

	CreateArchiveTaskPhase string

	CreateArchiveTaskState struct {
		Uris               []string                     `json:"uris,omitempty"`
		Dst                string                       `json:"dst,omitempty"`
		TempPath           string                       `json:"temp_path,omitempty"`
		ArchiveFile        string                       `json:"archive_file,omitempty"`
		Phase              CreateArchiveTaskPhase       `json:"phase,omitempty"`
		SlaveUploadTaskID  int                          `json:"slave__upload_task_id,omitempty"`
		SlaveArchiveTaskID int                          `json:"slave__archive_task_id,omitempty"`
		SlaveCompressState *SlaveCreateArchiveTaskState `json:"slave_compress_state,omitempty"`
		Failed             int                          `json:"failed,omitempty"`
		NodeState          `json:",inline"`
	}
)

const (
	CreateArchiveTaskPhaseNotStarted    CreateArchiveTaskPhase = "not_started"
	CreateArchiveTaskPhaseCompressFiles CreateArchiveTaskPhase = "compress_files"
	CreateArchiveTaskPhaseUploadArchive CreateArchiveTaskPhase = "upload_archive"

	CreateArchiveTaskPhaseAwaitSlaveCompressing        CreateArchiveTaskPhase = "await_slave_compressing"
	CreateArchiveTaskPhaseCreateAndAwaitSlaveUploading CreateArchiveTaskPhase = "await_slave_uploading"
	CreateArchiveTaskPhaseCompleteUpload               CreateArchiveTaskPhase = "complete_upload"

	ProgressTypeArchiveCount = "archive_count"
	ProgressTypeArchiveSize  = "archive_size"
	ProgressTypeUpload       = "upload"
	ProgressTypeUploadCount  = "upload_count"
)

func init() {
	queue.RegisterResumableTaskFactory(queue.CreateArchiveTaskType, NewCreateArchiveTaskFromModel)
}

// NewCreateArchiveTask creates a new CreateArchiveTask
func NewCreateArchiveTask(ctx context.Context, src []string, dst string) (queue.Task, error) {
	user := trans.FromContext(ctx)
	state := &CreateArchiveTaskState{
		Uris:      src,
		Dst:       dst,
		NodeState: NodeState{},
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}

	t := &CreateArchiveTask{
		DBTask: &queue.DBTask{
			Task: &ent.Task{
				Type:         queue.CreateArchiveTaskType,
				TraceID:      util.TraceID(ctx),
				PrivateState: string(stateBytes),
				PublicState:  &pb.TaskPublicState{},
			},
			DirectOwner: user,
		},
	}
	return t, nil
}

func NewCreateArchiveTaskFromModel(task *ent.Task) queue.Task {
	return &CreateArchiveTask{
		DBTask: &queue.DBTask{
			Task: task,
		},
	}
}

func (m *CreateArchiveTask) Do(ctx context.Context) (task.Status, error) {
	managerDep := filemanager.ManagerDepFromContext(ctx)
	dbfsDep := filemanager.DBFSDepFromContext(ctx)
	nodePool := filemanager.NodePoolFromContext(ctx)
	m.l = managerDep.Logger()

	m.Lock()
	if m.progress == nil {
		m.progress = &pbexplorer.TaskPhaseProgressResponse{
			ProgressMap: make(map[string]*pbexplorer.Progress),
		}
	}
	m.Unlock()

	// unmarshal state
	state := &CreateArchiveTaskState{}
	if err := json.Unmarshal([]byte(m.State()), state); err != nil {
		return task.StatusError, fmt.Errorf("failed to unmarshal state: %w", err)
	}
	m.state = state

	// select node
	node, err := allocateNode(ctx, nodePool, &m.state.NodeState, types.NodeCapabilityCreateArchive)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to allocate node: %w", err)
	}
	m.node = node

	next := task.StatusCompleted

	if m.node.IsMaster() {
		// Initialize temp folder
		// Compress files
		// Upload files to dst
		switch m.state.Phase {
		case CreateArchiveTaskPhaseNotStarted, "":
			next, err = m.initializeTempFolder(ctx, managerDep)
		case CreateArchiveTaskPhaseCompressFiles:
			next, err = m.createArchiveFile(ctx, managerDep, dbfsDep)
		case CreateArchiveTaskPhaseUploadArchive:
			next, err = m.uploadArchive(ctx, managerDep, dbfsDep)
		default:
			next, err = task.StatusError, fmt.Errorf("unknown phase %q: %w", m.state.Phase, queue.CriticalErr)
		}
	} else {
		// Listing all files and send to slave node for compressing
		// Await compressing and send to slave for uploading
		// Await uploading and complete upload
		switch m.state.Phase {
		case CreateArchiveTaskPhaseNotStarted, "":
			next, err = m.listEntitiesAndSendToSlave(ctx, managerDep, dbfsDep)
		case CreateArchiveTaskPhaseAwaitSlaveCompressing:
			next, err = m.awaitSlaveCompressing(ctx)
		case CreateArchiveTaskPhaseCreateAndAwaitSlaveUploading:
			next, err = m.createAndAwaitSlaveUploading(ctx, managerDep)
		case CreateArchiveTaskPhaseCompleteUpload:
			next, err = m.completeUpload(ctx)
		default:
			next, err = task.StatusError, fmt.Errorf("unknown phase %q: %w", m.state.Phase, queue.CriticalErr)
		}
	}

	newStateStr, marshalErr := json.Marshal(m.state)
	if marshalErr != nil {
		return task.StatusError, fmt.Errorf("failed to marshal state: %w", marshalErr)
	}

	m.Lock()
	m.Task.PrivateState = string(newStateStr)
	m.Unlock()
	return next, err
}

func (m *CreateArchiveTask) Cleanup(ctx context.Context) error {
	if m.state.SlaveCompressState != nil && m.state.SlaveCompressState.TempPath != "" && m.node != nil {
		if err := m.node.CleanupFolders(context.Background(), m.state.SlaveCompressState.TempPath); err != nil {
			m.l.WithContext(ctx).Warnf("Failed to cleanup slave temp folder %s: %s", m.state.SlaveCompressState.TempPath, err)
		}
	}

	if m.state.TempPath != "" {
		time.Sleep(time.Duration(1) * time.Second)
		return os.RemoveAll(m.state.TempPath)
	}

	return nil
}

func (m *CreateArchiveTask) initializeTempFolder(ctx context.Context, dep filemanager.ManagerDep) (task.Status, error) {
	tempPath, err := prepareTempFolder(ctx, dep, m)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to prepare temp folder: %w", err)
	}

	m.state.TempPath = tempPath
	m.state.Phase = CreateArchiveTaskPhaseCompressFiles
	m.ResumeAfter(0)
	return task.StatusSuspending, nil
}

func (m *CreateArchiveTask) listEntitiesAndSendToSlave(ctx context.Context, managerDep filemanager.ManagerDep,
	dbfsDep filemanager.DbfsDep) (task.Status, error) {
	uris, err := fs.NewUriFromStrings(m.state.Uris...)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to create uri from strings: %s (%w)", err, queue.CriticalErr)
	}

	payload := &SlaveCreateArchiveTaskState{
		Entities: make([]SlaveCreateArchiveEntity, 0, len(uris)),
		Policies: make(map[int]*ent.StoragePolicy),
	}

	user := trans.FromContext(ctx)
	fm := manager.NewFileManager(managerDep, dbfsDep, user)
	storagePolicyClient := managerDep.PolicyClient()

	failed, err := fm.CreateArchive(ctx, uris, io.Discard,
		fs.WithDryRun(func(name string, e fs.Entity) {
			payload.Entities = append(payload.Entities, SlaveCreateArchiveEntity{
				Entity: e.Model(),
				Path:   name,
			})
			if _, ok := payload.Policies[e.PolicyID()]; !ok {
				policy, err := storagePolicyClient.GetPolicyByID(ctx, e.PolicyID())
				if err != nil {
					m.l.WithContext(ctx).Warnf("Failed to get policy %d: %s", e.PolicyID(), err)
				} else {
					payload.Policies[e.PolicyID()] = policy
				}
			}
		}),
		//fs.WithMaxArchiveSize(users.Edges.Group.Settings.CompressSize),
	)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to compress files: %w", err)
	}

	m.state.Failed = failed
	payloadStr, err := json.Marshal(payload)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to marshal payload: %w", err)
	}

	taskId, err := m.node.CreateTask(ctx, queue.SlaveCreateArchiveTaskType, string(payloadStr))
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to create slave task: %w", err)
	}

	m.state.Phase = CreateArchiveTaskPhaseAwaitSlaveCompressing
	m.state.SlaveArchiveTaskID = taskId
	m.ResumeAfter((10 * time.Second))
	return task.StatusSuspending, nil
}

func (m *CreateArchiveTask) awaitSlaveCompressing(ctx context.Context) (task.Status, error) {
	t, err := m.node.GetTask(ctx, m.state.SlaveArchiveTaskID, false)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to get slave task: %w", err)
	}

	m.Lock()
	m.state.NodeState.progress = t.Progress
	m.Unlock()

	m.state.SlaveCompressState = &SlaveCreateArchiveTaskState{}
	if err := json.Unmarshal([]byte(t.PrivateState), m.state.SlaveCompressState); err != nil {
		return task.StatusError, fmt.Errorf("failed to unmarshal slave compress state: %s (%w)", err, queue.CriticalErr)
	}

	if t.Status == task.StatusError {
		return task.StatusError, fmt.Errorf("slave task failed: %s (%w)", t.Error, queue.CriticalErr)
	}

	if t.Status == task.StatusCanceled {
		return task.StatusError, fmt.Errorf("slave task canceled (%w)", queue.CriticalErr)
	}

	if t.Status == task.StatusCompleted {
		m.state.Phase = CreateArchiveTaskPhaseCreateAndAwaitSlaveUploading
		m.ResumeAfter(0)
		return task.StatusSuspending, nil
	}

	m.l.WithContext(ctx).Infof("Slave task %d is still compressing, resume after 30s.", m.state.SlaveArchiveTaskID)
	m.ResumeAfter((time.Second * 30))
	return task.StatusSuspending, nil
}

func (m *CreateArchiveTask) createAndAwaitSlaveUploading(ctx context.Context, dep filemanager.ManagerDep) (task.Status, error) {
	user := trans.FromContext(ctx)

	if m.state.SlaveUploadTaskID == 0 {
		dst, err := fs.NewUriFromString(m.state.Dst)
		if err != nil {
			return task.StatusError, fmt.Errorf("failed to parse dst uri %q: %s (%w)", m.state.Dst, err, queue.CriticalErr)
		}

		// Create slave upload task
		payload := &SlaveUploadTaskState{
			Files: []SlaveUploadEntity{
				{
					Size: m.state.SlaveCompressState.CompressedSize,
					Uri:  dst,
					Src:  m.state.SlaveCompressState.ZipFilePath,
				},
			},
			MaxParallel: dep.SettingProvider().MaxParallelTransfer(ctx),
			UserID:      int(user.Id),
		}

		payloadStr, err := json.Marshal(payload)
		if err != nil {
			return task.StatusError, fmt.Errorf("failed to marshal payload: %w", err)
		}

		taskId, err := m.node.CreateTask(ctx, queue.SlaveUploadTaskType, string(payloadStr))
		if err != nil {
			return task.StatusError, fmt.Errorf("failed to create slave task: %w", err)
		}

		m.state.NodeState.progress = nil
		m.state.SlaveUploadTaskID = taskId
		m.ResumeAfter(0)
		return task.StatusSuspending, nil
	}

	m.l.WithContext(ctx).Infof("Checking slave upload task %d...", m.state.SlaveUploadTaskID)
	t, err := m.node.GetTask(ctx, m.state.SlaveUploadTaskID, true)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to get slave task: %w", err)
	}

	m.Lock()
	m.state.NodeState.progress = t.Progress
	m.Unlock()

	if t.Status == task.StatusError {
		return task.StatusError, fmt.Errorf("slave task failed: %s (%w)", t.Error, queue.CriticalErr)
	}

	if t.Status == task.StatusCanceled {
		return task.StatusError, fmt.Errorf("slave task canceled (%w)", queue.CriticalErr)
	}

	if t.Status == task.StatusCompleted {
		m.state.Phase = CreateArchiveTaskPhaseCompleteUpload
		m.ResumeAfter(0)
		return task.StatusSuspending, nil
	}

	m.l.WithContext(ctx).Infof("Slave task %d is still uploading, resume after 30s.", m.state.SlaveUploadTaskID)
	m.ResumeAfter(time.Second * 30)
	return task.StatusSuspending, nil
}

func (m *CreateArchiveTask) completeUpload(ctx context.Context) (task.Status, error) {
	return task.StatusCompleted, nil
}

func (m *CreateArchiveTask) createArchiveFile(ctx context.Context, dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep) (task.Status, error) {
	uris, err := fs.NewUriFromStrings(m.state.Uris...)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to create uri from strings: %s (%w)", err, queue.CriticalErr)
	}

	user := trans.FromContext(ctx)
	fm := manager.NewFileManager(dep, dbfsDep, user)

	// Create temp zip files
	fileName := fmt.Sprintf("%s.zip", uuid.Must(uuid.NewV4()))
	zipFilePath := filepath.Join(
		m.state.TempPath,
		fileName,
	)
	zipFile, err := util.CreateNestedFile(zipFilePath)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to create zip files: %w", err)
	}

	defer zipFile.Close()

	// Start compressing
	m.Lock()
	m.progress.ProgressMap[ProgressTypeArchiveCount] = &pbexplorer.Progress{}
	m.progress.ProgressMap[ProgressTypeArchiveSize] = &pbexplorer.Progress{}
	m.Unlock()
	failed, err := fm.CreateArchive(ctx, uris, zipFile,
		fs.WithArchiveCompression(true),
		//fs.WithMaxArchiveSize(users.Edges.Group.Settings.CompressSize),
		fs.WithProgressFunc(func(current, diff int64, total int64) {
			atomic.AddInt64(&m.progress.ProgressMap[ProgressTypeArchiveSize].Current, diff)
			atomic.AddInt64(&m.progress.ProgressMap[ProgressTypeArchiveCount].Current, 1)
		}),
	)
	if err != nil {
		zipFile.Close()
		_ = os.Remove(zipFilePath)
		return task.StatusError, fmt.Errorf("failed to compress files: %w", err)
	}

	m.state.Failed = failed
	m.Lock()
	delete(m.progress.ProgressMap, ProgressTypeArchiveSize)
	delete(m.progress.ProgressMap, ProgressTypeArchiveCount)
	m.Unlock()

	m.state.Phase = CreateArchiveTaskPhaseUploadArchive
	m.state.ArchiveFile = fileName
	m.ResumeAfter(0)
	return task.StatusSuspending, nil
}

func (m *CreateArchiveTask) uploadArchive(ctx context.Context, dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep) (task.Status, error) {
	user := trans.FromContext(ctx)
	fm := manager.NewFileManager(dep, dbfsDep, user)
	zipFilePath := filepath.Join(
		m.state.TempPath,
		m.state.ArchiveFile,
	)

	m.l.WithContext(ctx).Infof("Uploading archive files %s to %s...", zipFilePath, m.state.Dst)

	uri, err := fs.NewUriFromString(m.state.Dst)
	if err != nil {
		return task.StatusError, fmt.Errorf(
			"failed to parse dst uri %q: %s (%w)",
			m.state.Dst,
			err,
			queue.CriticalErr,
		)
	}

	file, err := os.Open(zipFilePath)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to open compressed archive %q: %s", m.state.ArchiveFile, err)
	}
	defer file.Close()
	fi, err := file.Stat()
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to get files info: %w", err)
	}
	size := fi.Size()

	m.Lock()
	m.progress.ProgressMap[ProgressTypeUpload] = &pbexplorer.Progress{}
	m.Unlock()
	fileData := &fs.UploadRequest{
		Props: &fs.UploadProps{
			Uri:  uri,
			Size: size,
		},
		ProgressFunc: func(current, diff int64, total int64) {
			atomic.StoreInt64(&m.progress.ProgressMap[ProgressTypeUpload].Current, current)
			atomic.StoreInt64(&m.progress.ProgressMap[ProgressTypeUpload].Total, total)
		},
		File:   file,
		Seeker: file,
	}

	_, err = fm.Update(ctx, fileData)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to upload archive files: %w", err)
	}

	return task.StatusCompleted, nil
}

func (m *CreateArchiveTask) Progress(ctx context.Context) *pbexplorer.TaskPhaseProgressResponse {
	m.Lock()
	defer m.Unlock()

	if m.state.NodeState.progress != nil {
		merged := make(map[string]*pbexplorer.Progress)
		for k, v := range m.progress.ProgressMap {
			merged[k] = v
		}

		for k, v := range m.state.NodeState.progress.ProgressMap {
			merged[k] = v
		}

		m.progress.ProgressMap = merged
	}
	return m.progress
}

func (m *CreateArchiveTask) Summarize(hasher hashid.Encoder) *pbexplorer.Summary {
	// unmarshal state
	if m.state == nil {
		if err := json.Unmarshal([]byte(m.State()), &m.state); err != nil {
			return nil
		}
	}

	failed := m.state.Failed
	if m.state.SlaveCompressState != nil {
		failed = m.state.SlaveCompressState.Failed
	}

	return &pbexplorer.Summary{
		NodeId: int32(m.state.NodeID),
		Phase:  string(m.state.Phase),
		Props: map[string]string{
			SummaryKeySrcMultiple: strings.Join(m.state.Uris, ","),
			SummaryKeyDst:         m.state.Dst,
			SummaryKeyFailed:      strconv.Itoa(failed),
		},
	}
}

type (
	SlaveCreateArchiveEntity struct {
		Entity *ent.Entity `json:"entity"`
		Path   string      `json:"path"`
	}
	SlaveCreateArchiveTaskState struct {
		Entities       []SlaveCreateArchiveEntity `json:"entities"`
		Policies       map[int]*ent.StoragePolicy `json:"policies"`
		CompressedSize int64                      `json:"compressed_size"`
		TempPath       string                     `json:"temp_path"`
		ZipFilePath    string                     `json:"zip_file_path"`
		Failed         int                        `json:"failed"`
	}
	SlaveCreateArchiveTask struct {
		*queue.InMemoryTask

		mu       sync.RWMutex
		progress *pbexplorer.TaskPhaseProgressResponse
		l        *log.Helper
		state    *SlaveCreateArchiveTaskState
	}
)

// NewSlaveCreateArchiveTask creates a new SlaveCreateArchiveTask from raw private state
func NewSlaveCreateArchiveTask(ctx context.Context, props *pb.SlaveTaskProps, id int, state string) queue.Task {
	return &SlaveCreateArchiveTask{
		InMemoryTask: &queue.InMemoryTask{
			DBTask: &queue.DBTask{
				Task: &ent.Task{
					ID:      id,
					TraceID: util.TraceID(ctx),
					PublicState: &pb.TaskPublicState{
						SlaveTaskProps: props,
					},
					PrivateState: state,
				},
			},
		},

		progress: &pbexplorer.TaskPhaseProgressResponse{
			ProgressMap: make(map[string]*pbexplorer.Progress),
		},
	}
}

func (t *SlaveCreateArchiveTask) Do(ctx context.Context) (task.Status, error) {
	ctx = prepareSlaveTaskCtx(ctx, t.Model().PublicState.SlaveTaskProps)
	managerDep := filemanager.ManagerDepFromContext(ctx)
	dbfsDep := filemanager.DBFSDepFromContext(ctx)
	t.l = managerDep.Logger()
	fm := manager.NewFileManager(managerDep, dbfsDep, nil)

	// unmarshal state
	state := &SlaveCreateArchiveTaskState{}
	if err := json.Unmarshal([]byte(t.State()), state); err != nil {
		return task.StatusError, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	t.state = state

	totalFiles := int64(0)
	totalFileSize := int64(0)
	for _, e := range t.state.Entities {
		totalFiles++
		totalFileSize += e.Entity.Size
	}

	t.Lock()
	t.progress.ProgressMap[ProgressTypeArchiveCount] = &pbexplorer.Progress{Total: totalFiles}
	t.progress.ProgressMap[ProgressTypeArchiveSize] = &pbexplorer.Progress{Total: totalFileSize}
	t.Unlock()

	// 3. Create temp workspace
	tempPath, err := prepareTempFolder(ctx, managerDep, t)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to prepare temp folder: %w", err)
	}
	t.state.TempPath = tempPath

	// 2. Create archive files
	fileName := fmt.Sprintf("%s.zip", uuid.Must(uuid.NewV4()))
	zipFilePath := filepath.Join(
		t.state.TempPath,
		fileName,
	)
	zipFile, err := util.CreateNestedFile(zipFilePath)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to create zip files: %w", err)
	}

	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// 3. Download each entity and write into zip files
	for _, e := range t.state.Entities {
		policy, ok := t.state.Policies[e.Entity.StoragePolicyEntities]
		if !ok {
			state.Failed++
			t.l.WithContext(ctx).Warnf("Policy not found for entity %d, skipping...", e.Entity.ID)
			continue
		}

		entity := fs.NewEntity(e.Entity)
		es, err := fm.GetEntitySource(ctx, 0,
			fs.WithEntity(entity),
			fs.WithPolicy(fm.CastStoragePolicyOnSlave(ctx, policy)),
		)
		if err != nil {
			state.Failed++
			t.l.WithContext(ctx).Warnf("Failed to get entity source for entity %d: %s, skipping...", e.Entity.ID, err)
			continue
		}

		// Write to zip files
		header := &zip.FileHeader{
			Name:               e.Path,
			Modified:           entity.UpdatedAt(),
			UncompressedSize64: uint64(entity.Size()),
			Method:             zip.Deflate,
		}

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			es.Close()
			state.Failed++
			t.l.WithContext(ctx).Warnf("Failed to create zip header for %s: %s, skipping...", e.Path, err)
			continue
		}

		es.Apply(entitysource.WithContext(ctx))
		_, err = io.Copy(writer, es)
		es.Close()
		if err != nil {
			state.Failed++
			t.l.WithContext(ctx).Warnf("Failed to write entity %d to zip files: %s, skipping...", e.Entity.ID, err)
		}

		atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeArchiveSize].Current, entity.Size())
		atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeArchiveCount].Current, 1)
	}

	zipWriter.Close()
	stat, err := zipFile.Stat()
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to get compressed files info: %w", err)
	}

	t.state.CompressedSize = stat.Size()
	t.state.ZipFilePath = zipFilePath
	// Clear unused fields to save space
	t.state.Entities = nil
	t.state.Policies = nil

	newStateStr, marshalErr := json.Marshal(t.state)
	if marshalErr != nil {
		return task.StatusError, fmt.Errorf("failed to marshal state: %w", marshalErr)
	}

	t.Lock()
	t.Task.PrivateState = string(newStateStr)
	t.Unlock()
	return task.StatusCompleted, nil
}

func (m *SlaveCreateArchiveTask) Progress(ctx context.Context) *pbexplorer.TaskPhaseProgressResponse {
	m.Lock()
	defer m.Unlock()

	return m.progress
}
