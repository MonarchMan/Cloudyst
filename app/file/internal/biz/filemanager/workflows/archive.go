package workflows

import (
	"api/external/trans"
	"archive/zip"
	"common/hashid"
	"common/util"
	"context"
	"encoding/json"
	"file/ent"
	"file/internal/biz/cluster"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/encrypt"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/manager"
	"file/internal/biz/filemanager/manager/entitysource"
	"file/internal/biz/queue"
	"file/internal/data"
	"file/internal/data/types"
	"fmt"
	"io"
	"os"
	"path/filepath"
	mqueue "queue"
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
		progress mqueue.Progresses
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
	mqueue.RegisterResumableTaskFactory(queue.CreateArchiveTaskType, NewCreateArchiveTaskFromModel)
}

// NewCreateArchiveTask creates a new CreateArchiveTask
func NewCreateArchiveTask(ctx context.Context, src []string, dst string) (mqueue.Task, error) {
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
				PublicState:  &mqueue.TaskPublicState{},
			},
			DirectOwner: user,
		},
	}
	return t, nil
}

func NewCreateArchiveTaskFromModel(task mqueue.TaskRecord) mqueue.Task {
	wrapped, ok := task.(*data.TaskModel)
	if !ok {
		return nil
	}
	return &CreateArchiveTask{
		DBTask: &queue.DBTask{
			Task: wrapped.Task,
		},
	}
}

func (m *CreateArchiveTask) Do(ctx context.Context) (mqueue.TaskStatus, error) {
	managerDep := filemanager.ManagerDepFromContext(ctx)
	dbfsDep := filemanager.DBFSDepFromContext(ctx)
	nodePool := cluster.NodePoolFromContext(ctx)
	m.l = managerDep.Logger()

	m.Lock()
	if m.progress == nil {
		m.progress = make(mqueue.Progresses)
	}
	m.Unlock()

	// unmarshal state
	state := &CreateArchiveTaskState{}
	if err := json.Unmarshal([]byte(m.State()), state); err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to unmarshal state: %w", err)
	}
	m.state = state

	// select node
	node, err := allocateNode(ctx, nodePool, &m.state.NodeState, types.NodeCapabilityCreateArchive)
	if err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to allocate node: %w", err)
	}
	m.node = node

	next := mqueue.StatusCompleted

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
			next, err = mqueue.StatusError, fmt.Errorf("unknown phase %q: %w", m.state.Phase, queue.CriticalErr)
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
			next, err = mqueue.StatusError, fmt.Errorf("unknown phase %q: %w", m.state.Phase, queue.CriticalErr)
		}
	}

	newStateStr, marshalErr := json.Marshal(m.state)
	if marshalErr != nil {
		return mqueue.StatusError, fmt.Errorf("failed to marshal state: %w", marshalErr)
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

func (m *CreateArchiveTask) initializeTempFolder(ctx context.Context, dep filemanager.ManagerDep) (mqueue.TaskStatus, error) {
	tempPath, err := prepareTempFolder(ctx, dep, m)
	if err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to prepare temp folder: %w", err)
	}

	m.state.TempPath = tempPath
	m.state.Phase = CreateArchiveTaskPhaseCompressFiles
	m.ResumeAfter(0)
	return mqueue.StatusSuspending, nil
}

func (m *CreateArchiveTask) listEntitiesAndSendToSlave(ctx context.Context, managerDep filemanager.ManagerDep,
	dbfsDep filemanager.DbfsDep) (mqueue.TaskStatus, error) {
	uris, err := fs.NewUriFromStrings(m.state.Uris...)
	if err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to create uri from strings: %s (%w)", err, queue.CriticalErr)
	}

	payload := &SlaveCreateArchiveTaskState{
		Entities: make([]SlaveCreateArchiveEntity, 0, len(uris)),
		Policies: make(map[int]*ent.StoragePolicy),
	}

	user := m.Owner()
	fm := manager.NewFileManager(managerDep, dbfsDep, user)
	storagePolicyClient := managerDep.PolicyClient()
	masterKey, _ := managerDep.MasterEncryptKeyVault().GetMasterKey(ctx)

	failed, err := fm.CreateArchive(ctx, uris, io.Discard,
		fs.WithDryRun(func(name string, e fs.Entity) {
			entityModel, err := decryptEntityKeyIfNeeded(masterKey, e.Model())
			if err != nil {
				m.l.Warnf("Failed to decrypt entity key for %q: %s", name, err)
				return
			}

			payload.Entities = append(payload.Entities, SlaveCreateArchiveEntity{
				Entity: entityModel,
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
		return mqueue.StatusError, fmt.Errorf("failed to compress files: %w", err)
	}

	m.state.Failed = failed
	payloadStr, err := json.Marshal(payload)
	if err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to marshal payload: %w", err)
	}

	taskId, err := m.node.CreateTask(ctx, queue.SlaveCreateArchiveTaskType, string(payloadStr))
	if err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to create slave task: %w", err)
	}

	m.state.Phase = CreateArchiveTaskPhaseAwaitSlaveCompressing
	m.state.SlaveArchiveTaskID = taskId
	m.ResumeAfter((10 * time.Second))
	return mqueue.StatusSuspending, nil
}

func (m *CreateArchiveTask) awaitSlaveCompressing(ctx context.Context) (mqueue.TaskStatus, error) {
	t, err := m.node.GetTask(ctx, m.state.SlaveArchiveTaskID, false)
	if err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to get slave task: %w", err)
	}

	m.Lock()
	m.state.NodeState.progress = t.Progress
	m.Unlock()

	m.state.SlaveCompressState = &SlaveCreateArchiveTaskState{}
	if err := json.Unmarshal([]byte(t.PrivateState), m.state.SlaveCompressState); err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to unmarshal slave compress state: %s (%w)", err, queue.CriticalErr)
	}

	if t.Status == mqueue.StatusError {
		return mqueue.StatusError, fmt.Errorf("slave task failed: %s (%w)", t.Error, queue.CriticalErr)
	}

	if t.Status == mqueue.StatusCanceled {
		return mqueue.StatusError, fmt.Errorf("slave task canceled (%w)", queue.CriticalErr)
	}

	if t.Status == mqueue.StatusCompleted {
		m.state.Phase = CreateArchiveTaskPhaseCreateAndAwaitSlaveUploading
		m.ResumeAfter(0)
		return mqueue.StatusSuspending, nil
	}

	m.l.WithContext(ctx).Infof("Slave task %d is still compressing, resume after 30s.", m.state.SlaveArchiveTaskID)
	m.ResumeAfter((time.Second * 30))
	return mqueue.StatusSuspending, nil
}

func (m *CreateArchiveTask) createAndAwaitSlaveUploading(ctx context.Context, dep filemanager.ManagerDep) (mqueue.TaskStatus, error) {
	if m.state.SlaveUploadTaskID == 0 {
		dst, err := fs.NewUriFromString(m.state.Dst)
		if err != nil {
			return mqueue.StatusError, fmt.Errorf("failed to parse dst uri %q: %s (%w)", m.state.Dst, err, queue.CriticalErr)
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
			UserID:      m.OwnerID(),
		}

		payloadStr, err := json.Marshal(payload)
		if err != nil {
			return mqueue.StatusError, fmt.Errorf("failed to marshal payload: %w", err)
		}

		taskId, err := m.node.CreateTask(ctx, queue.SlaveUploadTaskType, string(payloadStr))
		if err != nil {
			return mqueue.StatusError, fmt.Errorf("failed to create slave task: %w", err)
		}

		m.state.NodeState.progress = nil
		m.state.SlaveUploadTaskID = taskId
		m.ResumeAfter(0)
		return mqueue.StatusSuspending, nil
	}

	m.l.WithContext(ctx).Infof("Checking slave upload task %d...", m.state.SlaveUploadTaskID)
	t, err := m.node.GetTask(ctx, m.state.SlaveUploadTaskID, true)
	if err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to get slave task: %w", err)
	}

	m.Lock()
	m.state.NodeState.progress = t.Progress
	m.Unlock()

	if t.Status == mqueue.StatusError {
		return mqueue.StatusError, fmt.Errorf("slave task failed: %s (%w)", t.Error, queue.CriticalErr)
	}

	if t.Status == mqueue.StatusCanceled {
		return mqueue.StatusError, fmt.Errorf("slave task canceled (%w)", queue.CriticalErr)
	}

	if t.Status == mqueue.StatusCompleted {
		m.state.Phase = CreateArchiveTaskPhaseCompleteUpload
		m.ResumeAfter(0)
		return mqueue.StatusSuspending, nil
	}

	m.l.WithContext(ctx).Infof("Slave task %d is still uploading, resume after 30s.", m.state.SlaveUploadTaskID)
	m.ResumeAfter(time.Second * 30)
	return mqueue.StatusSuspending, nil
}

func (m *CreateArchiveTask) completeUpload(ctx context.Context) (mqueue.TaskStatus, error) {
	return mqueue.StatusCompleted, nil
}

func (m *CreateArchiveTask) createArchiveFile(ctx context.Context, dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep) (mqueue.TaskStatus, error) {
	uris, err := fs.NewUriFromStrings(m.state.Uris...)
	if err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to create uri from strings: %s (%w)", err, queue.CriticalErr)
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
		return mqueue.StatusError, fmt.Errorf("failed to create zip files: %w", err)
	}

	defer zipFile.Close()

	// Start compressing
	m.Lock()
	m.progress[ProgressTypeArchiveCount] = &mqueue.Progress{}
	m.progress[ProgressTypeArchiveSize] = &mqueue.Progress{}
	m.Unlock()
	failed, err := fm.CreateArchive(ctx, uris, zipFile,
		fs.WithArchiveCompression(true),
		//fs.WithMaxArchiveSize(users.Edges.Group.Settings.CompressSize),
		fs.WithProgressFunc(func(current, diff int64, total int64) {
			atomic.AddInt64(&m.progress[ProgressTypeArchiveSize].Current, diff)
			atomic.AddInt64(&m.progress[ProgressTypeArchiveCount].Current, 1)
		}),
	)
	if err != nil {
		zipFile.Close()
		_ = os.Remove(zipFilePath)
		return mqueue.StatusError, fmt.Errorf("failed to compress files: %w", err)
	}

	m.state.Failed = failed
	m.Lock()
	delete(m.progress, ProgressTypeArchiveSize)
	delete(m.progress, ProgressTypeArchiveCount)
	m.Unlock()

	m.state.Phase = CreateArchiveTaskPhaseUploadArchive
	m.state.ArchiveFile = fileName
	m.ResumeAfter(0)
	return mqueue.StatusSuspending, nil
}

func (m *CreateArchiveTask) uploadArchive(ctx context.Context, dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep) (mqueue.TaskStatus, error) {
	user := trans.FromContext(ctx)
	fm := manager.NewFileManager(dep, dbfsDep, user)
	zipFilePath := filepath.Join(
		m.state.TempPath,
		m.state.ArchiveFile,
	)

	m.l.WithContext(ctx).Infof("Uploading archive files %s to %s...", zipFilePath, m.state.Dst)

	uri, err := fs.NewUriFromString(m.state.Dst)
	if err != nil {
		return mqueue.StatusError, fmt.Errorf(
			"failed to parse dst uri %q: %s (%w)",
			m.state.Dst,
			err,
			queue.CriticalErr,
		)
	}

	file, err := os.Open(zipFilePath)
	if err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to open compressed archive %q: %s", m.state.ArchiveFile, err)
	}
	defer file.Close()
	fi, err := file.Stat()
	if err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to get files info: %w", err)
	}
	size := fi.Size()

	m.Lock()
	m.progress[ProgressTypeUpload] = &mqueue.Progress{}
	m.Unlock()
	fileData := &fs.UploadRequest{
		Props: &fs.UploadProps{
			Uri:  uri,
			Size: size,
		},
		ProgressFunc: func(current, diff int64, total int64) {
			atomic.StoreInt64(&m.progress[ProgressTypeUpload].Current, current)
			atomic.StoreInt64(&m.progress[ProgressTypeUpload].Total, total)
		},
		File:   file,
		Seeker: file,
	}

	_, err = fm.Update(ctx, fileData)
	if err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to upload archive files: %w", err)
	}

	return mqueue.StatusCompleted, nil
}

func (m *CreateArchiveTask) Progress(ctx context.Context) mqueue.Progresses {
	m.Lock()
	defer m.Unlock()

	if m.state.NodeState.progress != nil {
		merged := make(mqueue.Progresses)
		for k, v := range m.progress {
			merged[k] = v
		}

		for k, v := range m.state.NodeState.progress {
			merged[k] = v
		}

		m.progress = merged
	}
	return m.progress
}

func (m *CreateArchiveTask) Summarize(hasher hashid.Encoder) *mqueue.Summary {
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

	return &mqueue.Summary{
		NodeID: m.state.NodeID,
		Phase:  string(m.state.Phase),
		Props: map[string]any{
			SummaryKeySrcMultiple: m.state.Uris,
			SummaryKeyDst:         m.state.Dst,
			SummaryKeyFailed:      failed,
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
		*mqueue.InMemoryTask

		mu       sync.RWMutex
		progress mqueue.Progresses
		l        *log.Helper
		state    *SlaveCreateArchiveTaskState
		props    *types.SlaveTaskProps
	}
)

// NewSlaveCreateArchiveTask creates a new SlaveCreateArchiveTask from raw private state
func NewSlaveCreateArchiveTask(ctx context.Context, props *types.SlaveTaskProps, id int, state string) mqueue.Task {
	return &SlaveCreateArchiveTask{
		InMemoryTask: &mqueue.InMemoryTask{
			TaskModel: &mqueue.TaskModel{
				ModelID:           id,
				ModelTraceID:      util.TraceID(ctx),
				ModelPrivateState: state,
			},
		},
		props: props,

		progress: make(mqueue.Progresses),
	}
}

func (t *SlaveCreateArchiveTask) Do(ctx context.Context) (mqueue.TaskStatus, error) {
	ctx = prepareSlaveTaskCtx(ctx, t.props)
	managerDep := filemanager.ManagerDepFromContext(ctx)
	dbfsDep := filemanager.DBFSDepFromContext(ctx)
	t.l = managerDep.Logger()
	fm := manager.NewFileManager(managerDep, dbfsDep, nil)

	// unmarshal state
	state := &SlaveCreateArchiveTaskState{}
	if err := json.Unmarshal([]byte(t.State()), state); err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	t.state = state

	totalFiles := int64(0)
	totalFileSize := int64(0)
	for _, e := range t.state.Entities {
		totalFiles++
		totalFileSize += e.Entity.Size
	}

	t.Lock()
	t.progress[ProgressTypeArchiveCount] = &mqueue.Progress{Total: totalFiles}
	t.progress[ProgressTypeArchiveSize] = &mqueue.Progress{Total: totalFileSize}
	t.Unlock()

	// 3. Create temp workspace
	tempPath, err := prepareTempFolder(ctx, managerDep, t)
	if err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to prepare temp folder: %w", err)
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
		return mqueue.StatusError, fmt.Errorf("failed to create zip files: %w", err)
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

		atomic.AddInt64(&t.progress[ProgressTypeArchiveSize].Current, entity.Size())
		atomic.AddInt64(&t.progress[ProgressTypeArchiveCount].Current, 1)
	}

	zipWriter.Close()
	stat, err := zipFile.Stat()
	if err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to get compressed files info: %w", err)
	}

	t.state.CompressedSize = stat.Size()
	t.state.ZipFilePath = zipFilePath
	// Clear unused fields to save space
	t.state.Entities = nil
	t.state.Policies = nil

	newStateStr, marshalErr := json.Marshal(t.state)
	if marshalErr != nil {
		return mqueue.StatusError, fmt.Errorf("failed to marshal state: %w", marshalErr)
	}

	t.Lock()
	t.ModelPrivateState = string(newStateStr)
	t.Unlock()
	return mqueue.StatusCompleted, nil
}

func (m *SlaveCreateArchiveTask) Progress(ctx context.Context) mqueue.Progresses {
	m.Lock()
	defer m.Unlock()

	return m.progress
}

func decryptEntityKeyIfNeeded(masterKey []byte, entity *ent.Entity) (*ent.Entity, error) {
	if entity.Props == nil || entity.Props.EncryptMetadata == nil {
		return entity, nil
	}

	decryptedKey, err := encrypt.DecryptWithMasterKey(masterKey, entity.Props.EncryptMetadata.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt entity key: %w", err)
	}

	entity.Props.EncryptMetadata.KeyPlainText = decryptedKey
	entity.Props.EncryptMetadata.Key = nil
	return entity, nil
}
