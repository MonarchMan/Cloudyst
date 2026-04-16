package data

import (
	"ai/ent"
	"ai/ent/aichatrole"
	pb "api/api/common/v1"
	"common/db"
	"context"
	"entmodule"
	"fmt"
)

type (
	RoleClient interface {
		TxOperator
		GetByID(ctx context.Context, id int) (*ent.AiChatRole, error)
		GetByIDs(ctx context.Context, ids []int) ([]*ent.AiChatRole, error)
		GetActiveByID(ctx context.Context, id int) (*ent.AiChatRole, error)
		List(ctx context.Context, args *ListChatRoleArgs) (*ListChatRoleResult, error)
		Upsert(ctx context.Context, role *ent.AiChatRole) (*ent.AiChatRole, error)
		Delete(ctx context.Context, id int) error
		BatchDelete(ctx context.Context, ids []int) (int, error)
		DeleteByUserID(ctx context.Context, id int) (int, error)
	}

	roleClient struct {
		maxSQLParam int
		client      *ent.Client
	}

	ListChatRoleArgs struct {
		*pb.PaginationArgs
		Name         string
		UserID       int
		PublicStatus bool
		Category     string
		Status       entmodule.Status
	}

	ListChatRoleResult struct {
		PaginationResults *pb.PaginationResults
		Roles             []*ent.AiChatRole
	}
)

func NewChatRoleClient(client *ent.Client, dbType db.DBType) RoleClient {
	return &roleClient{maxSQLParam: db.SqlParamLimit(dbType), client: client}
}

func (c *roleClient) SetClient(newClient *ent.Client) TxOperator {
	return &roleClient{client: newClient, maxSQLParam: c.maxSQLParam}
}

func (c *roleClient) GetClient() *ent.Client {
	return c.client
}

func (c *roleClient) GetByID(ctx context.Context, id int) (*ent.AiChatRole, error) {
	return c.client.AiChatRole.Get(ctx, id)
}

func (c *roleClient) GetByIDs(ctx context.Context, ids []int) ([]*ent.AiChatRole, error) {
	return c.client.AiChatRole.Query().Where(aichatrole.IDIn(ids...)).All(ctx)
}

func (c *roleClient) GetActiveByID(ctx context.Context, id int) (*ent.AiChatRole, error) {
	role, err := c.client.AiChatRole.Query().
		Where(aichatrole.ID(id), aichatrole.StatusEQ(entmodule.StatusActive)).
		First(ctx)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, fmt.Errorf("role not found")
	}
	if role.Status != entmodule.StatusActive {
		return nil, fmt.Errorf("role is not active")
	}
	return role, nil
}

func (c *roleClient) List(ctx context.Context, args *ListChatRoleArgs) (*ListChatRoleResult, error) {
	q := c.client.AiChatRole.Query()
	if args.Name != "" {
		q.Where(aichatrole.NameContainsFold(args.Name))
	}

	if args.UserID != 0 {
		q.Where(aichatrole.UserIDIn(args.UserID))
	}

	if args.PublicStatus {
		q.Where(aichatrole.PublicStatusEQ(true))
	}

	if args.Category != "" {
		q.Where(aichatrole.Category(args.Category))
	}

	if args.Status != "" {
		q.Where(aichatrole.StatusEQ(args.Status))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, err
	}
	q.Order(getChatRoleOrderOption(args)...)
	roles, err := q.Limit(int(args.PageSize)).Offset(int(args.PageSize * args.Page)).All(ctx)
	if err != nil {
		return nil, err
	}

	return &ListChatRoleResult{
		PaginationResults: &pb.PaginationResults{
			TotalItems: int32(total),
			Page:       args.Page,
			PageSize:   args.PageSize,
		},
		Roles: roles,
	}, nil
}

func (c *roleClient) Upsert(ctx context.Context, role *ent.AiChatRole) (*ent.AiChatRole, error) {
	if role.ID == 0 {
		q := c.client.AiChatRole.Create().
			SetName(role.Name).
			SetPublicStatus(role.PublicStatus).
			SetCategory(role.Category).
			SetStatus(role.Status).
			SetAvatar(role.Avatar).
			SetDescription(role.Description).
			SetSort(role.Sort).
			SetUserID(role.UserID).
			SetSystemMessage(role.SystemMessage).
			SetKnowledgeIds(role.KnowledgeIds).
			SetToolIds(role.ToolIds).
			SetMcpClientNames(role.McpClientNames)
		return q.Save(ctx)
	}
	q := c.client.AiChatRole.UpdateOneID(role.ID).
		SetName(role.Name).
		SetPublicStatus(role.PublicStatus).
		SetStatus(role.Status).
		SetDescription(role.Description).
		SetSort(role.Sort).
		SetSystemMessage(role.SystemMessage).
		SetKnowledgeIds(role.KnowledgeIds).
		SetToolIds(role.ToolIds).
		SetMcpClientNames(role.McpClientNames)

	if role.Avatar != "" {
		q = q.SetAvatar(role.Avatar)
	}

	if role.Category != "" {
		q = q.SetCategory(role.Category)
	}

	return q.Save(ctx)
}

func (c *roleClient) Delete(ctx context.Context, id int) error {
	return c.client.AiChatRole.DeleteOneID(id).Exec(ctx)
}

func (c *roleClient) BatchDelete(ctx context.Context, ids []int) (int, error) {
	return c.client.AiChatRole.Delete().Where(aichatrole.IDIn(ids...)).Exec(ctx)
}

func (c *roleClient) DeleteByUserID(ctx context.Context, id int) (int, error) {
	return c.client.AiChatRole.Delete().Where(aichatrole.UserIDIn(id)).Exec(ctx)
}

func getChatRoleOrderOption(args *ListChatRoleArgs) []aichatrole.OrderOption {
	orderTerm := db.GetOrderTerm(db.OrderDirection(args.OrderDirection))
	switch args.OrderBy {
	case aichatrole.FieldCreatedAt:
		return []aichatrole.OrderOption{aichatrole.ByCreatedAt(orderTerm), aichatrole.ByID(orderTerm)}
	case aichatrole.FieldSort:
		return []aichatrole.OrderOption{aichatrole.BySort(orderTerm), aichatrole.ByID(orderTerm)}
	default:
		return []aichatrole.OrderOption{aichatrole.ByID(orderTerm)}
	}
}
