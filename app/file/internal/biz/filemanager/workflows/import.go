package workflows

import (
	pb "api/api/file/common/v1"
	explorerpb "api/api/file/workflow/v1"
	userpb "api/api/user/common/v1"
	"api/external/trans"
	"common/hashid"
	"common/serializer"
	"common/util"
	"context"
	"encoding/json"
	"errors"
	"file/ent"
	"file/ent/task"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/manager"
	"file/internal/biz/queue"
	"file/internal/data/types"
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/go-kratos/kratos/v2/log"
)

type (
	ImportTask struct {
		*queue.DBTask

		l        *log.Helper
		state    *ImportTaskState
		progress *explorerpb.TaskPhaseProgressResponse
	}
	ImportTaskState struct {
		PolicyID         int             `json:"policy_id"`
		Src              string          `json:"src"`
		Recursive        bool            `json:"is_recursive"`
		Dst              string          `json:"dst"`
		Phase            ImportTaskPhase `json:"phase"`
		Failed           int             `json:"failed,omitempty"`
		ExtractMediaMeta bool            `json:"extract_media_meta"`
	}
	ImportTaskPhase string
)

const (
	ProgressTypeImported = "imported"
	ProgressTypeIndexed  = "indexed"
)

func init() {
	queue.RegisterResumableTaskFactory(queue.ImportTaskType, NewImportTaskFromModel)
}

func NewImportTask(ctx context.Context, user *userpb.User, src string, recursive bool, dst string, policyID int) (queue.Task, error) {
	state := &ImportTaskState{
		Src:       src,
		Recursive: recursive,
		Dst:       dst,
		PolicyID:  policyID,
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}

	t := &ImportTask{
		DBTask: &queue.DBTask{
			Task: &ent.Task{
				Type:         queue.ImportTaskType,
				TraceID:      util.TraceID(ctx),
				PrivateState: string(stateBytes),
				PublicState:  &pb.TaskPublicState{},
			},
			DirectOwner: user,
		},
	}

	return t, nil
}

func NewImportTaskFromModel(task *ent.Task) queue.Task {
	return &ImportTask{
		DBTask: &queue.DBTask{
			Task: task,
		},
	}
}

func (t *ImportTask) Do(ctx context.Context) (task.Status, error) {
	dep := filemanager.ManagerDepFromContext(ctx)
	dbfsDep := filemanager.DBFSDepFromContext(ctx)
	t.l = dep.Logger()

	t.Lock()
	if t.progress == nil {
		t.progress = &explorerpb.TaskPhaseProgressResponse{
			ProgressMap: make(map[string]*explorerpb.Progress),
		}
	}
	t.progress.ProgressMap[ProgressTypeIndexed] = &explorerpb.Progress{}
	t.Unlock()

	// unmarshal state
	state := &ImportTaskState{}
	if err := json.Unmarshal([]byte(t.State()), state); err != nil {
		return task.StatusError, fmt.Errorf("failed to unmarshal state: %w", err)
	}
	t.state = state

	next, err := t.processImport(ctx, dep, dbfsDep)

	newStateStr, marshalErr := json.Marshal(t.state)
	if marshalErr != nil {
		return task.StatusError, fmt.Errorf("failed to marshal state: %w", marshalErr)
	}

	t.Lock()
	t.Task.PrivateState = string(newStateStr)
	t.Unlock()
	return next, err
}

func (t *ImportTask) processImport(ctx context.Context, dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep) (task.Status, error) {
	user := trans.FromContext(ctx)
	fm := manager.NewFileManager(dep, dbfsDep, user)
	defer fm.Recycle()

	failed := 0
	dst, err := fs.NewUriFromString(t.state.Dst)
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to parse dst: %s (%w)", err, queue.CriticalErr)
	}

	physicalFiles, err := fm.ListPhysical(ctx, t.state.Src, t.state.PolicyID, t.state.Recursive,
		func(i int) {
			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeIndexed].Current, int64(i))
		})
	if err != nil {
		return task.StatusError, fmt.Errorf("failed to list physical files: %w", err)
	}

	t.l.WithContext(ctx).Infof("Importing %d physical files", len(physicalFiles))

	t.Lock()
	t.progress.ProgressMap[ProgressTypeImported] = &explorerpb.Progress{
		Total: int64(len(physicalFiles)),
	}
	delete(t.progress.ProgressMap, ProgressTypeIndexed)
	t.Unlock()

	for _, physicalFile := range physicalFiles {
		if physicalFile.IsDir {
			t.l.WithContext(ctx).Infof("Creating folder %s", physicalFile.RelativePath)
			_, err := fm.Create(ctx, dst.Join(physicalFile.RelativePath), types.FileTypeFolder)
			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeImported].Current, 1)
			if err != nil {
				t.l.WithContext(ctx).Warnf("Failed to create folder %s: %s", physicalFile.RelativePath, err)
				failed++
			}
		} else {
			t.l.WithContext(ctx).Infof("Importing files %s", physicalFile.RelativePath)
			err := fm.ImportPhysical(ctx, dst, t.state.PolicyID, physicalFile, t.state.ExtractMediaMeta)
			atomic.AddInt64(&t.progress.ProgressMap[ProgressTypeImported].Current, 1)
			if err != nil {
				var appErr serializer.AppError
				if errors.As(err, &appErr) && appErr.Code == serializer.CodeObjectExist {
					t.l.WithContext(ctx).Infof("File %s already exists, skipping", physicalFile.RelativePath)
					continue
				}
				t.l.WithContext(ctx).Errorf("Failed to import files %s: %s, skipping", physicalFile.RelativePath, err)
				failed++
			}
		}
	}

	return task.StatusCompleted, nil
}

func (t *ImportTask) Progress(ctx context.Context) *explorerpb.TaskPhaseProgressResponse {
	t.Lock()
	defer t.Unlock()
	return t.progress
}

func (t *ImportTask) Summarize(hasher hashid.Encoder) *explorerpb.Summary {
	// unmarshal state
	if t.state == nil {
		if err := json.Unmarshal([]byte(t.State()), &t.state); err != nil {
			return nil
		}
	}

	return &explorerpb.Summary{
		Phase: string(t.state.Phase),
		Props: map[string]string{
			SummaryKeyDst:            t.state.Dst,
			SummaryKeySrcStr:         t.state.Src,
			SummaryKeyFailed:         strconv.Itoa(t.state.Failed),
			SummaryKeySrcDstPolicyID: hashid.EncodePolicyID(hasher, t.state.PolicyID),
		},
	}
}
