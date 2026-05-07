package workflows

import (
	"api/external/data/userdata"
	"api/external/trans"
	"common/hashid"
	"common/serializer"
	"common/util"
	"context"
	"encoding/json"
	"errors"
	"file/ent"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/manager"
	"file/internal/biz/queue"
	"file/internal/data"
	"file/internal/data/types"
	"fmt"
	mqueue "queue"
	"sync/atomic"

	"github.com/go-kratos/kratos/v2/log"
)

type (
	ImportTask struct {
		*queue.DBTask

		l        *log.Helper
		state    *ImportTaskState
		progress mqueue.Progresses
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
	mqueue.RegisterResumableTaskFactory(queue.ImportTaskType, NewImportTaskFromModel)
}

func NewImportTask(ctx context.Context, user *userdata.User, src string, recursive bool, dst string, policyID int) (mqueue.Task, error) {
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
				PublicState:  &mqueue.TaskPublicState{},
			},
			DirectOwner: user,
		},
	}

	return t, nil
}

func NewImportTaskFromModel(task mqueue.TaskRecord) mqueue.Task {
	wrapped, ok := task.(*data.TaskModel)
	if !ok {
		return nil
	}
	return &ImportTask{
		DBTask: &queue.DBTask{
			Task: wrapped.Task,
		},
	}
}

func (t *ImportTask) Do(ctx context.Context) (mqueue.TaskStatus, error) {
	dep := filemanager.ManagerDepFromContext(ctx)
	dbfsDep := filemanager.DBFSDepFromContext(ctx)
	t.l = dep.Logger()

	t.Lock()
	if t.progress == nil {
		t.progress = make(mqueue.Progresses)
	}
	t.progress[ProgressTypeIndexed] = &mqueue.Progress{}
	t.Unlock()

	// unmarshal state
	state := &ImportTaskState{}
	if err := json.Unmarshal([]byte(t.State()), state); err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to unmarshal state: %w", err)
	}
	t.state = state

	next, err := t.processImport(ctx, dep, dbfsDep)

	newStateStr, marshalErr := json.Marshal(t.state)
	if marshalErr != nil {
		return mqueue.StatusError, fmt.Errorf("failed to marshal state: %w", marshalErr)
	}

	t.Lock()
	t.Task.PrivateState = string(newStateStr)
	t.Unlock()
	return next, err
}

func (t *ImportTask) processImport(ctx context.Context, dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep) (mqueue.TaskStatus, error) {
	user := trans.FromContext(ctx)
	fm := manager.NewFileManager(dep, dbfsDep, user)
	defer fm.Recycle()

	failed := 0
	dst, err := fs.NewUriFromString(t.state.Dst)
	if err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to parse dst: %s (%w)", err, queue.CriticalErr)
	}

	physicalFiles, err := fm.ListPhysical(ctx, t.state.Src, t.state.PolicyID, t.state.Recursive,
		func(i int) {
			atomic.AddInt64(&t.progress[ProgressTypeIndexed].Current, int64(i))
		})
	if err != nil {
		return mqueue.StatusError, fmt.Errorf("failed to list physical files: %w", err)
	}

	t.l.WithContext(ctx).Infof("Importing %d physical files", len(physicalFiles))

	t.Lock()
	t.progress[ProgressTypeImported] = &mqueue.Progress{
		Total: int64(len(physicalFiles)),
	}
	delete(t.progress, ProgressTypeIndexed)
	t.Unlock()

	for _, physicalFile := range physicalFiles {
		if physicalFile.IsDir {
			t.l.WithContext(ctx).Infof("Creating folder %s", physicalFile.RelativePath)
			_, err := fm.Create(ctx, dst.Join(physicalFile.RelativePath), types.FileTypeFolder)
			atomic.AddInt64(&t.progress[ProgressTypeImported].Current, 1)
			if err != nil {
				t.l.WithContext(ctx).Warnf("Failed to create folder %s: %s", physicalFile.RelativePath, err)
				failed++
			}
		} else {
			t.l.WithContext(ctx).Infof("Importing files %s", physicalFile.RelativePath)
			err := fm.ImportPhysical(ctx, dst, t.state.PolicyID, physicalFile, t.state.ExtractMediaMeta)
			atomic.AddInt64(&t.progress[ProgressTypeImported].Current, 1)
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

	return mqueue.StatusCompleted, nil
}

func (t *ImportTask) Progress(ctx context.Context) mqueue.Progresses {
	t.Lock()
	defer t.Unlock()
	return t.progress
}

func (t *ImportTask) Summarize(hasher hashid.Encoder) *mqueue.Summary {
	// unmarshal state
	if t.state == nil {
		if err := json.Unmarshal([]byte(t.State()), &t.state); err != nil {
			return nil
		}
	}

	return &mqueue.Summary{
		Phase: string(t.state.Phase),
		Props: map[string]any{
			SummaryKeyDst:            t.state.Dst,
			SummaryKeySrcStr:         t.state.Src,
			SummaryKeyFailed:         t.state.Failed,
			SummaryKeySrcDstPolicyID: hashid.EncodePolicyID(hasher, t.state.PolicyID),
		},
	}
}
