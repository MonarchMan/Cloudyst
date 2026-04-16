package data

import (
	"ai/ent"
	"ai/ent/aichatmessage"
	"ai/internal/biz/types"
	"common/db"
	"context"
	"fmt"

	"github.com/samber/lo"
)

type (
	LoadMessageWebPage struct{}
	ChatMessageClient  interface {
		TxOperator
		GetByID(ctx context.Context, id int) (*ent.AiChatMessage, error)
		GetByIDs(ctx context.Context, ids []int) ([]*ent.AiChatMessage, error)
		List(ctx context.Context, args *ListChatMessageArgs) (*ListChatMessageResult, error)
		Create(ctx context.Context, message *CreateChatMessageArgs) (*ent.AiChatMessage, error)
		UpdateContent(ctx context.Context, id int, content string, reasoningContent string) (*ent.AiChatMessage, error)
		Delete(ctx context.Context, id int) error
		BatchDelete(ctx context.Context, ids []int) (int, error)
		DeleteByConversationID(ctx context.Context, conversationID int) (int, error)
	}

	chatMessageClient struct {
		client      *ent.Client
		maxSQLParam int
	}

	ListChatMessageArgs struct {
		*db.PaginationArgs
		ConversationID int
		UserID         int
		RoleID         int
		ModelID        int
		Type           string
	}

	ListChatMessageResult struct {
		*db.PaginationResults
		ChatMessages []*ent.AiChatMessage
		PageMap      map[int][]*types.WebPage
	}

	CreateChatMessageArgs struct {
		CID           int
		UserID        int
		RoleID        int
		ModelID       int
		Type          string
		ReplyID       int
		Content       string
		ReasonContent string
		AttachUrls    []string
		WebPages      []*types.WebPage
	}
)

func NewChatMessageClient(client *ent.Client, dbType db.DBType) ChatMessageClient {
	return &chatMessageClient{client: client, maxSQLParam: db.SqlParamLimit(dbType)}
}

func (c *chatMessageClient) SetClient(newClient *ent.Client) TxOperator {
	return &chatMessageClient{client: newClient, maxSQLParam: c.maxSQLParam}
}

func (c *chatMessageClient) GetClient() *ent.Client {
	return c.client
}

func (c *chatMessageClient) GetByID(ctx context.Context, id int) (*ent.AiChatMessage, error) {
	return withMessageEagerLoading(ctx, c.client.AiChatMessage.Query()).Where(aichatmessage.ID(id)).Only(ctx)
}

func (c *chatMessageClient) GetByIDs(ctx context.Context, ids []int) ([]*ent.AiChatMessage, error) {
	return withMessageEagerLoading(ctx, c.client.AiChatMessage.Query()).Where(aichatmessage.IDIn(ids...)).All(ctx)
}

func (c *chatMessageClient) List(ctx context.Context, args *ListChatMessageArgs) (*ListChatMessageResult, error) {
	pageSize := db.CapPageSize(c.maxSQLParam, int(args.PageSize), 10)
	q := c.client.AiChatMessage.Query()
	if args.ConversationID != 0 {
		q.Where(aichatmessage.ConversationID(args.ConversationID))
	}

	if args.UserID != 0 {
		q.Where(aichatmessage.UserID(args.UserID))
	}

	if args.RoleID != 0 {
		q.Where(aichatmessage.RoleID(args.RoleID))
	}

	if args.ModelID != 0 {
		q.Where(aichatmessage.ModelID(args.ModelID))
	}

	if args.Type != "" {
		q.Where(aichatmessage.Type(args.Type))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, err
	}
	q.Order(getChatMessageOrderOption(args)...)
	cms, err := withMessageEagerLoading(ctx, q).Limit(int(args.PageSize)).Offset(int(args.Page * args.PageSize)).All(ctx)
	if err != nil {
		return nil, err
	}

	return &ListChatMessageResult{
		PaginationResults: &db.PaginationResults{
			TotalItems: total,
			Page:       args.Page,
			PageSize:   pageSize,
		},
		ChatMessages: cms,
	}, nil
}

func (c *chatMessageClient) Create(ctx context.Context, args *CreateChatMessageArgs) (*ent.AiChatMessage, error) {
	q := c.client.AiChatMessage.Create().
		SetConversationID(args.CID).
		SetUserID(args.UserID).
		SetRoleID(args.RoleID).
		SetType(args.Type).
		SetModelID(args.ModelID).
		SetReplyID(args.ReplyID).
		SetContent(args.Content).
		SetReasonContent(args.ReasonContent).
		SetAttachmentUrls(args.AttachUrls)

	m, err := q.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to save message: %w", err)
	}

	// 插入网页搜索记录
	if len(args.WebPages) > 0 {
		batch := lo.Map(args.WebPages, func(page *types.WebPage, index int) *ent.AiWebPageCreate {
			return c.client.AiWebPage.Create().
				SetName(page.Name).
				SetIcon(page.Icon).
				SetTitle(page.Title).
				SetURL(page.URL).
				SetSnippet(page.Snippet).
				SetSummary(page.Summary).
				SetMessageID(page.MessageID)
		})
		if err := c.client.AiWebPage.CreateBulk(batch...).Exec(ctx); err != nil {
			return nil, fmt.Errorf("failed to create web pages: %w", err)
		}
	}
	return m, nil
}

func (c *chatMessageClient) UpdateContent(ctx context.Context, id int, content string, reasoningContent string) (*ent.AiChatMessage, error) {
	return c.client.AiChatMessage.UpdateOneID(id).SetContent(content).SetReasonContent(reasoningContent).Save(ctx)
}

func (c *chatMessageClient) Delete(ctx context.Context, id int) error {
	return c.client.AiChatMessage.DeleteOneID(id).Exec(ctx)
}

func (c *chatMessageClient) BatchDelete(ctx context.Context, ids []int) (int, error) {
	return c.client.AiChatMessage.Delete().Where(aichatmessage.IDIn(ids...)).Exec(ctx)
}

func (c *chatMessageClient) DeleteByConversationID(ctx context.Context, conversationID int) (int, error) {
	return c.client.AiChatMessage.Delete().Where(aichatmessage.ConversationID(conversationID)).Exec(ctx)
}

func getChatMessageOrderOption(args *ListChatMessageArgs) []aichatmessage.OrderOption {
	orderTerm := db.GetOrderTerm(args.OrderDir)
	switch args.OrderBy {
	case aichatmessage.FieldCreatedAt:
		return []aichatmessage.OrderOption{aichatmessage.ByUpdatedAt(orderTerm), aichatmessage.ByID(orderTerm)}
	default:
		return []aichatmessage.OrderOption{aichatmessage.ByID(orderTerm)}
	}
}

func withMessageEagerLoading(ctx context.Context, q *ent.AiChatMessageQuery) *ent.AiChatMessageQuery {
	if v, ok := ctx.Value(LoadMessageWebPage{}).(bool); ok && v {
		q.WithAiWebPage()
	}
	return q
}
