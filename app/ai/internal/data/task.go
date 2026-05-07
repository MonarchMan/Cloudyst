package data

import (
	"ai/ent"
	"ai/ent/task"
	"api/external/data/common"
	"common/hashid"
	"context"
	"errors"
	"fmt"
	"queue"
	"time"

	"github.com/samber/lo"
)

type TaskModel struct {
	*ent.Task
}

func NewTaskModel(task *ent.Task) *TaskModel {
	return &TaskModel{
		Task: task,
	}
}

func (t *TaskModel) ID() int {
	return t.Task.ID
}

func (t *TaskModel) Type() string {
	return t.Task.Type
}

func (t *TaskModel) Status() queue.TaskStatus {
	return t.Task.Status
}

func (t *TaskModel) OwnerID() int {
	return t.Task.UserID
}

func (t *TaskModel) State() string {
	return t.PrivateState
}

func (t *TaskModel) Retried() int {
	return t.Task.PublicState.RetryCount
}

func (t *TaskModel) Error() error {
	if t.Task != nil && t.Task.PublicState.Error != "" {
		return errors.New(t.Task.PublicState.Error)
	}
	return nil
}

func (t *TaskModel) ErrorHistory() []error {
	if t.Task != nil {
		return lo.Map(t.Task.PublicState.ErrorHistory, func(err string, index int) error {
			return errors.New(err)
		})
	}

	return nil
}

func (t *TaskModel) TraceID() string {
	return t.Task.TraceID
}

func (t *TaskModel) ResumeTime() int64 {
	return t.Task.PublicState.ResumeTime
}

type TaskClient interface {
	TxOperator
	queue.TaskClient
	// GetTaskByID returns the task with the given ID.
	GetTaskByID(ctx context.Context, taskID int) (*ent.Task, error)
	// SetCompleteByID sets the task with the given ID to complete.
	SetCompleteByID(ctx context.Context, taskID int) error
	// List returns a list of tasks with the given args.
	List(ctx context.Context, args *ListTaskArgs) (*ListTaskResult, error)
	// DeleteByIDs deletes the tasks with the given IDs.
	DeleteByIDs(ctx context.Context, ids ...int) error
	// DeleteBy deletes the tasks with the given args.
	DeleteBy(ctx context.Context, args *DeleteTaskArgs) error
	// CancelTasks marks cancelable tasks as canceled.
	CancelTasks(ctx context.Context, ids []int, taskType string, ownerID int) ([]*ent.Task, error)
	// ResumeTasks resumes the tasks with the given IDs.
	ResumeTasks(ctx context.Context, ids []int, taskType string, ownerID int) ([]*ent.Task, error)
}

type (
	ListTaskArgs struct {
		*common.PaginationArgs
		IDs     []int
		Types   []string
		Status  []queue.TaskStatus
		UserID  int
		TraceID string
	}

	ListTaskResult struct {
		*common.PaginationResults
		Tasks []*ent.Task
	}

	DeleteTaskArgs struct {
		NotAfter time.Time
		Types    []string
		Status   []queue.TaskStatus
		Uids     []int
	}
)

func NewTaskClient(client *ent.Client, dbType common.DBType, hasher hashid.Encoder) TaskClient {
	return &taskClient{client: client, maxSQlParam: common.SqlParamLimit(dbType), hasher: hasher}
}

type taskClient struct {
	maxSQlParam int
	hasher      hashid.Encoder
	client      *ent.Client
}

func (c *taskClient) SetClient(newClient *ent.Client) TxOperator {
	return &taskClient{client: newClient, maxSQlParam: c.maxSQlParam, hasher: c.hasher}
}

func (c *taskClient) GetClient() *ent.Client {
	return c.client
}

func (c *taskClient) New(ctx context.Context, task *queue.TaskArgs) (queue.TaskRecord, error) {
	stm := c.client.Task.
		Create().
		SetType(task.Type).
		SetPublicState(task.PublicState)
	if task.PrivateState != "" {
		stm.SetPrivateState(task.PrivateState)
	}

	if task.OwnerID != 0 {
		stm.SetUserID(task.OwnerID)
	}

	if task.Status != "" {
		stm.SetStatus(task.Status)
	}

	if task.TraceID != "" {
		stm.SetTraceID(task.TraceID)
	}

	newTask, err := stm.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	return NewTaskModel(newTask), nil
}

func (c *taskClient) DeleteByIDs(ctx context.Context, ids ...int) error {
	_, err := c.client.Task.Delete().Where(task.IDIn(ids...)).Exec(ctx)
	return err
}

func (c *taskClient) DeleteBy(ctx context.Context, args *DeleteTaskArgs) error {
	query := c.client.Task.
		Delete().
		Where(task.CreatedAtLTE(args.NotAfter))

	if len(args.Status) > 0 {
		query.Where(task.StatusIn(args.Status...))
	}

	if len(args.Types) > 0 {
		query.Where(task.TypeIn(args.Types...))
	}

	if len(args.Uids) > 0 {
		query.Where(task.UserIDIn(args.Uids...))
	}

	_, err := query.Exec(ctx)
	return err
}

func (c *taskClient) Update(ctx context.Context, taskModel queue.TaskRecord, args *queue.TaskArgs) (queue.TaskRecord, error) {
	wrapped, ok := taskModel.(*TaskModel)
	if !ok {
		return nil, fmt.Errorf("taskModel is not defined as ent.Task")
	}
	t := wrapped.Task
	stm := c.client.Task.UpdateOne(t).
		SetPublicState(args.PublicState)

	t.PublicState = args.PublicState

	if args.PrivateState != "" {
		stm.SetPrivateState(args.PrivateState)
		t.PrivateState = args.PrivateState
	}

	if t.Status != "" {
		stm.SetStatus(args.Status)
		t.Status = args.Status
	}

	if err := stm.Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	return taskModel, nil
}

func (c *taskClient) GetPendingTasks(ctx context.Context, taskType ...string) ([]queue.TaskRecord, error) {
	tasks, err := c.client.Task.Query().
		Where(task.StatusIn(queue.StatusProcessing, queue.StatusQueued, queue.StatusSuspending)).
		Where(task.TypeIn(taskType...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	records := lo.Map(tasks, func(task *ent.Task, _ int) queue.TaskRecord {
		return NewTaskModel(task)
	})

	return records, nil
}

func (c *taskClient) GetTaskByID(ctx context.Context, taskID int) (*ent.Task, error) {
	return c.client.Task.Query().
		Where(task.ID(taskID)).
		First(ctx)
}

func (c *taskClient) SetCompleteByID(ctx context.Context, taskID int) error {
	_, err := c.client.Task.UpdateOneID(taskID).
		SetStatus(queue.StatusCompleted).
		Save(ctx)
	return err
}

func (c *taskClient) List(ctx context.Context, args *ListTaskArgs) (*ListTaskResult, error) {
	q := c.client.Task.Query()
	if len(args.IDs) > 0 {
		q.Where(task.IDIn(args.IDs...))
	}

	if args.UserID != 0 {
		q.Where(task.UserID(args.UserID))
	}

	if args.Types != nil {
		q.Where(task.TypeIn(args.Types...))
	}

	if args.Status != nil {
		q.Where(task.StatusIn(args.Status...))
	}

	if args.TraceID != "" {
		q.Where(task.TraceID(args.TraceID))
	}

	pageSize := common.CapPageSize(c.maxSQlParam, args.PageSize, 1)
	q.Order(getTaskOrderOption(args)...)
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, err
	}
	res, err := q.Limit(pageSize).Offset(args.Page * args.PageSize).All(ctx)
	if err != nil {
		return nil, err
	}

	return &ListTaskResult{
		Tasks: res,
		PaginationResults: &common.PaginationResults{
			Page:       args.Page,
			PageSize:   pageSize,
			TotalItems: total,
		},
	}, nil
}

func (c *taskClient) CancelTasks(ctx context.Context, ids []int, taskType string, ownerID int) ([]*ent.Task, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	q := c.client.Task.Query().
		Where(task.IDIn(ids...), task.Type(taskType)).
		Where(task.StatusIn(queue.StatusQueued, queue.StatusProcessing, queue.StatusSuspending, queue.StatusCanceled))
	if ownerID != 0 {
		q.Where(task.UserID(ownerID))
	}

	tasks, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		if t.Status == queue.StatusCanceled {
			continue
		}
		if _, err = c.client.Task.UpdateOneID(t.ID).
			SetStatus(queue.StatusCanceled).
			Save(ctx); err != nil {
			return nil, err
		}
		t.Status = queue.StatusCanceled
	}
	return tasks, nil
}

func (c *taskClient) ResumeTasks(ctx context.Context, ids []int, taskType string, ownerID int) ([]*ent.Task, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	q := c.client.Task.Query().
		Where(task.IDIn(ids...), task.Type(taskType)).
		Where(task.StatusIn(queue.StatusCanceled, queue.StatusSuspending, queue.StatusQueued, queue.StatusProcessing))
	if ownerID != 0 {
		q.Where(task.UserID(ownerID))
	}

	tasks, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		if t.Status != queue.StatusCanceled {
			continue
		}
		if _, err = c.client.Task.UpdateOneID(t.ID).
			SetStatus(queue.StatusSuspending).
			Save(ctx); err != nil {
			return nil, err
		}
		t.Status = queue.StatusSuspending
	}
	return tasks, nil
}

func getTaskOrderOption(args *ListTaskArgs) []task.OrderOption {
	orderTerm := common.GetOrderTerm(args.OrderDir)
	switch args.OrderBy {
	default:
		return []task.OrderOption{task.ByID(orderTerm)}
	}
}
