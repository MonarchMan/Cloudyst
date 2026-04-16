package data

import (
	"ai/ent"
	"ai/ent/aiwebpage"
	"ai/internal/biz/types"
	pb "api/api/file/common/v1"
	"common/db"
	"context"
)

type (
	WebPageClient interface {
		TxOperator
		GetByID(ctx context.Context, id int) (*ent.AiWebPage, error)
		GetByIDs(ctx context.Context, ids []int) ([]*ent.AiWebPage, error)
		ListByMessageIDs(ctx context.Context, id []int) ([]*ent.AiWebPage, error)
		List(ctx context.Context, args *ListWebPageArgs) (*ListWebPageResult, error)
		Create(ctx context.Context, webPage *types.WebPage) (*ent.AiWebPage, error)
		BatchCreate(ctx context.Context, webPages []*types.WebPage) ([]*ent.AiWebPage, error)
		Delete(ctx context.Context, id int) error
		BatchDelete(ctx context.Context, ids []int) (int, error)
	}

	webPageClient struct {
		maxSQLParam int
		client      *ent.Client
	}

	ListWebPageArgs struct {
		*pb.PaginationArgs
		name      string
		messageID int
	}

	ListWebPageResult struct {
		*pb.PaginationResults
		Pages []*ent.AiWebPage
	}
)

func NewWebPageClient(client *ent.Client, dbType db.DBType) WebPageClient {
	return &webPageClient{maxSQLParam: db.SqlParamLimit(dbType), client: client}
}

func (c *webPageClient) SetClient(newClient *ent.Client) TxOperator {
	return &webPageClient{maxSQLParam: c.maxSQLParam, client: newClient}
}

func (c *webPageClient) GetClient() *ent.Client {
	return c.client
}

func (c *webPageClient) GetByID(ctx context.Context, id int) (*ent.AiWebPage, error) {
	return withWebPageEagerLoading(ctx, c.client.AiWebPage.Query()).Where(aiwebpage.ID(id)).Only(ctx)
}

func (c *webPageClient) GetByIDs(ctx context.Context, ids []int) ([]*ent.AiWebPage, error) {
	return withWebPageEagerLoading(ctx, c.client.AiWebPage.Query()).Where(aiwebpage.IDIn(ids...)).All(ctx)
}

func (c *webPageClient) List(ctx context.Context, args *ListWebPageArgs) (*ListWebPageResult, error) {
	pageSize := db.CapPageSize(c.maxSQLParam, int(args.PageSize), 100)
	q := c.client.AiWebPage.Query()
	if args.name != "" {
		q.Where(aiwebpage.NameContainsFold(args.name))
	}
	if args.messageID > 0 {
		q.Where(aiwebpage.MessageID(args.messageID))
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, err
	}
	q.Order(getWebPageOrderOption(args)...)
	pages, err := withWebPageEagerLoading(ctx, q).Limit(int(args.PageSize)).Offset(int(args.Page * args.PageSize)).All(ctx)
	if err != nil {
		return nil, err
	}
	return &ListWebPageResult{
		PaginationResults: &pb.PaginationResults{
			Page:       args.Page,
			PageSize:   int32(pageSize),
			TotalItems: int32(total),
		},
		Pages: pages,
	}, nil
}

func (c *webPageClient) ListByMessageIDs(ctx context.Context, id []int) ([]*ent.AiWebPage, error) {
	return withWebPageEagerLoading(ctx, c.client.AiWebPage.Query()).Where(aiwebpage.MessageIDIn(id...)).All(ctx)
}

func (c *webPageClient) Create(ctx context.Context, webPage *types.WebPage) (*ent.AiWebPage, error) {
	return c.client.AiWebPage.Create().
		SetName(webPage.Name).
		SetIcon(webPage.Icon).
		SetTitle(webPage.Title).
		SetURL(webPage.URL).
		SetSnippet(webPage.Snippet).
		SetSummary(webPage.Summary).
		SetMessageID(webPage.MessageID).
		Save(ctx)
}

func (c *webPageClient) BatchCreate(ctx context.Context, webPages []*types.WebPage) ([]*ent.AiWebPage, error) {
	batch := make([]*ent.AiWebPageCreate, len(webPages))
	for i, webPage := range webPages {
		batch[i] = c.client.AiWebPage.Create().
			SetName(webPage.Name).
			SetIcon(webPage.Icon).
			SetTitle(webPage.Title).
			SetURL(webPage.URL).
			SetSnippet(webPage.Snippet).
			SetSummary(webPage.Summary).
			SetMessageID(webPage.MessageID)
	}

	return c.client.AiWebPage.CreateBulk(batch...).Save(ctx)
}

func (c *webPageClient) Delete(ctx context.Context, id int) error {
	return c.client.AiWebPage.DeleteOneID(id).Exec(ctx)
}

func (c *webPageClient) BatchDelete(ctx context.Context, ids []int) (int, error) {
	return c.client.AiWebPage.Delete().Where(aiwebpage.IDIn(ids...)).Exec(ctx)
}

func getWebPageOrderOption(args *ListWebPageArgs) []aiwebpage.OrderOption {
	orderTerm := db.GetOrderTerm(db.OrderDirection(args.OrderDirection))
	switch args.OrderBy {
	case aiwebpage.FieldUpdatedAt:
		return []aiwebpage.OrderOption{aiwebpage.ByUpdatedAt(orderTerm), aiwebpage.ByID(orderTerm)}
	default:
		return []aiwebpage.OrderOption{aiwebpage.ByID(orderTerm)}
	}
}

func withWebPageEagerLoading(ctx context.Context, q *ent.AiWebPageQuery) *ent.AiWebPageQuery {
	if v, ok := ctx.Value(LoadMessageWebPage{}).(bool); ok && v {
		q.WithAiChatMessage()
	}
	return q
}
