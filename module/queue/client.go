package queue

import (
	"context"
)

// TaskRecord 是 queue 包内部对"持久化任务记录"的抽象，
// 不依赖任何 ent 类型，各服务自行实现。
type TaskRecord interface {
	// ID returns the Task ID
	ID() int
	// Type returns the Task type
	Type() string
	// Status returns the Task status
	Status() TaskStatus
	// OwnerID returns the Task owner ID
	OwnerID() int
	// State returns the internal Task state
	State() string
	// Retried returns the number of times the Task has been retried
	Retried() int
	// Error returns the error of the Task
	Error() error
	// ErrorHistory returns the error history of the Task
	ErrorHistory() []error
	// TraceID returns the correlation ID of the Task
	TraceID() string
	// ResumeTime returns the time when the Task is resumed
	ResumeTime() int64
}

// TaskClient 是 queue 对持久化层的唯一依赖，
// 各服务注入自己的 ent 实现。
type TaskClient interface {
	// New creates a new task with the given args.
	New(ctx context.Context, args *TaskArgs) (TaskRecord, error)
	// Update updates the task with the given args.
	Update(ctx context.Context, record TaskRecord, args *TaskArgs) (TaskRecord, error)
	// GetPendingTasks returns all pending tasks of given type.
	GetPendingTasks(ctx context.Context, types ...string) ([]TaskRecord, error)
}
