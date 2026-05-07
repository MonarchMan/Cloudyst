package queue

import (
	"ai/ent"
	"ai/internal/data"
	"api/external/data/userdata"
	"common/hashid"
	"context"
	"errors"
	"queue"
	"sync"
	"time"

	"github.com/samber/lo"
)

const (
	IngestTaskType   = "rag_ingest"
	RetrieveTaskType = "rag_retrieve"
	ReindexTaskType  = "rag_reindex"
)

// DBTask implements Task interface related to DB schema
type DBTask struct {
	DirectOwner *userdata.User
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

func (t *DBTask) Status() queue.TaskStatus {
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

func (t *DBTask) Owner() *userdata.User {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.DirectOwner != nil {
		return t.DirectOwner
	}
	if t.Task != nil {
		return &userdata.User{ID: t.Task.UserID}
	}
	return nil
}

func (t *DBTask) OwnerID() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.DirectOwner != nil {
		return t.DirectOwner.ID
	}
	if t.Task != nil {
		return t.Task.UserID
	}
	return 0
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
		return t.Task.PublicState.ExecutedDuration
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

func (t *DBTask) Model() queue.TaskRecord {
	t.mu.Lock()
	defer t.mu.Unlock()
	return data.NewTaskModel(t.Task)
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

func (t *DBTask) CanCancel() bool {
	return true
}

func (t *DBTask) OnPersisted(taskModel queue.TaskRecord) {
	wrapped, ok := taskModel.(*data.TaskModel)
	if !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Task = wrapped.Task
}

func (t *DBTask) OnError(err error, d time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Task != nil {
		t.Task.PublicState.Error = err.Error()
		t.Task.PublicState.ExecutedDuration = d
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
		t.Task.PublicState.ExecutedDuration += d
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

func (t *DBTask) Progress(ctx context.Context) queue.Progresses {
	return nil
}

func (t *DBTask) OnStatusTransition(newStatus queue.TaskStatus) {
	// Nop
}

func (t *DBTask) Lock() {
	t.mu.Lock()
}

func (t *DBTask) Unlock() {
	t.mu.Unlock()
}

func (t *DBTask) Summarize(hasher hashid.Encoder) *queue.Summary {
	return &queue.Summary{}
}

func (t *DBTask) ResumeAfter(next time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Task != nil {
		t.Task.PublicState.ResumeTime = time.Now().Add(next).Unix()
	}
}
