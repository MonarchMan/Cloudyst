package data

import (
	"api/external/data/common"
	"common/hashid"
	"context"
	"errors"
	"file/ent"
	"file/ent/task"
	"fmt"
	"queue"
	"time"

	"entgo.io/ent/dialect/sql"
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
}

type (
	ListTaskArgs struct {
		*PaginationArgs
		Types   []string
		Status  []queue.TaskStatus
		UserID  int
		TraceID string
	}

	ListTaskResult struct {
		*PaginationResults
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

	if t.PrivateState != "" {
		stm.SetPrivateState(t.PrivateState)
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

	var (
		tasks         []*ent.Task
		err           error
		paginationRes *PaginationResults
	)

	if args.UseCursorPagination {
		tasks, paginationRes, err = c.cursorPagination(ctx, q, args, 1)
	} else {
		tasks, paginationRes, err = c.offsetPagination(ctx, q, args, 1)
	}

	if err != nil {
		return nil, fmt.Errorf("query failed with paginiation: %w", err)
	}

	return &ListTaskResult{
		Tasks:             tasks,
		PaginationResults: paginationRes,
	}, nil
}

func (c *taskClient) cursorPagination(ctx context.Context, query *ent.TaskQuery, args *ListTaskArgs,
	paramMargin int) ([]*ent.Task, *PaginationResults, error) {
	pageSize := capPageSize(c.maxSQlParam, args.PageSize, paramMargin)
	query.Order(task.ByID(sql.OrderDesc()))

	var (
		pageToken  *PageToken
		err        error
		queryPaged = query
	)
	if args.PageToken != "" {
		pageToken, err = pageTokenFromString(args.PageToken, c.hasher, hashid.TaskID)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid page token %q: %w", args.PageToken, err)
		}

		queryPaged = query.Where(task.IDLT(pageToken.ID))
	}

	// Use page size + 1 to determine if there are more items to come
	queryPaged.Limit(pageSize + 1)

	tasks, err := queryPaged.
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	// More items to come
	nextTokenStr := ""
	if len(tasks) > pageSize {
		lastItem := tasks[len(tasks)-2]
		nextToken, err := getTaskNextPageToken(c.hasher, lastItem)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate next page token: %w", err)
		}

		nextTokenStr = nextToken
	}

	return lo.Subset(tasks, 0, uint(pageSize)), &PaginationResults{
		PageSize:      pageSize,
		NextPageToken: nextTokenStr,
		IsCursor:      true,
	}, nil
}

func (c *taskClient) offsetPagination(ctx context.Context, query *ent.TaskQuery, args *ListTaskArgs,
	paramMargin int) ([]*ent.Task, *PaginationResults, error) {
	pageSize := capPageSize(c.maxSQlParam, args.PageSize, paramMargin)
	query.Order(getTaskOrderOption(args)...)

	// Count total items
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	logs, err := query.
		Limit(pageSize).
		Offset(args.Page * args.PageSize).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	return logs, &PaginationResults{
		PageSize:   pageSize,
		TotalItems: total,
		Page:       args.Page,
	}, nil

}

func getTaskOrderOption(args *ListTaskArgs) []task.OrderOption {
	orderTerm := getOrderTerm(args.Order)
	switch args.OrderBy {
	default:
		return []task.OrderOption{task.ByID(orderTerm)}
	}
}

// getTaskNextPageToken returns the next page token for the given last task.
func getTaskNextPageToken(hasher hashid.Encoder, last *ent.Task) (string, error) {
	token := &PageToken{
		ID: last.ID,
	}

	return token.Encode(hasher, hashid.EncodeTaskID)
}
