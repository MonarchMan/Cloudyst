package data

import (
	"ai/ent"
	"ai/ent/aichatconversation"
	"common/db"
	"context"
	"fmt"
	"time"
)

type (
	ChatConversationClient interface {
		TxOperator
		GetByID(ctx context.Context, id int) (*ent.AiChatConversation, error)
		GetByIDs(ctx context.Context, ids []int) ([]*ent.AiChatConversation, error)
		ListByUserID(ctx context.Context, id int) ([]*ent.AiChatConversation, error)
		List(ctx context.Context, args *ListChatConversationArgs) (*ListChatConversationResult, error)
		Upsert(ctx context.Context, conversation *UpsertChatConversationParams) (*ent.AiChatConversation, error)
		UpdateModel(ctx context.Context, id int, modelID int, model string) (*ent.AiChatConversation, error)
		Delete(ctx context.Context, id int) error
		DeleteUnpinnedByUserID(ctx context.Context, uid int) (int, error)
		BatchDelete(ctx context.Context, ids []int) (int, error)
	}

	chatConversationClient struct {
		maxSQLParam int
		client      *ent.Client
	}

	ListChatConversationArgs struct {
		*db.PaginationArgs
		Title   string
		Pinned  bool
		UserID  int
		RoleID  int
		ModelID int
		Start   *time.Time
		End     *time.Time
	}

	ListChatConversationResult struct {
		*db.PaginationResults
		Conversations []*ent.AiChatConversation
	}

	UpsertChatConversationParams struct {
		Existed     *ent.AiChatConversation
		Title       string
		Pinned      bool
		SysMsg      string
		Temperature float64
		MaxTokens   int
		MaxContexts int
		UserID      int
		RoleID      int
		ModelID     int
		Model       string
	}
)

func NewChatConversationClient(client *ent.Client, dbType db.DBType) ChatConversationClient {
	return &chatConversationClient{maxSQLParam: db.SqlParamLimit(dbType), client: client}
}

func (c *chatConversationClient) SetClient(newClient *ent.Client) TxOperator {
	return &chatConversationClient{maxSQLParam: c.maxSQLParam, client: newClient}
}

func (c *chatConversationClient) GetClient() *ent.Client {
	return c.client
}

func (c *chatConversationClient) GetByID(ctx context.Context, id int) (*ent.AiChatConversation, error) {
	chat, err := c.client.AiChatConversation.Query().Where(aichatconversation.ID(id)).First(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get ai chatconversation %d: %w", id, err)
	}
	return chat, nil
}

func (c *chatConversationClient) GetByIDs(ctx context.Context, ids []int) ([]*ent.AiChatConversation, error) {
	chats, err := c.client.AiChatConversation.Query().Where(aichatconversation.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get ai chatconversations %v: %w", ids, err)
	}
	return chats, nil
}

func (c *chatConversationClient) ListByUserID(ctx context.Context, uid int) ([]*ent.AiChatConversation, error) {
	return c.client.AiChatConversation.Query().Where(aichatconversation.UserID(uid)).All(ctx)
}

func (c *chatConversationClient) List(ctx context.Context, args *ListChatConversationArgs) (*ListChatConversationResult, error) {
	q := c.client.AiChatConversation.Query()
	if args.Title != "" {
		q.Where(aichatconversation.TitleContainsFold(args.Title))
	}

	if args.Pinned {
		q.Where(aichatconversation.Pinned(args.Pinned))
	}

	if args.UserID != 0 {
		q.Where(aichatconversation.UserID(args.UserID))
	}

	if args.RoleID != 0 {
		q.Where(aichatconversation.RoleID(args.RoleID))
	}

	if args.ModelID != 0 {
		q.Where(aichatconversation.ModelID(args.ModelID))
	}

	if args.Start != nil {
		q.Where(aichatconversation.CreatedAtGTE(*args.Start))
	}
	if args.End != nil {
		q.Where(aichatconversation.CreatedAtLTE(*args.End))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, err
	}

	q.Order(getChatConversationOrderOption(args)...)
	chats, err := q.Limit(args.PageSize).Offset(args.Page * args.PageSize).All(ctx)
	if err != nil {
		return nil, err
	}

	return &ListChatConversationResult{
		PaginationResults: &db.PaginationResults{
			Page:       args.Page,
			PageSize:   args.PageSize,
			TotalItems: total,
		},
		Conversations: chats,
	}, nil
}

func (c *chatConversationClient) Upsert(ctx context.Context, conversation *UpsertChatConversationParams) (*ent.AiChatConversation, error) {
	if conversation.Existed == nil {
		query := c.client.AiChatConversation.Create().
			SetTitle(conversation.Title).
			SetPinned(conversation.Pinned).
			SetUserID(conversation.UserID).
			SetRoleID(conversation.RoleID).
			SetSystemMessage(conversation.SysMsg).
			SetModelID(conversation.ModelID).
			SetModel(conversation.Model).
			SetTemperature(conversation.Temperature).
			SetMaxTokens(conversation.MaxTokens).
			SetMaxContexts(conversation.MaxContexts)
		return query.Save(ctx)
	}
	query := c.client.AiChatConversation.UpdateOneID(conversation.Existed.ID).
		SetTitle(conversation.Title).
		SetPinned(conversation.Pinned).
		SetUserID(conversation.UserID).
		SetRoleID(conversation.RoleID).
		SetSystemMessage(conversation.SysMsg).
		SetModelID(conversation.ModelID).
		SetModel(conversation.Model)

	if conversation.Temperature >= 0 {
		query.SetTemperature(conversation.Temperature)
	}

	if conversation.MaxTokens >= 0 {
		query.SetMaxTokens(conversation.MaxTokens)
	}

	if conversation.MaxContexts >= 0 {
		query.SetMaxContexts(conversation.MaxContexts)
	}

	return query.Save(ctx)
}

func (c *chatConversationClient) UpdateModel(ctx context.Context, id int, modelID int, model string) (*ent.AiChatConversation, error) {
	return c.client.AiChatConversation.UpdateOneID(id).
		SetModelID(modelID).
		SetModel(model).
		Save(ctx)
}

func (c *chatConversationClient) Delete(ctx context.Context, id int) error {
	return c.client.AiChatConversation.DeleteOneID(id).Exec(ctx)
}

func (c *chatConversationClient) DeleteUnpinnedByUserID(ctx context.Context, uid int) (int, error) {
	return c.client.AiChatConversation.Delete().
		Where(aichatconversation.UserID(uid), aichatconversation.PinnedEQ(false)).
		Exec(ctx)
}

func (c *chatConversationClient) BatchDelete(ctx context.Context, ids []int) (int, error) {
	return c.client.AiChatConversation.Delete().Where(aichatconversation.IDIn(ids...)).Exec(ctx)
}

func getChatConversationOrderOption(args *ListChatConversationArgs) []aichatconversation.OrderOption {
	orderTerm := db.GetOrderTerm(args.OrderDir)
	switch args.OrderBy {
	case aichatconversation.FieldUpdatedAt:
		return []aichatconversation.OrderOption{aichatconversation.ByUpdatedAt(orderTerm), aichatconversation.ByID(orderTerm)}
	default:
		return []aichatconversation.OrderOption{aichatconversation.ByID(orderTerm)}
	}
}
