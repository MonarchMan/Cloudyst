package data

import (
	"ai/ent"
	"ai/ent/aiknowledgesegment"
	"ai/internal/biz/types"
	pb "api/api/common/v1"
	"common/db"
	"context"
	"entmodule"

	"github.com/samber/lo"
)

type (
	KnowledgeSegmentClient interface {
		TxOperator
		GetByID(ctx context.Context, id int) (*ent.AiKnowledgeSegment, error)
		GetByIDs(ctx context.Context, ids []int) ([]*ent.AiKnowledgeSegment, error)
		GetActiveByDocumentIDs(ctx context.Context, docIDs []int) ([]*ent.AiKnowledgeSegment, error)
		List(ctx context.Context, args *ListKnowledgeSegmentArgs) (*ListKnowledgeSegmentResult, error)
		Upsert(ctx context.Context, segment *types.KnowledgeSegment) (*ent.AiKnowledgeSegment, error)
		BatchCreate(ctx context.Context, segments []*types.KnowledgeSegment) ([]*ent.AiKnowledgeSegment, error)
		Delete(ctx context.Context, id int) error
		DeleteByDocumentID(ctx context.Context, documentID int) (int, error)
		BatchDelete(ctx context.Context, ids []int) (int, error)
		UpdateRetrievalCountByIDs(ctx context.Context, ids []int, count int) error
		AddRetrievalCount(ctx context.Context, id int, count int) error
		UpdateVectorID(ctx context.Context, id int, vid string) error
		SetEmptyVectorID(ctx context.Context, ids []int) error
		GetByVectorIDs(ctx context.Context, ds []string) ([]*ent.AiKnowledgeSegment, error)
		UpdateStatus(ctx context.Context, id int, status entmodule.Status) (*ent.AiKnowledgeSegment, error)
	}

	knowledgeSegmentClient struct {
		maxSQLParam int
		client      *ent.Client
	}

	ListKnowledgeSegmentArgs struct {
		*pb.PaginationArgs
		KnowledgeID int
		DocumentID  int
	}

	ListKnowledgeSegmentResult struct {
		*pb.PaginationResults
		KnowledgeSegments []*ent.AiKnowledgeSegment
	}
)

func NewKnowledgeSegmentClient(client *ent.Client, dbType db.DBType) KnowledgeSegmentClient {
	return &knowledgeSegmentClient{
		client:      client,
		maxSQLParam: db.SqlParamLimit(dbType),
	}
}

func (c *knowledgeSegmentClient) SetClient(newClient *ent.Client) TxOperator {
	return &knowledgeSegmentClient{client: newClient, maxSQLParam: c.maxSQLParam}
}

func (c *knowledgeSegmentClient) GetClient() *ent.Client {
	return c.client
}

func (c *knowledgeSegmentClient) GetByID(ctx context.Context, id int) (*ent.AiKnowledgeSegment, error) {
	return withKnowledgeSegmentEagerLoading(ctx, c.client.AiKnowledgeSegment.Query()).Where(aiknowledgesegment.ID(id)).Only(ctx)
}

func (c *knowledgeSegmentClient) GetByIDs(ctx context.Context, ids []int) ([]*ent.AiKnowledgeSegment, error) {
	return withKnowledgeSegmentEagerLoading(ctx, c.client.AiKnowledgeSegment.Query()).Where(aiknowledgesegment.IDIn(ids...)).All(ctx)
}

func (c *knowledgeSegmentClient) GetActiveByDocumentIDs(ctx context.Context, docIDs []int) ([]*ent.AiKnowledgeSegment, error) {
	if len(docIDs) == 1 {
		return withKnowledgeSegmentEagerLoading(ctx, c.client.AiKnowledgeSegment.Query()).
			Where(aiknowledgesegment.DocumentID(docIDs[0]), aiknowledgesegment.StatusEQ(entmodule.StatusActive)).
			All(ctx)
	}
	return withKnowledgeSegmentEagerLoading(ctx, c.client.AiKnowledgeSegment.Query()).
		Where(aiknowledgesegment.DocumentIDIn(docIDs...), aiknowledgesegment.StatusEQ(entmodule.StatusActive)).
		All(ctx)
}

func (c *knowledgeSegmentClient) List(ctx context.Context, args *ListKnowledgeSegmentArgs) (*ListKnowledgeSegmentResult, error) {
	q := c.client.AiKnowledgeSegment.Query()
	if args.DocumentID != 0 {
		q.Where(aiknowledgesegment.DocumentID(args.DocumentID))
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, err
	}

	q.Order(getKnowledgeSegmentOrderOption(args)...)
	segments, err := withKnowledgeSegmentEagerLoading(ctx, q).Limit(int(args.PageSize)).Offset(int(args.Page * args.PageSize)).All(ctx)
	if err != nil {
		return nil, err
	}

	return &ListKnowledgeSegmentResult{
		PaginationResults: &pb.PaginationResults{
			TotalItems: int32(total),
			Page:       args.Page,
			PageSize:   args.PageSize,
		},
		KnowledgeSegments: segments,
	}, nil
}

func (c *knowledgeSegmentClient) Upsert(ctx context.Context, segment *types.KnowledgeSegment) (*ent.AiKnowledgeSegment, error) {
	if segment.ID == 0 {
		return c.client.AiKnowledgeSegment.Create().
			SetDocumentID(segment.DocumentID).
			SetTokens(segment.Tokens).
			SetVectorID(segment.VectorID).
			SetStatus(segment.Status).
			Save(ctx)
	}
	return c.client.AiKnowledgeSegment.UpdateOneID(segment.ID).
		SetTokens(segment.Tokens).
		SetVectorID(segment.VectorID).
		SetStatus(segment.Status).
		Save(ctx)
}

func (c *knowledgeSegmentClient) BatchCreate(ctx context.Context, segments []*types.KnowledgeSegment) ([]*ent.AiKnowledgeSegment, error) {
	batch := lo.Map(segments, func(segment *types.KnowledgeSegment, _ int) *ent.AiKnowledgeSegmentCreate {
		return c.client.AiKnowledgeSegment.Create().
			SetDocumentID(segment.DocumentID).
			SetTokens(segment.Tokens).
			SetVectorID(segment.VectorID).
			SetStatus(segment.Status)
	})
	return c.client.AiKnowledgeSegment.CreateBulk(batch...).Save(ctx)
}

func (c *knowledgeSegmentClient) Delete(ctx context.Context, id int) error {
	return c.client.AiKnowledgeSegment.DeleteOneID(id).Exec(ctx)
}

func (c *knowledgeSegmentClient) BatchDelete(ctx context.Context, ids []int) (int, error) {
	return c.client.AiKnowledgeSegment.Delete().Where(aiknowledgesegment.IDIn(ids...)).Exec(ctx)
}

func (c *knowledgeSegmentClient) DeleteByDocumentID(ctx context.Context, documentID int) (int, error) {
	return c.client.AiKnowledgeSegment.Delete().Where(aiknowledgesegment.DocumentID(documentID)).Exec(ctx)
}

func (c *knowledgeSegmentClient) UpdateRetrievalCountByIDs(ctx context.Context, ids []int, count int) error {
	return c.client.AiKnowledgeSegment.Update().
		Where(aiknowledgesegment.IDIn(ids...)).
		SetRetrievalCount(count).
		Exec(ctx)
}

func (c *knowledgeSegmentClient) AddRetrievalCount(ctx context.Context, id int, count int) error {
	return c.client.AiKnowledgeSegment.UpdateOneID(id).AddRetrievalCount(count).Exec(ctx)
}

func (c *knowledgeSegmentClient) UpdateVectorID(ctx context.Context, id int, s string) error {
	return c.client.AiKnowledgeSegment.UpdateOneID(id).SetVectorID(s).Exec(ctx)
}

func (c *knowledgeSegmentClient) SetEmptyVectorID(ctx context.Context, ids []int) error {
	return c.client.AiKnowledgeSegment.Update().
		Where(aiknowledgesegment.IDIn(ids...)).
		SetVectorID("").
		Exec(ctx)
}

func (c *knowledgeSegmentClient) GetByVectorIDs(ctx context.Context, ds []string) ([]*ent.AiKnowledgeSegment, error) {
	return c.client.AiKnowledgeSegment.Query().Where(aiknowledgesegment.VectorIDIn(ds...)).All(ctx)
}

func (c *knowledgeSegmentClient) UpdateStatus(ctx context.Context, id int, status entmodule.Status) (*ent.AiKnowledgeSegment, error) {
	return c.client.AiKnowledgeSegment.UpdateOneID(id).SetStatus(status).Save(ctx)
}

func withKnowledgeSegmentEagerLoading(ctx context.Context, q *ent.AiKnowledgeSegmentQuery) *ent.AiKnowledgeSegmentQuery {
	if v, ok := ctx.Value(LoadDocumentSegment{}).(bool); ok && v {
		q.WithAiKnowledgeDocument()
	}
	return q
}

func getKnowledgeSegmentOrderOption(args *ListKnowledgeSegmentArgs) []aiknowledgesegment.OrderOption {
	orderTerm := db.GetOrderTerm(db.OrderDirection(args.OrderDirection))
	switch args.OrderBy {
	case aiknowledgesegment.FieldRetrievalCount:
		return []aiknowledgesegment.OrderOption{aiknowledgesegment.ByRetrievalCount(orderTerm), aiknowledgesegment.ByID(orderTerm)}
	case aiknowledgesegment.FieldUpdatedAt:
		return []aiknowledgesegment.OrderOption{aiknowledgesegment.ByUpdatedAt(orderTerm), aiknowledgesegment.ByID(orderTerm)}
	default:
		return []aiknowledgesegment.OrderOption{aiknowledgesegment.ByID(orderTerm)}
	}
}
