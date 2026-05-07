package workflow

import (
	"ai/ent"
	"ai/internal/biz/queue"
	"ai/internal/data"
	"api/external/trans"
	"common/cache"
	"common/constants"
	"context"
	"fmt"
	mqueue "queue"
	"strconv"
	"time"
)

const (
	ResumableTaskKeyPrefix = "resumable_task:"
	ResumableTaskTTL       = int((30 * time.Minute) / time.Second)
)

type WorkflowBiz interface {
	ListTasks(ctx context.Context, args *data.ListTaskArgs) (*data.ListTaskResult, error)
	GetTask(ctx context.Context, id int) (*ent.Task, error)
	GetTaskPhaseProgress(ctx context.Context, id int) (mqueue.Progresses, error)
	CancelTasks(ctx context.Context, ids []int, taskType string, terminate bool) error
	ResumeTasks(ctx context.Context, ids []int, taskType string) error
}

type workflowBiz struct {
	tc data.TaskClient
	kv cache.Driver
	qm *queue.QueueManager
}

func NewWorkflowBiz(tc data.TaskClient, kv cache.Driver, qm *queue.QueueManager) WorkflowBiz {
	return &workflowBiz{
		tc: tc,
		kv: kv,
		qm: qm,
	}
}

func (b *workflowBiz) ListTasks(ctx context.Context, args *data.ListTaskArgs) (*data.ListTaskResult, error) {
	return b.tc.List(ctx, args)
}

func (b *workflowBiz) GetTask(ctx context.Context, id int) (*ent.Task, error) {
	t, err := b.tc.GetTaskByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query task: %w", err)
	}
	user := trans.FromContext(ctx)
	if t.UserID != user.ID && !user.Group.Permissions.Enabled(int(constants.GroupPermissionIsAdmin)) {
		return nil, fmt.Errorf("user is not allowed to view task")
	}
	return t, nil
}

func (b *workflowBiz) DeleteTasks(ctx context.Context, id ...int) error {
	return b.tc.DeleteByIDs(ctx, id...)
}

func (b *workflowBiz) GetTaskPhaseProgress(ctx context.Context, id int) (mqueue.Progresses, error) {
	model, err := b.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	t, err := mqueue.NewTaskFromModel(data.NewTaskModel(model))
	if err != nil {
		return nil, err
	}
	return t.Progress(ctx), nil
}

func (b *workflowBiz) CancelTasks(ctx context.Context, ids []int, taskType string, terminate bool) error {
	if len(ids) == 0 {
		return nil
	}
	if err := validateTaskType(taskType); err != nil {
		return err
	}

	user := trans.FromContext(ctx)
	ownerID := user.ID
	if user.Group.Permissions.Enabled(int(constants.GroupPermissionIsAdmin)) {
		ownerID = 0
	}

	cancelledTasks, err := b.tc.CancelTasks(ctx, ids, taskType, ownerID)
	if err != nil {
		return fmt.Errorf("failed to cancel tasks: %w", err)
	}
	cancelledIDs := taskIDs(cancelledTasks)
	if len(cancelledIDs) == 0 {
		return nil
	}

	switch taskType {
	case queue.IngestTaskType:
		if q := b.qm.IngestQueue(); q != nil {
			q.CancelTasks(ctx, cancelledIDs...)
		}
	case queue.ReindexTaskType:
		if q := b.qm.ReindexQueue(); q != nil {
			q.CancelTasks(ctx, cancelledIDs...)
		}
	}

	if terminate {
		return b.tc.DeleteByIDs(ctx, cancelledIDs...)
	}

	return b.addResumableTaskIDs(taskType, user.ID, cancelledIDs)
}

func (b *workflowBiz) ResumeTasks(ctx context.Context, ids []int, taskType string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := validateTaskType(taskType); err != nil {
		return err
	}

	user := trans.FromContext(ctx)
	requestedIDs := uniqueInts(ids)
	resumeIDs := b.filterResumableTaskIDs(taskType, user.ID, requestedIDs)
	if len(resumeIDs) == 0 {
		return fmt.Errorf("no requested resumable tasks found")
	}

	ownerID := user.ID
	if user.Group.Permissions.Enabled(int(constants.GroupPermissionIsAdmin)) {
		ownerID = 0
	}

	resumedTasks, err := b.tc.ResumeTasks(ctx, resumeIDs, taskType, ownerID)
	if err != nil {
		return fmt.Errorf("failed to resume tasks: %w", err)
	}
	resumedIDs := taskIDs(resumedTasks)
	if len(resumedIDs) == 0 {
		return fmt.Errorf("no requested resumable tasks found")
	}
	if err := b.qm.ResumeTasks(ctx, resumedIDs, taskType); err != nil {
		return fmt.Errorf("failed to queue resumed tasks: %w", err)
	}

	return b.removeResumableTaskIDs(taskType, user.ID, resumedIDs)
}

func validateTaskType(taskType string) error {
	switch taskType {
	case queue.IngestTaskType, queue.ReindexTaskType:
		return nil
	default:
		return fmt.Errorf("invalid task type: %s", taskType)
	}
}

func resumableTaskKeyPrefix(taskType string, userID int) string {
	return fmt.Sprintf("%s%s:%d:", ResumableTaskKeyPrefix, taskType, userID)
}

func resumableTaskKey(taskType string, userID, taskID int) string {
	return resumableTaskKeyPrefix(taskType, userID) + strconv.Itoa(taskID)
}

func taskIDs(tasks []*ent.Task) []int {
	ids := make([]int, 0, len(tasks))
	for _, task := range tasks {
		if task != nil {
			ids = append(ids, task.ID)
		}
	}
	return ids
}

func uniqueInts(ids []int) []int {
	seen := make(map[int]struct{}, len(ids))
	res := make([]int, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		res = append(res, id)
	}
	return res
}

func (b *workflowBiz) addResumableTaskIDs(taskType string, userID int, ids []int) error {
	for _, id := range uniqueInts(ids) {
		key := resumableTaskKey(taskType, userID, id)
		if _, ok := b.kv.Get(key); ok {
			continue
		}
		if err := b.kv.Set(key, true, ResumableTaskTTL); err != nil {
			return err
		}
	}
	return nil
}

func (b *workflowBiz) filterResumableTaskIDs(taskType string, userID int, ids []int) []int {
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, strconv.Itoa(id))
	}
	found, _ := b.kv.Gets(keys, resumableTaskKeyPrefix(taskType, userID))

	res := make([]int, 0, len(found))
	for _, id := range ids {
		if _, ok := found[strconv.Itoa(id)]; ok {
			res = append(res, id)
		}
	}
	return res
}

func (b *workflowBiz) removeResumableTaskIDs(taskType string, userID int, ids []int) error {
	keys := make([]string, 0, len(ids))
	for _, id := range uniqueInts(ids) {
		keys = append(keys, strconv.Itoa(id))
	}
	return b.kv.Delete(resumableTaskKeyPrefix(taskType, userID), keys...)
}
