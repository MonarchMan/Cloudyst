package queue

import (
	"common/hashid"
	"common/util"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gofrs/uuid"
	"github.com/samber/lo"
)

type (
	Task interface {
		TaskRecord
		// Model returns the Task model with basic info
		Model() TaskRecord
		// Do executes the Task
		Do(ctx context.Context) (TaskStatus, error)
		// ShouldPersist returns true if the Task should be persisted into DB
		ShouldPersist() bool
		// Persisted returns true if the Task is persisted in DB
		Persisted() bool
		// Executed returns the duration of the Task execution
		Executed() time.Duration
		// ResumeAfter sets the time when the Task should be resumed
		ResumeAfter(next time.Duration)
		// Progress returns the Task progress
		Progress(ctx context.Context) Progresses
		// Summarize returns the Task summary for UI display
		Summarize(hasher hashid.Encoder) *Summary
		// CanCancel returns true if the Task can be canceled
		CanCancel() bool
		// OnSuspend is called when queue decides to suspend the Task
		OnSuspend(time int64)
		// OnPersisted is called when the Task is persisted or updated in DB
		OnPersisted(task TaskRecord)
		// OnError is called when the Task encounters an error
		OnError(err error, d time.Duration)
		// OnRetry is called when the iteration returns error and before retry
		OnRetry(err error)
		// OnIterationComplete is called when the one iteration is completed
		OnIterationComplete(executed time.Duration)
		// OnStatusTransition is called when the Task status is changed
		OnStatusTransition(newStatus TaskStatus)

		// Cleanup is called when the Task is done or error.
		Cleanup(ctx context.Context) error

		Lock()
		Unlock()
	}
	ResumableTaskFactory func(model TaskRecord) Task
	Progress             struct {
		Total      int64  `json:"total"`
		Current    int64  `json:"current"`
		Identifier string `json:"identifier"`
	}
	Progresses map[string]*Progress
	Summary    struct {
		NodeID int            `json:"-"`
		Phase  string         `json:"phase,omitempty"`
		Props  map[string]any `json:"props,omitempty"`
	}

	stateTransition  func(ctx context.Context, task Task, newStatus TaskStatus, q *queue) error
	manualResumeTask interface {
		RequireManualResume() bool
	}
)

var (
	taskFactories sync.Map
)

func init() {
	gob.Register(Progresses{})
}

// RegisterResumableTaskFactory registers a resumable Task factory
func RegisterResumableTaskFactory(taskType string, factory ResumableTaskFactory) {
	taskFactories.Store(taskType, factory)
}

func shouldAutoResume(task Task) bool {
	if t, ok := task.(manualResumeTask); ok {
		return !t.RequireManualResume()
	}
	return true
}

// NewTaskFromModel creates a Task from ent.Task model
func NewTaskFromModel(model TaskRecord) (Task, error) {
	if factory, ok := taskFactories.Load(model.Type()); ok {
		return factory.(ResumableTaskFactory)(model), nil
	}

	return nil, fmt.Errorf("unknown Task type: %s", model.Type())
}

// TaskModel 是 InMemoryTask 内部持有的纯 Go 结构，
type TaskModel struct {
	ModelID           int
	ModelType         string
	ModelStatus       TaskStatus
	ModelPrivateState string
	ModelPublicState  *TaskPublicState
	ModelTraceID      string
	ModelOwner        int
}

func (t *TaskModel) ID() int {
	return t.ModelID
}

func (t *TaskModel) Type() string {
	return t.ModelType
}

func (t *TaskModel) Status() TaskStatus {
	return t.ModelStatus
}

func (t *TaskModel) OwnerID() int {
	return t.ModelOwner
}

func (t *TaskModel) State() string {
	return t.ModelPrivateState
}

func (t *TaskModel) Retried() int {
	return t.ModelPublicState.RetryCount
}

func (t *TaskModel) Error() error {
	return errors.New(t.ModelPublicState.Error)
}

func (t *TaskModel) ErrorHistory() []error {
	return lo.Map(t.ModelPublicState.ErrorHistory, func(err string, index int) error {
		return errors.New(err)
	})
}

func (t *TaskModel) TraceID() string {
	return t.ModelTraceID
}

func (t *TaskModel) ResumeTime() int64 {
	return t.ModelPublicState.ResumeTime
}

// InMemoryTask implements part Task interface using in-memory constants.
type InMemoryTask struct {
	mu sync.Mutex
	*TaskModel
}

func NewInMemoryTask(taskType string, owner int) *InMemoryTask {
	id, _ := uuid.NewV4()
	return &InMemoryTask{
		TaskModel: &TaskModel{
			ModelType:        taskType,
			ModelStatus:      StatusQueued,
			ModelTraceID:     id.String(),
			ModelOwner:       owner,
			ModelPublicState: &TaskPublicState{},
		},
	}
}

// —— Task 接口实现 ——

func (t *InMemoryTask) ID() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ModelID
}

func (t *InMemoryTask) Type() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ModelType
}

func (t *InMemoryTask) Status() TaskStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ModelStatus
}

func (t *InMemoryTask) OwnerID() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ModelOwner
}

func (t *InMemoryTask) State() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ModelPrivateState
}

func (t *InMemoryTask) PublicState() *TaskPublicState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ModelPublicState
}

// SetState 供具体任务类型更新自己的 state JSON
func (t *InMemoryTask) SetState(s string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ModelPrivateState = s
}

func (t *InMemoryTask) ShouldPersist() bool {
	return false
}

func (t *InMemoryTask) Persisted() bool {
	return false
}

func (t *InMemoryTask) Executed() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ModelPublicState.ExecutedDuration
}

func (t *InMemoryTask) Retried() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ModelPublicState.RetryCount
}

func (t *InMemoryTask) Error() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ModelPublicState.Error != "" {
		return errors.New(t.ModelPublicState.Error)
	}
	return nil
}

func (t *InMemoryTask) ErrorHistory() []error {
	t.mu.Lock()
	defer t.mu.Unlock()
	errs := make([]error, len(t.ModelPublicState.ErrorHistory))
	for i, s := range t.ModelPublicState.ErrorHistory {
		errs[i] = errors.New(s)
	}
	return errs
}

func (t *InMemoryTask) TraceID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ModelTraceID
}

func (t *InMemoryTask) ResumeTime() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ModelPublicState.ResumeTime
}

func (t *InMemoryTask) ResumeAfter(next time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ModelPublicState.ResumeTime = time.Now().Add(next).Unix()
}

func (t *InMemoryTask) Progress(_ context.Context) Progresses {
	return nil
}

func (t *InMemoryTask) Summarize(_ hashid.Encoder) *Summary {
	return &Summary{}
}

func (t *InMemoryTask) Model() TaskRecord {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t
}

func (t *InMemoryTask) CanCancel() bool {
	return false
}

// —— 生命周期回调 ——

func (t *InMemoryTask) OnStatusTransition(newStatus TaskStatus) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ModelStatus = newStatus
}

// OnPersisted 对 InMemoryTask 无意义，空实现保持接口一致
func (t *InMemoryTask) OnPersisted(_ TaskRecord) {}

func (t *InMemoryTask) OnError(err error, d time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ModelPublicState.Error = err.Error()
	t.ModelPublicState.ExecutedDuration += d
}

func (t *InMemoryTask) OnRetry(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ModelPublicState.ErrorHistory = append(
		t.ModelPublicState.ErrorHistory, err.Error(),
	)
	t.ModelPublicState.RetryCount++
}

func (t *InMemoryTask) OnIterationComplete(d time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ModelPublicState.ExecutedDuration += d
}

func (t *InMemoryTask) OnSuspend(resumeTime int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ModelPublicState.ResumeTime = resumeTime
}

func (t *InMemoryTask) Cleanup(_ context.Context) error {
	return nil
}

func (t *InMemoryTask) Lock() {
	t.mu.Lock()
}

func (t *InMemoryTask) Unlock() {
	t.mu.Unlock()
}

var stateTransitions map[TaskStatus]map[TaskStatus]stateTransition

func init() {
	stateTransitions = map[TaskStatus]map[TaskStatus]stateTransition{
		"": {
			StatusQueued: persistTask,
		},
		StatusQueued: {
			StatusProcessing: func(ctx context.Context, task Task, newStatus TaskStatus, q *queue) error {
				if err := persistTask(ctx, task, newStatus, q); err != nil {
					return err
				}
				return nil
			},
			StatusQueued: func(ctx context.Context, task Task, newStatus TaskStatus, q *queue) error {
				return nil
			},
			StatusError: func(ctx context.Context, task Task, newStatus TaskStatus, q *queue) error {
				q.metric.IncFailureTask()
				return persistTask(ctx, task, newStatus, q)
			},
			StatusCanceled: func(ctx context.Context, task Task, newStatus TaskStatus, q *queue) error {
				q.metric.IncFailureTask()
				if err := task.Cleanup(ctx); err != nil {
					q.logger.Error("Task cleanup failed: %s", err.Error())
				}
				if q.registry != nil {
					q.registry.Delete(task.ID())
				}
				return persistTask(ctx, task, newStatus, q)
			},
		},
		StatusProcessing: {
			StatusQueued: persistTask,
			StatusCompleted: func(ctx context.Context, task Task, newStatus TaskStatus, q *queue) error {
				q.logger.Info("Execution completed in %s with %d retries, clean up...", task.Executed(), task.Retried())
				q.metric.IncSuccessTask()

				if err := task.Cleanup(ctx); err != nil {
					q.logger.Error("Task cleanup failed: %s", err.Error())
				}

				if q.registry != nil {
					q.registry.Delete(task.ID())
				}

				if err := persistTask(ctx, task, newStatus, q); err != nil {
					return err
				}
				return nil
			},
			StatusError: func(ctx context.Context, task Task, newStatus TaskStatus, q *queue) error {
				q.logger.Error("Execution failed with error in %s with %d retries, clean up...", task.Executed(), task.Retried())
				q.metric.IncFailureTask()

				if err := task.Cleanup(ctx); err != nil {
					q.logger.Error("Task cleanup failed: %s", err.Error())
				}

				if q.registry != nil {
					q.registry.Delete(task.ID())
				}

				if err := persistTask(ctx, task, newStatus, q); err != nil {
					return err
				}

				return nil
			},
			StatusCanceled: func(ctx context.Context, task Task, newStatus TaskStatus, q *queue) error {
				q.logger.Info("Execution canceled, clean up...", task.Executed(), task.Retried())
				q.metric.IncFailureTask()

				if err := task.Cleanup(ctx); err != nil {
					q.logger.Error("Task cleanup failed: %s", err.Error())
				}

				if q.registry != nil {
					q.registry.Delete(task.ID())
				}

				if err := persistTask(ctx, task, newStatus, q); err != nil {
					return err
				}

				return nil
			},
			StatusProcessing: persistTask,
			StatusSuspending: func(ctx context.Context, task Task, newStatus TaskStatus, q *queue) error {
				q.metric.IncSuspendingTask()
				if err := persistTask(ctx, task, newStatus, q); err != nil {
					return err
				}
				q.logger.Info("Task %d suspended, resume time: %d", task.ID(), task.ResumeTime())
				if !shouldAutoResume(task) {
					return nil
				}
				return q.QueueTask(ctx, task)
			},
		},
		StatusSuspending: {
			StatusProcessing: func(ctx context.Context, task Task, newStatus TaskStatus, q *queue) error {
				q.metric.DecSuspendingTask()
				return persistTask(ctx, task, newStatus, q)
			},
			StatusError: func(ctx context.Context, task Task, newStatus TaskStatus, q *queue) error {
				q.metric.IncFailureTask()
				return persistTask(ctx, task, newStatus, q)
			},
			StatusCanceled: func(ctx context.Context, task Task, newStatus TaskStatus, q *queue) error {
				q.metric.DecSuspendingTask()
				q.metric.IncFailureTask()
				if err := task.Cleanup(ctx); err != nil {
					q.logger.Error("Task cleanup failed: %s", err.Error())
				}
				if q.registry != nil {
					q.registry.Delete(task.ID())
				}
				return persistTask(ctx, task, newStatus, q)
			},
		},
	}

}

func persistTask(ctx context.Context, task Task, newState TaskStatus, q *queue) error {
	// Persist Task into inventory
	if task.ShouldPersist() {
		if err := saveTaskToInventory(ctx, task, newState, q); err != nil {
			return err
		}
	} else {
		task.OnStatusTransition(newState)
	}

	return nil
}

func saveTaskToInventory(ctx context.Context, task Task, newStatus TaskStatus, q *queue) error {
	var (
		errStr     string
		errHistory []string
	)
	if err := task.Error(); err != nil {
		errStr = err.Error()
	}

	errHistory = lo.Map(task.ErrorHistory(), func(err error, index int) string {
		return err.Error()
	})

	args := &TaskArgs{
		Status: newStatus,
		Type:   task.Type(),
		PublicState: &TaskPublicState{
			RetryCount:       task.Retried(),
			ExecutedDuration: task.Executed(),
			ErrorHistory:     errHistory,
			Error:            errStr,
			ResumeTime:       task.ResumeTime(),
		},
		PrivateState: task.State(),
		OwnerID:      task.OwnerID(),
		TraceID:      util.TraceID(ctx),
	}

	var (
		res TaskRecord
		err error
	)

	if !task.Persisted() {
		res, err = q.taskClient.New(ctx, args)
	} else {
		res, err = q.taskClient.Update(ctx, task.Model(), args)
	}
	if err != nil {
		return fmt.Errorf("failed to persist Task into DB: %w", err)
	}

	task.OnPersisted(res)
	return nil
}
