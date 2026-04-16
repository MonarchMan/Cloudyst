package data

import (
	"ai/ent"
	"ai/ent/aiknowledgedocument"
	"ai/ent/aiknowledgesegment"
	"ai/internal/biz/types"
	pb "api/api/common/v1"
	"common/db"
	"context"
	"entmodule"
	"fmt"

	"github.com/samber/lo"
)

type (
	LoadDocumentSegment     struct{}
	KnowledgeDocumentClient interface {
		TxOperator
		GetByID(ctx context.Context, id int) (*ent.AiKnowledgeDocument, error)
		GetActiveByID(ctx context.Context, id int) (*ent.AiKnowledgeDocument, error)
		GetByIDs(ctx context.Context, ids []int) ([]*ent.AiKnowledgeDocument, error)
		GetActiveByKnowledgeID(ctx context.Context, knowledgeID int) ([]*ent.AiKnowledgeDocument, error)
		List(ctx context.Context, args *ListKnowledgeDocumentArgs) (*ListKnowledgeDocumentResult, error)
		Upsert(ctx context.Context, args *UpsertDocumentArgs) (*ent.AiKnowledgeDocument, error)
		UpdateKnowledgeID(ctx context.Context, id, knowledgeID int) (*ent.AiKnowledgeDocument, error)
		BatchCreate(ctx context.Context, args []*UpsertDocumentArgs) ([]*ent.AiKnowledgeDocument, error)
		UpdateStatus(ctx context.Context, id int, status entmodule.Status) (*ent.AiKnowledgeDocument, error)
		UpdateProcess(ctx context.Context, id int, status types.DocumentStatus) (*ent.AiKnowledgeDocument, error)
		UpdateContentLenVersionAndProcess(ctx context.Context, id int, len int, version string, status types.DocumentStatus) (*ent.AiKnowledgeDocument, error)
		Delete(ctx context.Context, kid int) error
		BatchDelete(ctx context.Context, ids []int) (int, error)
		UpdateRetrievalCount(ctx context.Context, id int, count int) error
		AddRetrievalCount(ctx context.Context, id int, count int) error
	}

	knowledgeDocumentClient struct {
		client      *ent.Client
		maxSQLParam int
	}

	ListKnowledgeDocumentArgs struct {
		*pb.PaginationArgs
		KnowledgeID int
		Name        string
		Status      entmodule.Status
	}

	ListKnowledgeDocumentResult struct {
		*pb.PaginationResults
		Documents []*ent.AiKnowledgeDocument
	}

	UpsertDocumentArgs struct {
		ID               int
		KnowledgeID      int
		Name             string
		Url              string
		Version          string
		ContentLen       int
		SegmentMaxTokens int
		RetrievalCount   int
		Process          types.DocumentStatus
		Status           entmodule.Status
	}
)

func NewKnowledgeDocumentClient(client *ent.Client, dbType db.DBType) KnowledgeDocumentClient {
	return &knowledgeDocumentClient{client: client, maxSQLParam: db.SqlParamLimit(dbType)}
}

func (c *knowledgeDocumentClient) SetClient(newClient *ent.Client) TxOperator {
	return &knowledgeDocumentClient{client: newClient, maxSQLParam: c.maxSQLParam}
}

func (c *knowledgeDocumentClient) GetClient() *ent.Client {
	return c.client
}

func (c *knowledgeDocumentClient) GetByID(ctx context.Context, id int) (*ent.AiKnowledgeDocument, error) {
	return withKnowledgeDocumentEagerLoading(ctx, c.client.AiKnowledgeDocument.Query()).Where(aiknowledgedocument.ID(id)).Only(ctx)
}

func (c *knowledgeDocumentClient) GetByIDs(ctx context.Context, ids []int) ([]*ent.AiKnowledgeDocument, error) {
	return withKnowledgeDocumentEagerLoading(ctx, c.client.AiKnowledgeDocument.Query()).Where(aiknowledgedocument.IDIn(ids...)).All(ctx)
}

func (c *knowledgeDocumentClient) GetActiveByID(ctx context.Context, id int) (*ent.AiKnowledgeDocument, error) {
	return withKnowledgeDocumentEagerLoading(ctx, c.client.AiKnowledgeDocument.Query()).
		Where(aiknowledgedocument.ID(id), aiknowledgedocument.StatusEQ(entmodule.StatusActive)).
		First(ctx)
}

func (c *knowledgeDocumentClient) GetActiveByKnowledgeID(ctx context.Context, knowledgeID int) ([]*ent.AiKnowledgeDocument, error) {
	return withKnowledgeDocumentEagerLoading(ctx, c.client.AiKnowledgeDocument.Query()).
		Where(aiknowledgedocument.KnowledgeID(knowledgeID), aiknowledgedocument.StatusEQ(entmodule.StatusActive)).
		All(ctx)
}

func (c *knowledgeDocumentClient) List(ctx context.Context, args *ListKnowledgeDocumentArgs) (*ListKnowledgeDocumentResult, error) {
	q := c.client.AiKnowledgeDocument.Query()
	if args.KnowledgeID != 0 {
		q.Where(aiknowledgedocument.KnowledgeIDIn(args.KnowledgeID))
	}

	if args.Name != "" {
		q.Where(aiknowledgedocument.NameContainsFold(args.Name))
	}

	if args.Status != "" {
		q.Where(aiknowledgedocument.StatusEQ(args.Status))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, err
	}
	q.Order(getKnowledgeDocumentOrderOption(args)...)
	documents, err := withKnowledgeDocumentEagerLoading(ctx, q).Limit(int(args.PageSize)).Offset(int(args.Page * args.PageSize)).All(ctx)
	if err != nil {
		return nil, err
	}

	return &ListKnowledgeDocumentResult{
		PaginationResults: &pb.PaginationResults{
			Page:       args.Page,
			PageSize:   args.PageSize,
			TotalItems: int32(total),
		},
		Documents: documents,
	}, nil
}

func (c *knowledgeDocumentClient) Upsert(ctx context.Context, args *UpsertDocumentArgs) (*ent.AiKnowledgeDocument, error) {
	if args.ID == 0 {
		q := c.client.AiKnowledgeDocument.Create().
			SetKnowledgeID(args.KnowledgeID).
			SetName(args.Name).
			SetURL(args.Url).
			SetVersion(args.Version).
			SetContentLength(args.ContentLen).
			SetSegmentMaxTokens(args.SegmentMaxTokens).
			SetRetrievalCount(args.RetrievalCount).
			SetStatus(args.Status)
		return q.Save(ctx)
	}
	q := c.client.AiKnowledgeDocument.UpdateOneID(args.ID).
		SetName(args.Name).
		SetSegmentMaxTokens(args.SegmentMaxTokens).
		SetRetrievalCount(args.RetrievalCount).
		SetStatus(args.Status)
	if args.KnowledgeID > 0 {
		q.Where(aiknowledgedocument.KnowledgeID(args.KnowledgeID))
	}
	return q.Save(ctx)
}

func (c *knowledgeDocumentClient) UpdateKnowledgeID(ctx context.Context, id, knowledgeID int) (*ent.AiKnowledgeDocument, error) {
	return c.client.AiKnowledgeDocument.UpdateOneID(id).SetKnowledgeID(knowledgeID).Save(ctx)
}

func (c *knowledgeDocumentClient) BatchCreate(ctx context.Context, args []*UpsertDocumentArgs) ([]*ent.AiKnowledgeDocument, error) {
	batch := lo.Map(args, func(doc *UpsertDocumentArgs, index int) *ent.AiKnowledgeDocumentCreate {
		return c.client.AiKnowledgeDocument.Create().
			SetKnowledgeID(doc.KnowledgeID).
			SetName(doc.Name).
			SetURL(doc.Url).
			SetContentLength(doc.ContentLen).
			SetSegmentMaxTokens(doc.SegmentMaxTokens).
			SetRetrievalCount(doc.RetrievalCount).
			SetStatus(doc.Status)
	})
	return c.client.AiKnowledgeDocument.CreateBulk(batch...).Save(ctx)
}

func (c *knowledgeDocumentClient) UpdateStatus(ctx context.Context, id int, status entmodule.Status) (*ent.AiKnowledgeDocument, error) {
	return c.client.AiKnowledgeDocument.UpdateOneID(id).SetStatus(status).Save(ctx)
}

func (c *knowledgeDocumentClient) UpdateProcess(ctx context.Context, id int, status types.DocumentStatus) (*ent.AiKnowledgeDocument, error) {
	return c.client.AiKnowledgeDocument.UpdateOneID(id).SetProcess(status).Save(ctx)
}

func (c *knowledgeDocumentClient) UpdateContentLenVersionAndProcess(ctx context.Context, id int, len int, version string, status types.DocumentStatus) (*ent.AiKnowledgeDocument, error) {
	q := c.client.AiKnowledgeDocument.UpdateOneID(id).
		SetContentLength(len).
		SetProcess(status)
	if version != "" {
		q.SetVersion(version)
	}
	return q.Save(ctx)
}

func (c *knowledgeDocumentClient) Delete(ctx context.Context, kid int) error {
	// AiKnowledgeSegment
	if _, err := c.client.AiKnowledgeSegment.Delete().Where(aiknowledgesegment.DocumentID(kid)).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete ai_knowledge_segement: %w", err)
	}
	return c.client.AiKnowledgeDocument.DeleteOneID(kid).Exec(ctx)
}

func (c *knowledgeDocumentClient) BatchDelete(ctx context.Context, ids []int) (int, error) {
	// AiKnowledgeSegment
	if _, err := c.client.AiKnowledgeSegment.Delete().Where(aiknowledgesegment.DocumentIDIn(ids...)).Exec(ctx); err != nil {
		return 0, fmt.Errorf("failed to delete ai_knowledge_segement: %w", err)
	}

	return c.client.AiKnowledgeDocument.Delete().Where(aiknowledgedocument.IDIn(ids...)).Exec(ctx)
}

func (c *knowledgeDocumentClient) UpdateRetrievalCount(ctx context.Context, id int, count int) error {
	return c.client.AiKnowledgeDocument.UpdateOneID(id).SetRetrievalCount(count).Exec(ctx)
}

func (c *knowledgeDocumentClient) AddRetrievalCount(ctx context.Context, id int, count int) error {
	return c.client.AiKnowledgeDocument.UpdateOneID(id).AddRetrievalCount(count).Exec(ctx)
}

func withKnowledgeDocumentEagerLoading(ctx context.Context, q *ent.AiKnowledgeDocumentQuery) *ent.AiKnowledgeDocumentQuery {
	if v, ok := ctx.Value(LoadKnowledgeDocument{}).(bool); ok && v {
		q.WithAiKnowledge()
	}
	if v, ok := ctx.Value(LoadDocumentSegment{}).(bool); ok && v {
		q.WithAiKnowledgeSegment()
	}
	return q
}

func getKnowledgeDocumentOrderOption(args *ListKnowledgeDocumentArgs) []aiknowledgedocument.OrderOption {
	orderTerm := db.GetOrderTerm(db.OrderDirection(args.OrderDirection))
	switch args.OrderBy {
	case aiknowledgedocument.FieldRetrievalCount:
		return []aiknowledgedocument.OrderOption{aiknowledgedocument.ByRetrievalCount(orderTerm), aiknowledgedocument.ByID(orderTerm)}
	case aiknowledgedocument.FieldUpdatedAt:
		return []aiknowledgedocument.OrderOption{aiknowledgedocument.ByUpdatedAt(orderTerm), aiknowledgedocument.ByID(orderTerm)}
	default:
		return []aiknowledgedocument.OrderOption{aiknowledgedocument.ByID(orderTerm)}
	}
}
