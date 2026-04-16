package data

import (
	"ai/ent"
	"ai/ent/aitool"
	pb "api/api/common/v1"
	"common/db"
	"context"
	"entmodule"
)

type (
	ToolClient interface {
		TxOperator
		GetByID(ctx context.Context, id int) (*ent.AiTool, error)
		GetByIDs(ctx context.Context, ids []int) ([]*ent.AiTool, error)
		GetActiveByID(ctx context.Context, id int) (*ent.AiTool, error)
		List(ctx context.Context, args *ListToolArgs) (*ListToolResult, error)
		Upsert(ctx context.Context, tool *ent.AiTool) (*ent.AiTool, error)
		Delete(ctx context.Context, id int) error
		BatchDelete(ctx context.Context, ids []int) (int, error)
	}
	toolClient struct {
		maxSQLParam int
		client      *ent.Client
	}

	ListToolArgs struct {
		*pb.PaginationArgs
		Name   string
		Type   string
		Status entmodule.Status
	}

	ListToolResult struct {
		*pb.PaginationResults
		Tools []*ent.AiTool
	}
)

func NewToolClient(client *ent.Client, dbType db.DBType) ToolClient {
	return &toolClient{client: client, maxSQLParam: db.SqlParamLimit(dbType)}
}

func (c *toolClient) SetClient(newClient *ent.Client) TxOperator {
	return &toolClient{client: newClient, maxSQLParam: c.maxSQLParam}
}

func (c *toolClient) GetClient() *ent.Client {
	return c.client
}

func (c *toolClient) GetByID(ctx context.Context, id int) (*ent.AiTool, error) {
	return c.client.AiTool.Get(ctx, id)
}

func (c *toolClient) GetByIDs(ctx context.Context, ids []int) ([]*ent.AiTool, error) {
	return c.client.AiTool.Query().Where(aitool.IDIn(ids...)).All(ctx)
}

func (c *toolClient) GetActiveByID(ctx context.Context, id int) (*ent.AiTool, error) {
	return c.client.AiTool.Query().
		Where(aitool.ID(id), aitool.StatusEQ(entmodule.StatusActive)).
		First(ctx)
}

func (c *toolClient) List(ctx context.Context, args *ListToolArgs) (*ListToolResult, error) {
	pageSize := db.CapPageSize(c.maxSQLParam, int(args.PageSize), 10)
	q := c.client.AiTool.Query()
	if args.Name != "" {
		q.Where(aitool.NameContainsFold(args.Name))
	}

	if args.Type != "" {
		q.Where(aitool.Type(args.Type))
	}

	if args.Status != "" {
		q.Where(aitool.StatusEQ(args.Status))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, err
	}

	q.Order(getToolOrderOption(args)...)
	tools, err := q.Limit(int(args.PageSize)).Offset(pageSize * int(args.Page)).All(ctx)
	if err != nil {
		return nil, err
	}

	return &ListToolResult{
		PaginationResults: &pb.PaginationResults{
			TotalItems: int32(total),
			Page:       args.Page,
			PageSize:   int32(pageSize),
		},
		Tools: tools,
	}, nil
}

func (c *toolClient) Upsert(ctx context.Context, tool *ent.AiTool) (*ent.AiTool, error) {
	if tool.ID == 0 {
		q := c.client.AiTool.Create().
			SetName(tool.Name).
			SetDescription(tool.Description).
			SetType(tool.Type).
			SetStatus(tool.Status).
			SetParameters(tool.Parameters)
		return q.Save(ctx)
	}

	q := c.client.AiTool.UpdateOneID(tool.ID).
		SetName(tool.Name)
	if tool.Description != "" {
		q.SetDescription(tool.Description)
	}
	if tool.Type != "" {
		q.SetType(tool.Type)
	}
	if tool.Status != "" {
		q.SetStatus(tool.Status)
	}
	if tool.Parameters != "" {
		q.SetParameters(tool.Parameters)
	}
	return q.Save(ctx)
}

func (c *toolClient) Delete(ctx context.Context, id int) error {
	return c.client.AiTool.DeleteOneID(id).Exec(ctx)
}

func (c *toolClient) BatchDelete(ctx context.Context, ids []int) (int, error) {
	return c.client.AiTool.Delete().Where(aitool.IDIn(ids...)).Exec(ctx)
}

func getToolOrderOption(args *ListToolArgs) []aitool.OrderOption {
	orderTerm := db.GetOrderTerm(db.OrderDirection(args.OrderDirection))
	switch args.OrderBy {
	case aitool.FieldUpdatedAt:
		return []aitool.OrderOption{aitool.ByUpdatedAt(orderTerm), aitool.ByID(orderTerm)}
	default:
		return []aitool.OrderOption{aitool.ByID(orderTerm)}
	}
}
