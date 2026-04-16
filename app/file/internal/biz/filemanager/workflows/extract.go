package workflows

import (
	pb "api/api/file/common/v1"
	pbexplorer "api/api/file/workflow/v1"
	"api/external/trans"
	"common/hashid"
	"common/util"
	"context"
	"encoding/json"
	"file/ent"
	"file/ent/task"
	"file/internal/biz/cluster"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/fs/dbfs"
	"file/internal/biz/filemanager/manager"
	"file/internal/biz/queue"
	"file/internal/data/types"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/gofrs/uuid"
	"github.com/mholt/archives"
)

type (
	ExtractArchiveTask struct {
		*queue.DBTask

		l        *log.Helper
		state    *ExtractArchiveTaskState
		progress *pbexplorer.TaskPhaseProgressResponse
		node     cluster.Node
	}
	ExtractArchiveTaskPhase string
	ExtractArchiveTaskState struct {
		Uri             string   `json:"uri,omitempty"`
		Encoding        string   `json:"encoding,omitempty"`
		Dst             string   `json:"dst,omitempty"`
		TempPath        string   `json:"temp_path,omitempty"`
		TempZipFilePath string   `json:"temp_zip_file_path,omitempty"`
		ProcessedCursor string   `json:"processed_cursor,omitempty"`
		SlaveTaskID     int      `json:"slave_task_id,omitempty"`
		Password        string   `json:"password,omitempty"`
		FileMask        []string `json:"file_mask,omitempty"`
		NodeState       `json:",inline"`
		Phase           ExtractArchiveTaskPhase `json:"phase,omitempty"`
	}
)

const (
	ExtractArchivePhaseNotStarted         ExtractArchiveTaskPhase = ""
	ExtractArchivePhaseDownloadZip        ExtractArchiveTaskPhase = "download_zip"
	ExtractArchivePhaseAwaitSlaveComplete ExtractArchiveTaskPhase = "await_slave_complete"

	ProgressTypeExtractCount = "extract_count"
	ProgressTypeExtractSize  = "extract_size"
	ProgressTypeDownload     = "download"

	SummaryKeySrc         = "src"
	SummaryKeySrcPhysical = "src_physical"
	SummaryKeyDst         = "dst"
)

func init() {
	queue.RegisterResumableTaskFactory(queue.ExtractArchiveTaskType, NewExtractArchiveTaskFromModel)
}

// NewExtractArchiveTask creates a new ExtractArchiveTask
func NewExtractArchiveTask(ctx context.Context, src, dst, encoding, password string, mask []string) (queue.Task, error) {
	state := &ExtractArchiveTaskState{
		Uri:       src,
		Dst:       dst,
		Encoding:  encoding,
		NodeState: NodeState{},
		Password:  password,
		FileMask:  mask,
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}

	user := trans.FromContext(ctx)
	t := &ExtractArchiveTask{
		DBTask: &queue.DBTask{
			Task: &ent.Task{
				Type:         queue.ExtractArchiveTaskType,
				TraceID:      util.TraceID(ctx),
				PrivateState: string(stateBytes),
				PublicState:  &pb.TaskPublicState{},
			},
			DirectOwner: user,
		},
	}
	return t, nil
}

func NewExtractArchiveTaskFromModel(task *ent.Task) queue.Task {
	return &ExtractArchiveTask{
		DBTask: &queue.DBTask{
			Task: task,
		},
	}
}

func (t *ExtractArchiveTask) Do(ctx context.Context) (task.Status, error) {
	dep := filemanager.ManagerDepFromContext(ctx)
	dbfsDep := filemanager.DBFSDepFromContext(ctx)
	np := filemanager.NodePoolFromContext(ctx)
	t.l = dep.Logger()

	t.Lock()
	if t.progress == nil {
		t.progress = &pbexplorer.TaskPhaseProgressResponse{
			ProgressMap: make(map[string]*pbexplorer.Progress),
		}
	}
	t.Unlock()

	// unmarshal state
	state := &ExtractArchiveTaskState{}
	if err := json.Unmarshal([]byte(t.State()), state); err != nil {
		return task.StatusError, fmt.Errorf("failed to unmarshal state: %w", err)
	}
	t.state = state

	// select node
	node, err := allocateNode(ctx, np, &t.state.NodeState, types.NodeCapabilityExtractArchive)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to allocate node: %w", err)
	}
	t.node = node

	next := task.StatusCompleted

	if node.IsMaster() {
		switch t.state.Phase {
		case ExtractArchivePhaseNotStarted:
			next, err = t.masterExtractArchive(ctx, dep, dbfsDep)
		case ExtractArchivePhaseDownloadZip:
			next, err = t.masterDownloadZip(ctx, dep, dbfsDep)
		default:
			next, err = task.StatusError, fmt.Errorf("unknown phase %q: %w", t.state.Phase, queue.CriticalErr)
		}
	} else {
		switch t.state.Phase {
		case ExtractArchivePhaseNotStarted:
			next, err = t.createSlaveExtractTask(ctx, dep, dbfsDep)
		case ExtractArchivePhaseAwaitSlaveComplete:
			next, err = t.awaitSlaveExtractComplete(ctx)
		default:
			next, err = task.StatusError, fmt.Errorf("unknown phase %q: %w", t.state.Phase, queue.CriticalErr)
		}
	}

	newStateStr, marshalErr := json.Marshal(t.state)
	if marshalErr != nil {
		return task.StatusError, fmt.Errorf("failed to marshal state: %w", marshalErr)
	}

	t.Lock()
	t.Task.PrivateState = string(newStateStr)
	t.Unlock()
	return next, err
}

func (t *ExtractArchiveTask) createSlaveExtractTask(ctx context.Context, dep filemanager.ManagerDep,
	dbfsDep filemanager.DbfsDep) (task.Status, error) {
	uri, err := fs.NewUriFromString(t.state.Uri)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to parse src uri: %s (%w)", err, queue.CriticalErr)
	}

	user := trans.FromContext(ctx)
	fm := manager.NewFileManager(dep, dbfsDep, user)

	// Get entity source to extract
	archiveFile, err := fm.Get(ctx, uri, dbfs.WithFileEntities(), dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityDownloadFile), dbfs.WithNotRoot())
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to get archive files: %s (%w)", err, queue.CriticalErr)
	}

	// Validate files size
	//if users.Edges.Group.Settings.DecompressSize > 0 && archiveFile.Size() > users.Edges.Group.Settings.DecompressSize {
	//	return task.StatusError,
	//		fmt.Errorf("files size %d exceeds the limit %d (%w)", archiveFile.Size(), users.Edges.Group.Settings.DecompressSize, queue.CriticalErr)
	//}

	// Create slave task
	storagePolicyClient := dep.PolicyClient()
	policy, err := storagePolicyClient.GetPolicyByID(ctx, archiveFile.PrimaryEntity().PolicyID())
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to get policy: %w", err)
	}

	payload := &SlaveExtractArchiveTaskState{
		FileName: archiveFile.DisplayName(),
		Entity:   archiveFile.PrimaryEntity().Model(),
		Policy:   policy,
		Encoding: t.state.Encoding,
		Dst:      t.state.Dst,
		UserID:   int(user.Id),
		Password: t.state.Password,
		FileMask: t.state.FileMask,
	}

	payloadStr, err := json.Marshal(payload)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to marshal payload: %w", err)
	}

	taskId, err := t.node.CreateTask(ctx, queue.SlaveExtractArchiveType, string(payloadStr))
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to create slave task: %w", err)
	}

	t.state.Phase = ExtractArchivePhaseAwaitSlaveComplete
	t.state.SlaveTaskID = taskId
	t.ResumeAfter(10 * time.Second)
	return task.StatusSuspending, nil
}

func (t *ExtractArchiveTask) awaitSlaveExtractComplete(ctx context.Context) (task.Status, error) {
	st, err := t.node.GetTask(ctx, t.state.SlaveTaskID, true)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to get slave task: %w", err)
	}

	t.Lock()
	t.state.NodeState.progress = st.Progress
	t.Unlock()

	if st.Status == task.StatusError {
		return task.StatusError, fmt.Errorf("slave task failed: %s (%w)", t.Error, queue.CriticalErr)
	}

	if st.Status == task.StatusCanceled {
		return task.StatusError, fmt.Errorf("slave task canceled (%w)", queue.CriticalErr)
	}

	if st.Status == task.StatusCompleted {
		return task.StatusCompleted, nil
	}

	t.l.WithContext(ctx).Infof("Slave task %d is still compressing, resume after 30s.", t.state.SlaveTaskID)
	t.ResumeAfter(time.Second * 30)
	return task.StatusSuspending, nil
}

func (t *ExtractArchiveTask) masterExtractArchive(ctx context.Context, dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep) (task.Status, error) {
	uri, err := fs.NewUriFromString(t.state.Uri)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to parse src uri: %s (%w)", err, queue.CriticalErr)
	}

	dst, err := fs.NewUriFromString(t.state.Dst)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to parse dst uri: %s (%w)", err, queue.CriticalErr)
	}

	user := trans.FromContext(ctx)
	fm := manager.NewFileManager(dep, dbfsDep, user)

	// Get entity source to extract
	archiveFile, err := fm.Get(ctx, uri, dbfs.WithFileEntities(), dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityDownloadFile), dbfs.WithNotRoot())
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to get archive files: %s (%w)", err, queue.CriticalErr)
	}

	// Validate files size
	//if users.Edges.Group.Settings.DecompressSize > 0 && archiveFile.Size() > users.Edges.Group.Settings.DecompressSize {
	//	return task.StatusError,
	//		fmt.Errorf("files size %d exceeds the limit %d (%w)", archiveFile.Size(), users.Edges.Group.Settings.DecompressSize, queue.CriticalErr)
	//}

	es, err := fm.GetEntitySource(ctx, 0, fs.WithEntity(archiveFile.PrimaryEntity()))
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to get entity source: %w", err)
	}

	defer es.Close()

	t.l.WithContext(ctx).Infof("Extracting archive %q to %q", uri, t.state.Dst)
	// Identify files format
	format, readStream, err := archives.Identify(ctx, archiveFile.DisplayName(), es)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to identify archive format: %w", err)
	}

	t.l.WithContext(ctx).Infof("Archive files %q format identified as %q", uri, format.Extension())

	extractor, ok := format.(archives.Extractor)
	if !ok {
		return task.StatusError, fmt.Errorf("format not an extractor %s", format.Extension())
	}

	formatExt := format.Extension()
	if formatExt == ".zip" || formatExt == ".7z" {
		// Zip/7Z extractor requires a Seeker+ReadAt
		if t.state.TempZipFilePath == "" && !es.IsLocal() {
			t.state.Phase = ExtractArchivePhaseDownloadZip
			t.ResumeAfter(0)
			return task.StatusSuspending, nil
		}

		if t.state.TempZipFilePath != "" {
			// Use temp zip files path
			zipFile, err := os.Open(t.state.TempZipFilePath)
			if err != nil {
				return task.StatusError, fmt.Errorf("failed to open temp zip files: %w", err)
			}

			defer zipFile.Close()
			readStream = zipFile
		}

		if es.IsLocal() {
			if _, err = es.Seek(0, 0); err != nil {
				return task.StatusError, fmt.Errorf("failed to seek entity source: %w", err)
			}

			readStream = es
		}
	}

	if zipExtractor, ok := extractor.(archives.Zip); ok {
		if t.state.Encoding != "" {
			t.l.WithContext(ctx).Infof("Using encoding %q for zip archive", t.state.Encoding)
			encoding, ok := manager.ZipEncodings[strings.ToLower(t.state.Encoding)]
			if !ok {
				t.l.WithContext(ctx).Warnf("Unknown encoding %q, fallback to default encoding", t.state.Encoding)
			} else {
				zipExtractor.TextEncoding = encoding
				extractor = zipExtractor
			}
		}
	} else if rarExtractor, ok := extractor.(archives.Rar); ok && t.state.Password != "" {
		rarExtractor.Password = t.state.Password
		extractor = rarExtractor
	} else if sevenZipExtractor, ok := extractor.(archives.SevenZip); ok && t.state.Password != "" {
		sevenZipExtractor.Password = t.state.Password
		extractor = sevenZipExtractor
	}

	needSkipToCursor := false
	if t.state.ProcessedCursor != "" {
		needSkipToCursor = true
	}
	t.Lock()
	t.progress.ProgressMap[ProgressTypeExtractCount] = &pbexplorer.Progress{}
	t.progress.ProgressMap[ProgressTypeExtractSize] = &pbexplorer.Progress{}
	t.Unlock()

	// extract and upload
	err = extractor.Extract(ctx, readStream, func(ctx context.Context, f archives.FileInfo) error {
		if needSkipToCursor && f.NameInArchive != t.state.ProcessedCursor {
			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractCount].Current, 1)
			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractSize].Current, f.Size())
			t.l.WithContext(ctx).Infof("File %q already processed, skipping...", f.NameInArchive)
			return nil
		}

		// Found cursor, start from cursor +1
		if t.state.ProcessedCursor == f.NameInArchive {
			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractCount].Current, 1)
			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractSize].Current, f.Size())
			needSkipToCursor = false
			return nil
		}

		rawPath := util.FormSlash(f.NameInArchive)
		savePath := dst.JoinRaw(rawPath)

		// If files mask is not empty, check if the path is in the mask
		if len(t.state.FileMask) > 0 && !isFileInMask(rawPath, t.state.FileMask) {
			t.l.WithContext(ctx).Warnf("File %q is not in the mask, skipping...", f.NameInArchive)
			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractCount].Current, 1)
			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractSize].Current, f.Size())
			return nil
		}

		// Check if path is legit
		if !strings.HasPrefix(savePath.Path(), util.FillSlash(path.Clean(dst.Path()))) {
			t.l.WithContext(ctx).Warnf("Path %q is not legit, skipping...", f.NameInArchive)
			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractCount].Current, 1)
			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractSize].Current, f.Size())
			return nil
		}

		if f.FileInfo.IsDir() {
			_, err := fm.Create(ctx, savePath, types.FileTypeFolder)
			if err != nil {
				t.l.WithContext(ctx).Warnf("Failed to create directory %q: %s, skipping...", rawPath, err)
			}

			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractCount].Current, 1)
			t.state.ProcessedCursor = f.NameInArchive
			return nil
		}

		fileStream, err := f.Open()
		if err != nil {
			t.l.WithContext(ctx).Warnf("Failed to open files %q in archive files: %s, skipping...", rawPath, err)
			return nil
		}

		fileData := &fs.UploadRequest{
			Props: &fs.UploadProps{
				Uri:  savePath,
				Size: f.Size(),
				LastModified: func() *time.Time {
					t := f.FileInfo.ModTime().Local()
					return &t
				}(),
			},
			ProgressFunc: func(current, diff int64, total int64) {
				atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractSize].Current, diff)
			},
			File: fileStream,
		}

		_, err = fm.Update(ctx, fileData, fs.WithNoEntityType())
		if err != nil {
			return fmt.Errorf("failed to upload files %q in archive files: %w", rawPath, err)
		}

		atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractCount].Current, 1)
		t.state.ProcessedCursor = f.NameInArchive
		return nil
	})

	if err != nil {
		return task.StatusError, fmt.Errorf("failed to extract archive: %w", err)
	}

	return task.StatusCompleted, nil
}

func (t *ExtractArchiveTask) masterDownloadZip(ctx context.Context, dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep) (task.Status, error) {
	uri, err := fs.NewUriFromString(t.state.Uri)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to parse src uri: %s (%w)", err, queue.CriticalErr)
	}

	user := trans.FromContext(ctx)
	fm := manager.NewFileManager(dep, dbfsDep, user)

	// Get entity source to extract
	archiveFile, err := fm.Get(ctx, uri, dbfs.WithFileEntities(), dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityDownloadFile), dbfs.WithNotRoot())
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to get archive files: %s (%w)", err, queue.CriticalErr)
	}

	es, err := fm.GetEntitySource(ctx, 0, fs.WithEntity(archiveFile.PrimaryEntity()))
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to get entity source: %w", err)
	}

	defer es.Close()

	// For non-local entity, we need to download the whole zip files first
	tempPath, err := prepareTempFolder(ctx, dep, t)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to prepare temp folder: %w", err)
	}
	t.state.TempPath = tempPath

	fileName := fmt.Sprintf("%s.zip", uuid.Must(uuid.NewV4()))
	zipFilePath := filepath.Join(
		t.state.TempPath,
		fileName,
	)

	zipFile, err := util.CreateNestedFile(zipFilePath)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to create zip files: %w", err)
	}

	t.Lock()
	t.progress.ProgressMap[ProgressTypeDownload] = &pbexplorer.Progress{Total: es.Entity().Size()}
	t.Unlock()

	defer zipFile.Close()
	if _, err := io.Copy(zipFile, util.NewCallbackReader(es, func(i int64) {
		atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeDownload].Current, i)
	})); err != nil {
		zipFile.Close()
		if err := os.Remove(zipFilePath); err != nil {
			t.l.WithContext(ctx).Warnf("Failed to remove temp zip files %q: %s", zipFilePath, err)
		}
		return task.StatusError, fmt.Errorf("failed to copy zip files to local temp: %w", err)
	}

	t.Lock()
	delete(t.progress.ProgressMap, ProgressTypeDownload)
	t.Unlock()
	t.state.TempZipFilePath = zipFilePath
	t.state.Phase = ExtractArchivePhaseNotStarted
	t.ResumeAfter(0)
	return task.StatusSuspending, nil
}

func (t *ExtractArchiveTask) Summarize(hasher hashid.Encoder) *pbexplorer.Summary {
	if t.state == nil {
		if err := json.Unmarshal([]byte(t.State()), &t.state); err != nil {
			return nil
		}
	}

	return &pbexplorer.Summary{
		NodeId: int32(t.state.NodeID),
		Phase:  string(t.state.Phase),
		Props: map[string]string{
			SummaryKeySrc: t.state.Uri,
			SummaryKeyDst: t.state.Dst,
		},
	}
}

func (t *ExtractArchiveTask) Progress(ctx context.Context) *pbexplorer.TaskPhaseProgressResponse {
	t.Lock()
	defer t.Unlock()

	if t.state.NodeState.progress != nil {
		merged := make(map[string]*pbexplorer.Progress)
		for k, v := range t.progress.ProgressMap {
			merged[k] = v
		}

		for k, v := range t.state.NodeState.progress.ProgressMap {
			merged[k] = v
		}

		t.progress.ProgressMap = merged
	}
	return t.progress
}

func (t *ExtractArchiveTask) Cleanup(ctx context.Context) error {
	if t.state.TempPath != "" {
		time.Sleep(time.Duration(1) * time.Second)
		return os.RemoveAll(t.state.TempPath)
	}

	return nil
}

type (
	SlaveExtractArchiveTask struct {
		*queue.InMemoryTask

		l        *log.Helper
		state    *SlaveExtractArchiveTaskState
		progress *pbexplorer.TaskPhaseProgressResponse
		node     cluster.Node
	}

	SlaveExtractArchiveTaskState struct {
		FileName        string             `json:"file_name"`
		Entity          *ent.Entity        `json:"entity"`
		Policy          *ent.StoragePolicy `json:"policy"`
		Encoding        string             `json:"encoding,omitempty"`
		Dst             string             `json:"dst,omitempty"`
		UserID          int                `json:"user_id"`
		TempPath        string             `json:"temp_path,omitempty"`
		TempZipFilePath string             `json:"temp_zip_file_path,omitempty"`
		ProcessedCursor string             `json:"processed_cursor,omitempty"`
		Password        string             `json:"password,omitempty"`
		FileMask        []string           `json:"file_mask,omitempty"`
	}
)

// NewSlaveExtractArchiveTask creates a new SlaveExtractArchiveTask from raw private state
func NewSlaveExtractArchiveTask(ctx context.Context, props *pb.SlaveTaskProps, id int, state string) queue.Task {
	return &SlaveExtractArchiveTask{
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

func (t *SlaveExtractArchiveTask) Do(ctx context.Context) (task.Status, error) {
	ctx = prepareSlaveTaskCtx(ctx, t.Model().PublicState.SlaveTaskProps)
	dep := filemanager.ManagerDepFromContext(ctx)
	dbfsDep := filemanager.DBFSDepFromContext(ctx)
	np := filemanager.NodePoolFromContext(ctx)
	t.l = dep.Logger()
	if np == nil {
		return task.StatusError, fmt.Errorf("failed to get node pool")
	}

	var err error
	t.node, err = np.Get(ctx, types.NodeCapabilityNone, 0)
	if err != nil || !t.node.IsMaster() {
		return task.StatusError, fmt.Errorf("failed to get master node: %w", err)
	}

	fm := manager.NewFileManager(dep, dbfsDep, nil)

	// unmarshal state
	state := &SlaveExtractArchiveTaskState{}
	if err := json.Unmarshal([]byte(t.State()), state); err != nil {
		return task.StatusError, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	t.state = state
	t.Lock()
	if t.progress == nil {
		t.progress = &pbexplorer.TaskPhaseProgressResponse{
			ProgressMap: make(map[string]*pbexplorer.Progress),
		}
	}
	t.progress.ProgressMap[ProgressTypeExtractCount] = &pbexplorer.Progress{}
	t.progress.ProgressMap[ProgressTypeExtractSize] = &pbexplorer.Progress{}
	t.Unlock()

	dst, err := fs.NewUriFromString(t.state.Dst)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to parse dst uri: %s (%w)", err, queue.CriticalErr)
	}

	// 1. Get entity source
	entity := fs.NewEntity(t.state.Entity)
	es, err := fm.GetEntitySource(ctx, 0, fs.WithEntity(entity), fs.WithPolicy(fm.CastStoragePolicyOnSlave(ctx, t.state.Policy)))
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to get entity source: %w", err)
	}

	defer es.Close()

	// 2. Identify files format
	format, readStream, err := archives.Identify(ctx, t.state.FileName, es)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to identify archive format: %w", err)
	}
	t.l.WithContext(ctx).Infof("Archive files %q format identified as %q", t.state.FileName, format.Extension())

	extractor, ok := format.(archives.Extractor)
	if !ok {
		return task.StatusError, fmt.Errorf("format not an extractor %q", format.Extension())
	}

	formatExt := format.Extension()
	if formatExt == ".zip" || formatExt == ".7z" {
		if _, err = es.Seek(0, 0); err != nil {
			return task.StatusError, fmt.Errorf("failed to seek entity source: %w", err)
		}

		if t.state.TempZipFilePath == "" && !es.IsLocal() {
			tempPath, err := prepareTempFolder(ctx, dep, t)
			if err != nil {
				return task.StatusError, fmt.Errorf("failed to prepare temp folder: %w", err)
			}
			t.state.TempPath = tempPath

			fileName := fmt.Sprintf("%s.zip", uuid.Must(uuid.NewV4()))
			zipFilePath := filepath.Join(
				t.state.TempPath,
				fileName,
			)
			zipFile, err := util.CreateNestedFile(zipFilePath)
			if err != nil {
				return task.StatusError, fmt.Errorf("failed to create zip files: %w", err)
			}

			t.Lock()
			t.progress.ProgressMap[ProgressTypeDownload] = &pbexplorer.Progress{Total: es.Entity().Size()}
			t.Unlock()

			defer zipFile.Close()
			if _, err := io.Copy(zipFile, util.NewCallbackReader(es, func(i int64) {
				atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeDownload].Current, i)
			})); err != nil {
				return task.StatusError, fmt.Errorf("failed to copy zip files to local temp: %w", err)
			}

			zipFile.Close()
			t.state.TempZipFilePath = zipFilePath
		}

		if es.IsLocal() {
			readStream = es
		} else if t.state.TempZipFilePath != "" {
			// Use temp zip files path
			zipFile, err := os.Open(t.state.TempZipFilePath)
			if err != nil {
				return task.StatusError, fmt.Errorf("failed to open temp zip files: %w", err)
			}

			defer zipFile.Close()
			readStream = zipFile
		}

		if es.IsLocal() {
			readStream = es
		}
	}

	if zipExtractor, ok := extractor.(archives.Zip); ok {
		if t.state.Encoding != "" {
			t.l.WithContext(ctx).Infof("Using encoding %q for zip archive", t.state.Encoding)
			encoding, ok := manager.ZipEncodings[strings.ToLower(t.state.Encoding)]
			if !ok {
				t.l.WithContext(ctx).Warnf("Unknown encoding %q, fallback to default encoding", t.state.Encoding)
			} else {
				zipExtractor.TextEncoding = encoding
				extractor = zipExtractor
			}
		}
	} else if rarExtractor, ok := extractor.(archives.Rar); ok && t.state.Password != "" {
		rarExtractor.Password = t.state.Password
		extractor = rarExtractor
	} else if sevenZipExtractor, ok := extractor.(archives.SevenZip); ok && t.state.Password != "" {
		sevenZipExtractor.Password = t.state.Password
		extractor = sevenZipExtractor
	}

	needSkipToCursor := false
	if t.state.ProcessedCursor != "" {
		needSkipToCursor = true
	}

	// 3. Extract and upload
	err = extractor.Extract(ctx, readStream, func(ctx context.Context, f archives.FileInfo) error {
		if needSkipToCursor && f.NameInArchive != t.state.ProcessedCursor {
			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractCount].Current, 1)
			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractSize].Current, f.Size())
			t.l.WithContext(ctx).Infof("File %q already processed, skipping...", f.NameInArchive)
			return nil
		}

		// Found cursor, start from cursor +1
		if t.state.ProcessedCursor == f.NameInArchive {
			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractCount].Current, 1)
			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractSize].Current, f.Size())
			needSkipToCursor = false
			return nil
		}

		rawPath := util.FormSlash(f.NameInArchive)
		savePath := dst.JoinRaw(rawPath)

		// If files mask is not empty, check if the path is in the mask
		if len(t.state.FileMask) > 0 && !isFileInMask(rawPath, t.state.FileMask) {
			t.l.WithContext(ctx).Debugf("File %q is not in the mask, skipping...", f.NameInArchive)
			return nil
		}

		// Check if path is legit
		if !strings.HasPrefix(savePath.Path(), util.FillSlash(path.Clean(dst.Path()))) {
			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractCount].Current, 1)
			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractSize].Current, f.Size())
			t.l.WithContext(ctx).Warnf("Path %q is not legit, skipping...", f.NameInArchive)
			return nil
		}

		if f.FileInfo.IsDir() {
			_, err := fm.Create(ctx, savePath, types.FileTypeFolder, fs.WithNode(t.node), fs.WithStatelessUserID(t.state.UserID))
			if err != nil {
				t.l.WithContext(ctx).Warnf("Failed to create directory %q: %s, skipping...", rawPath, err)
			}

			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractCount].Current, 1)
			t.state.ProcessedCursor = f.NameInArchive
			return nil
		}

		fileStream, err := f.Open()
		if err != nil {
			t.l.WithContext(ctx).Warnf("Failed to open files %q in archive files: %s, skipping...", rawPath, err)
			return nil
		}

		fileData := &fs.UploadRequest{
			Props: &fs.UploadProps{
				Uri:  savePath,
				Size: f.Size(),
				LastModified: func() *time.Time {
					t := f.FileInfo.ModTime().Local()
					return &t
				}(),
			},
			ProgressFunc: func(current, diff int64, total int64) {
				atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractSize].Current, diff)
			},
			File: fileStream,
		}

		_, err = fm.Update(ctx, fileData, fs.WithNode(t.node), fs.WithStatelessUserID(t.state.UserID), fs.WithNoEntityType())
		if err != nil {
			return fmt.Errorf("failed to upload files %q in archive files: %w", rawPath, err)
		}

		atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeExtractCount].Current, 1)
		t.state.ProcessedCursor = f.NameInArchive
		return nil
	})

	if err != nil {
		return task.StatusError, fmt.Errorf("failed to extract archive: %w", err)
	}

	return task.StatusCompleted, nil
}

func (t *SlaveExtractArchiveTask) Cleanup(ctx context.Context) error {
	if t.state.TempPath != "" {
		time.Sleep(time.Duration(1) * time.Second)
		return os.RemoveAll(t.state.TempPath)
	}

	return nil
}

func (t *SlaveExtractArchiveTask) Progress(ctx context.Context) *pbexplorer.TaskPhaseProgressResponse {
	t.Lock()
	defer t.Unlock()
	return t.progress
}

func isFileInMask(path string, mask []string) bool {
	if len(mask) == 0 {
		return true
	}

	for _, m := range mask {
		if path == m || strings.HasPrefix(path, m+"/") {
			return true
		}
	}

	return false
}
