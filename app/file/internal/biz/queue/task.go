package queue

import (
	pb "api/api/file/common/v1"
	pbexplorer "api/api/file/workflow/v1"
	userpb "api/api/user/common/v1"
	"common/hashid"
	"common/util"
	"context"
	"encoding/gob"
	"errors"
	"file/ent"
	"file/ent/task"
	"file/internal/data"
	"fmt"
	"sync"
	"time"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/durationpb"
)

type (
	Task interface {
		Do(ctx context.Context) (task.Status, error)

		// ID returns the Task ID
		ID() int
		// Type returns the Task type
		Type() string
		// Status returns the Task status
		Status() task.Status
		// Owner returns the Task owner
		Owner() *userpb.User
		// State returns the external Task state
		State() string
		// ShouldPersist returns true if the Task should be persisted into DB
		ShouldPersist() bool
		// Persisted returns true if the Task is persisted in DB
		Persisted() bool
		// Executed returns the duration of the Task execution
		Executed() time.Duration
		// Retried returns the number of times the Task has been retried
		Retried() int
		// Error returns the error of the Task
		Error() error
		// ErrorHistory returns the error history of the Task
		ErrorHistory() []error
		// Model returns the ent model of the Task
		Model() *ent.Task
		// TraceID returns the correlation ID of the Task
		TraceID() string
		// ResumeTime returns the time when the Task is resumed
		ResumeTime() int64
		// ResumeAfter sets the time when the Task should be resumed
		ResumeAfter(next time.Duration)
		Progress(ctx context.Context) *pbexplorer.TaskPhaseProgressResponse
		// Summarize returns the Task summary for UI display
		Summarize(hasher hashid.Encoder) *pbexplorer.Summary
		// OnSuspend is called when queue decides to suspend the Task
		OnSuspend(time int64)
		// OnPersisted is called when the Task is persisted or updated in DB
		OnPersisted(task *ent.Task)
		// OnError is called when the Task encounters an error
		OnError(err error, d time.Duration)
		// OnRetry is called when the iteration returns error and before retry
		OnRetry(err error)
		// OnIterationComplete is called when the one iteration is completed
		OnIterationComplete(executed time.Duration)
		// OnStatusTransition is called when the Task status is changed
		OnStatusTransition(newStatus task.Status)

		// Cleanup is called when the Task is done or error.
		Cleanup(ctx context.Context) error

		Lock()
		Unlock()
	}
	ResumableTaskFactory func(model *ent.Task) Task
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

	stateTransition func(ctx context.Context, task Task, newStatus task.Status, q *queue) error
)

var (
	taskFactories sync.Map
)

const (
	MediaMetaTaskType             = "media_meta"
	EntityRecycleRoutineTaskType  = "entity_recycle_routine"
	ExplicitEntityRecycleTaskType = "explicit_entity_recycle"
	UploadSentinelCheckTaskType   = "upload_sentinel_check"
	CreateArchiveTaskType         = "create_archive"
	ExtractArchiveTaskType        = "extract_archive"
	RelocateTaskType              = "relocate"
	RemoteDownloadTaskType        = "remote_download"
	ImportTaskType                = "import"

	SlaveCreateArchiveTaskType = "slave_create_archive"
	SlaveUploadTaskType        = "slave_upload"
	SlaveExtractArchiveType    = "slave_extract_archive"

	FullTextIndexTaskType       = "full_text_index"
	FullTextCopyTaskType        = "full_text_copy"
	FullTextChangeOwnerTaskType = "full_text_change_owner"
	FullTextDeleteTaskType      = "full_text_delete"
	FullTextRebuildTaskType     = "full_text_rebuild"
)

func init() {
	gob.Register(Progresses{})
}

// RegisterResumableTaskFactory registers a resumable Task factory
func RegisterResumableTaskFactory(taskType string, factory ResumableTaskFactory) {
	taskFactories.Store(taskType, factory)
}

// NewTaskFromModel creates a Task from ent.Task model
func NewTaskFromModel(model *ent.Task) (Task, error) {
	if factory, ok := taskFactories.Load(model.Type); ok {
		return factory.(ResumableTaskFactory)(model), nil
	}

	return nil, fmt.Errorf("unknown Task type: %s", model.Type)
}

// InMemoryTask implements part Task interface using in-memory constants.
type InMemoryTask struct {
	*DBTask
}

func (i *InMemoryTask) ShouldPersist() bool {
	return false
}

func (t *InMemoryTask) OnStatusTransition(newStatus task.Status) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Task != nil {
		t.Task.Status = newStatus
	}
}

// DBTask implements Task interface related to DB schema
type DBTask struct {
	DirectOwner *userpb.User
	Task        *ent.Task

	mu sync.Mutex
}

func (t *DBTask) ID() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Task != nil {
		return t.Task.ID
	}
	return 0
}

func (t *DBTask) Status() task.Status {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Task != nil {
		return t.Task.Status
	}
	return ""
}

func (t *DBTask) Type() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Task.Type
}

func (t *DBTask) Owner() *userpb.User {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.DirectOwner != nil {
		return t.DirectOwner
	}
	if t.Task != nil {
		return &userpb.User{Id: int64(t.Task.UserID)}
	}
	return nil
}

func (t *DBTask) State() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Task != nil {
		return t.Task.PrivateState
	}
	return ""
}

func (t *DBTask) Persisted() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.Task != nil && t.Task.ID != 0
}

func (t *DBTask) Executed() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Task != nil {
		return t.Task.PublicState.ExecutedDuration.AsDuration()
	}
	return 0
}

func (t *DBTask) Retried() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Task != nil {
		return int(t.Task.PublicState.RetryCount)
	}
	return 0
}

func (t *DBTask) Error() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Task != nil && t.Task.PublicState.Error != "" {
		return errors.New(t.Task.PublicState.Error)
	}

	return nil
}

func (t *DBTask) ErrorHistory() []error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Task != nil {
		return lo.Map(t.Task.PublicState.ErrorHistory, func(err string, index int) error {
			return errors.New(err)
		})
	}

	return nil
}

func (t *DBTask) Model() *ent.Task {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Task
}

func (t *DBTask) Cleanup(ctx context.Context) error {
	return nil
}

func (t *DBTask) TraceID() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Task != nil {
		return t.Task.TraceID
	}
	return ""
}

func (t *DBTask) ShouldPersist() bool {
	return true
}

func (t *DBTask) OnPersisted(task *ent.Task) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Task = task
}

func (t *DBTask) OnError(err error, d time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Task != nil {
		t.Task.PublicState.Error = err.Error()
		taskDuration := t.Task.PublicState.ExecutedDuration.AsDuration()
		t.Task.PublicState.ExecutedDuration = durationpb.New(taskDuration + d)
	}
}

func (t *DBTask) OnRetry(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Task != nil {
		if t.Task.PublicState.ErrorHistory == nil {
			t.Task.PublicState.ErrorHistory = make([]string, 0)
		}

		t.Task.PublicState.ErrorHistory = append(t.Task.PublicState.ErrorHistory, err.Error())
		t.Task.PublicState.RetryCount++
	}
}

func (t *DBTask) OnIterationComplete(d time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Task != nil {
		taskDuration := t.Task.PublicState.ExecutedDuration.AsDuration()
		t.Task.PublicState.ExecutedDuration = durationpb.New(taskDuration + d)
	}
}

func (t *DBTask) ResumeTime() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Task != nil {
		return t.Task.PublicState.ResumeTime
	}
	return 0
}

func (t *DBTask) OnSuspend(time int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Task != nil {
		t.Task.PublicState.ResumeTime = time
	}
}

func (t *DBTask) Progress(ctx context.Context) *pbexplorer.TaskPhaseProgressResponse {
	return nil
}

func (t *DBTask) OnStatusTransition(newStatus task.Status) {
	// Nop
}

func (t *DBTask) Lock() {
	t.mu.Lock()
}

func (t *DBTask) Unlock() {
	t.mu.Unlock()
}

func (t *DBTask) Summarize(hasher hashid.Encoder) *pbexplorer.Summary {
	return &pbexplorer.Summary{}
}

func (t *DBTask) ResumeAfter(next time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Task != nil {
		t.Task.PublicState.ResumeTime = time.Now().Add(next).Unix()
	}
}

var stateTransitions map[task.Status]map[task.Status]stateTransition

func init() {
	stateTransitions = map[task.Status]map[task.Status]stateTransition{
		"": {
			task.StatusQueued: persistTask,
		},
		task.StatusQueued: {
			task.StatusProcessing: func(ctx context.Context, task Task, newStatus task.Status, q *queue) error {
				if err := persistTask(ctx, task, newStatus, q); err != nil {
					return err
				}
				return nil
			},
			task.StatusQueued: func(ctx context.Context, task Task, newStatus task.Status, q *queue) error {
				return nil
			},
			task.StatusError: func(ctx context.Context, task Task, newStatus task.Status, q *queue) error {
				q.metric.IncFailureTask()
				return persistTask(ctx, task, newStatus, q)
			},
		},
		task.StatusProcessing: {
			task.StatusQueued: persistTask,
			task.StatusCompleted: func(ctx context.Context, task Task, newStatus task.Status, q *queue) error {
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
			task.StatusError: func(ctx context.Context, task Task, newStatus task.Status, q *queue) error {
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
			task.StatusCanceled: func(ctx context.Context, task Task, newStatus task.Status, q *queue) error {
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
			task.StatusProcessing: persistTask,
			task.StatusSuspending: func(ctx context.Context, task Task, newStatus task.Status, q *queue) error {
				q.metric.IncSuspendingTask()
				if err := persistTask(ctx, task, newStatus, q); err != nil {
					return err
				}
				q.logger.Info("Task %d suspended, resume time: %d", task.ID(), task.ResumeTime())
				return q.QueueTask(ctx, task)
			},
		},
		task.StatusSuspending: {
			task.StatusProcessing: func(ctx context.Context, task Task, newStatus task.Status, q *queue) error {
				q.metric.DecSuspendingTask()
				return persistTask(ctx, task, newStatus, q)
			},
			task.StatusError: func(ctx context.Context, task Task, newStatus task.Status, q *queue) error {
				q.metric.IncFailureTask()
				return persistTask(ctx, task, newStatus, q)
			},
		},
	}

}

func persistTask(ctx context.Context, task Task, newState task.Status, q *queue) error {
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

func saveTaskToInventory(ctx context.Context, task Task, newStatus task.Status, q *queue) error {
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

	args := &data.TaskArgs{
		Status: newStatus,
		Type:   task.Type(),
		PublicState: &pb.TaskPublicState{
			RetryCount:       int32(task.Retried()),
			ExecutedDuration: durationpb.New(task.Executed()),
			ErrorHistory:     errHistory,
			Error:            errStr,
			ResumeTime:       task.ResumeTime(),
		},
		PrivateState: task.State(),
		OwnerID:      int(task.Owner().Id),
		TraceID:      util.TraceID(ctx),
	}

	var (
		res *ent.Task
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

type DBFileTask struct {
	*DBTask
}
