package workflows

import (
	pb "api/api/file/common/v1"
	explorerpb "api/api/file/workflow/v1"
	"common/serializer"
	"common/util"
	"context"
	"encoding/json"
	"file/ent"
	"file/ent/task"
	"file/internal/biz/cluster"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/manager"
	"file/internal/biz/queue"
	"file/internal/data/types"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/go-kratos/kratos/v2/log"
)

type (
	SlaveUploadEntity struct {
		Uri   *fs.URI `json:"uri"`
		Src   string  `json:"src"`
		Size  int64   `json:"size"`
		Index int     `json:"index"`
	}
	SlaveUploadTaskState struct {
		MaxParallel          int                 `json:"max_parallel"`
		Files                []SlaveUploadEntity `json:"files"`
		Transferred          map[int]interface{} `json:"transferred"`
		UserID               int                 `json:"user_id"`
		First5TransferErrors string              `json:"first_5_transfer_errors,omitempty"`
	}
	SlaveUploadTask struct {
		*queue.InMemoryTask

		progress *explorerpb.TaskPhaseProgressResponse
		l        *log.Helper
		state    *SlaveUploadTaskState
		node     cluster.Node
	}
)

// NewSlaveUploadTask creates a new SlaveUploadTask from raw private state
func NewSlaveUploadTask(ctx context.Context, props *pb.SlaveTaskProps, id int, state string) queue.Task {
	return &SlaveUploadTask{
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

		progress: &explorerpb.TaskPhaseProgressResponse{
			ProgressMap: make(map[string]*explorerpb.Progress),
		},
	}
}

func (t *SlaveUploadTask) Do(ctx context.Context) (task.Status, error) {
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
	state := &SlaveUploadTaskState{}
	if err := json.Unmarshal([]byte(t.State()), state); err != nil {
		return task.StatusError, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	t.state = state
	if t.state.Transferred == nil {
		t.state.Transferred = make(map[int]interface{})
	}

	wg := sync.WaitGroup{}
	worker := make(chan int, t.state.MaxParallel)
	for i := 0; i < t.state.MaxParallel; i++ {
		worker <- i
	}

	// Sum up total count
	totalCount := 0
	totalSize := int64(0)
	for _, res := range state.Files {
		totalSize += res.Size
		totalCount++
	}
	t.Lock()
	t.progress.ProgressMap[ProgressTypeUploadCount] = &explorerpb.Progress{}
	t.progress.ProgressMap[ProgressTypeUpload] = &explorerpb.Progress{}
	t.Unlock()
	atomic.StoreInt64(&t.progress.ProgressMap[ProgressTypeUploadCount].Total, int64(totalCount))
	atomic.StoreInt64(&t.progress.ProgressMap[ProgressTypeUpload].Total, totalSize)
	ae := serializer.NewAggregateError()
	transferFunc := func(workerId, fileId int, file SlaveUploadEntity) {
		t.l.WithContext(ctx).Infof("Uploading files %s to %s...", file.Src, file.Uri.String())

		progressKey := fmt.Sprintf("%s%d", ProgressTypeUploadSinglePrefix, workerId)
		t.Lock()
		t.progress.ProgressMap[progressKey] = &explorerpb.Progress{Identifier: file.Uri.String(), Total: file.Size}
		fileProgress := t.progress.ProgressMap[progressKey]
		uploadProgress := t.progress.ProgressMap[ProgressTypeUpload]
		uploadCountProgress := t.progress.ProgressMap[ProgressTypeUploadCount]
		t.Unlock()

		defer func() {
			atomic.AddInt64(&uploadCountProgress.Current, 1)
			worker <- workerId
			wg.Done()
		}()

		handle, err := os.Open(filepath.FromSlash(file.Src))
		if err != nil {
			t.l.WithContext(ctx).Warnf("Failed to open files %s: %s", file.Src, err.Error())
			atomic.AddInt64(&fileProgress.Current, file.Size)
			ae.Add(path.Base(file.Src), fmt.Errorf("failed to open files: %w", err))
			return
		}

		stat, err := handle.Stat()
		if err != nil {
			t.l.WithContext(ctx).Warnf("Failed to get files stat for %s: %s", file.Src, err.Error())
			handle.Close()
			atomic.AddInt64(&fileProgress.Current, file.Size)
			ae.Add(path.Base(file.Src), fmt.Errorf("failed to get files stat: %w", err))
			return
		}

		fileData := &fs.UploadRequest{
			Props: &fs.UploadProps{
				Uri:  file.Uri,
				Size: stat.Size(),
			},
			ProgressFunc: func(current, diff int64, total int64) {
				atomic.AddInt64(&fileProgress.Current, diff)
				atomic.AddInt64(&uploadCountProgress.Current, 1)
				atomic.StoreInt64(&fileProgress.Total, total)
			},
			File:   handle,
			Seeker: handle,
		}

		_, err = fm.Update(ctx, fileData, fs.WithNode(t.node), fs.WithStatelessUserID(t.state.UserID), fs.WithNoEntityType())
		if err != nil {
			handle.Close()
			t.l.WithContext(ctx).Warnf("Failed to upload files %s: %s", file.Src, err.Error())
			atomic.AddInt64(&uploadProgress.Current, file.Size)
			ae.Add(path.Base(file.Src), fmt.Errorf("failed to upload files: %w", err))
			return
		}

		t.Lock()
		t.state.Transferred[fileId] = nil
		t.Unlock()
		handle.Close()
	}

	// Start upload files
	for fileId, file := range t.state.Files {
		// Check if files is already transferred
		if _, ok := t.state.Transferred[fileId]; ok {
			t.l.WithContext(ctx).Infof("File %s already transferred, skipping...", file.Src)
			t.Lock()
			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeUpload].Current, file.Size)
			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeUploadCount].Current, 1)
			t.Unlock()
			continue
		}

		select {
		case <-ctx.Done():
			return task.StatusError, ctx.Err()
		case workerId := <-worker:
			wg.Add(1)

			go transferFunc(workerId, fileId, file)
		}
	}

	wg.Wait()

	t.state.First5TransferErrors = ae.FormatFirstN(5)
	newStateStr, marshalErr := json.Marshal(t.state)
	if marshalErr != nil {
		return task.StatusError, fmt.Errorf("failed to marshal state: %w", marshalErr)
	}
	t.Lock()
	t.Task.PrivateState = string(newStateStr)
	t.Unlock()

	// If all files are failed to transfer, return error
	if len(t.state.Transferred) != len(t.state.Files) {
		t.l.WithContext(ctx).Warnf("%d files not transferred", len(t.state.Files)-len(t.state.Transferred))
		if len(t.state.Transferred) == 0 {
			return task.StatusError, fmt.Errorf("all files failed to transfer")
		}

	}

	return task.StatusCompleted, nil
}

func (m *SlaveUploadTask) Progress(ctx context.Context) *explorerpb.TaskPhaseProgressResponse {
	m.Lock()
	defer m.Unlock()

	res := &explorerpb.TaskPhaseProgressResponse{
		ProgressMap: make(map[string]*explorerpb.Progress),
	}
	for k, v := range m.progress.ProgressMap {
		res.ProgressMap[k] = v
	}
	return res
}
