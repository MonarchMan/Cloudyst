package data

import (
	"ai/ent"
	"ai/ent/aiknowledgesegment"
	"ai/internal/biz/types"
	"api/external/data/common"
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
		GetByDocumentChunkIndexes(ctx context.Context, documentID int, indexes []int) ([]*ent.AiKnowledgeSegment, error)
		GetNeighbors(ctx context.Context, documentID int, chunkIndex int, window int) ([]*ent.AiKnowledgeSegment, error)
		GetNeighborsByID(ctx context.Context, id int, window int) ([]*ent.AiKnowledgeSegment, error)
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
		*common.PaginationArgs
		KnowledgeID int
		DocumentID  int
	}

	ListKnowledgeSegmentResult struct {
		*common.PaginationResults
		KnowledgeSegments []*ent.AiKnowledgeSegment
	}
)

func NewKnowledgeSegmentClient(client *ent.Client, dbType common.DBType) KnowledgeSegmentClient {
	return &knowledgeSegmentClient{
		client:      client,
		maxSQLParam: common.SqlParamLimit(dbType),
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

func (c *knowledgeSegmentClient) GetByDocumentChunkIndexes(ctx context.Context, documentID int, indexes []int) ([]*ent.AiKnowledgeSegment, error) {
	if documentID == 0 || len(indexes) == 0 {
		return nil, nil
	}
	return withKnowledgeSegmentEagerLoading(ctx, c.client.AiKnowledgeSegment.Query()).
		Where(
			aiknowledgesegment.DocumentID(documentID),
			aiknowledgesegment.ChunkIndexIn(indexes...),
		).
		Order(aiknowledgesegment.ByChunkIndex()).
		All(ctx)
}

func (c *knowledgeSegmentClient) GetNeighbors(ctx context.Context, documentID int, chunkIndex int, window int) ([]*ent.AiKnowledgeSegment, error) {
	if documentID == 0 {
		return nil, nil
	}
	if window < 0 {
		window = 0
	}
	start := chunkIndex - window
	if start < 0 {
		start = 0
	}
	end := chunkIndex + window
	return withKnowledgeSegmentEagerLoading(ctx, c.client.AiKnowledgeSegment.Query()).
		Where(
			aiknowledgesegment.DocumentID(documentID),
			aiknowledgesegment.ChunkIndexGTE(start),
			aiknowledgesegment.ChunkIndexLTE(end),
			aiknowledgesegment.StatusEQ(entmodule.StatusActive),
		).
		Order(aiknowledgesegment.ByChunkIndex()).
		All(ctx)
}

func (c *knowledgeSegmentClient) GetNeighborsByID(ctx context.Context, id int, window int) ([]*ent.AiKnowledgeSegment, error) {
	seg, err := c.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return c.GetNeighbors(ctx, seg.DocumentID, seg.ChunkIndex, window)
}

func (c *knowledgeSegmentClient) List(ctx context.Context, args *ListKnowledgeSegmentArgs) (*ListKnowledgeSegmentResult, error) {
	q := c.client.AiKnowledgeSegment.Query()
	if args.KnowledgeID != 0 {
		q.Where(aiknowledgesegment.KnowledgeID(args.KnowledgeID))
	}
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
		PaginationResults: &common.PaginationResults{
			TotalItems: total,
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
			SetKnowledgeID(segment.KnowledgeID).
			SetContentLength(segment.ContentLen).
			SetTokens(segment.Tokens).
			SetChunkIndex(segment.ChunkIndex).
			SetSectionPath(segment.SectionPath).
			SetStartOffset(segment.StartOffset).
			SetEndOffset(segment.EndOffset).
			SetMetadata(nonNilMetadata(segment.Metadata)).
			SetVectorID(segment.VectorID).
			SetStatus(defaultSegmentStatus(segment.Status)).
			Save(ctx)
	}
	return c.client.AiKnowledgeSegment.UpdateOneID(segment.ID).
		SetKnowledgeID(segment.KnowledgeID).
		SetContentLength(segment.ContentLen).
		SetTokens(segment.Tokens).
		SetChunkIndex(segment.ChunkIndex).
		SetSectionPath(segment.SectionPath).
		SetStartOffset(segment.StartOffset).
		SetEndOffset(segment.EndOffset).
		SetMetadata(nonNilMetadata(segment.Metadata)).
		SetVectorID(segment.VectorID).
		SetStatus(defaultSegmentStatus(segment.Status)).
		Save(ctx)
}

func (c *knowledgeSegmentClient) BatchCreate(ctx context.Context, segments []*types.KnowledgeSegment) ([]*ent.AiKnowledgeSegment, error) {
	batch := lo.Map(segments, func(segment *types.KnowledgeSegment, _ int) *ent.AiKnowledgeSegmentCreate {
		return c.client.AiKnowledgeSegment.Create().
			SetDocumentID(segment.DocumentID).
			SetKnowledgeID(segment.KnowledgeID).
			SetContentLength(segment.ContentLen).
			SetTokens(segment.Tokens).
			SetChunkIndex(segment.ChunkIndex).
			SetSectionPath(segment.SectionPath).
			SetStartOffset(segment.StartOffset).
			SetEndOffset(segment.EndOffset).
			SetMetadata(nonNilMetadata(segment.Metadata)).
			SetVectorID(segment.VectorID).
			SetStatus(defaultSegmentStatus(segment.Status))
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
	orderTerm := common.GetOrderTerm(args.OrderDir)
	switch args.OrderBy {
	case aiknowledgesegment.FieldRetrievalCount:
		return []aiknowledgesegment.OrderOption{aiknowledgesegment.ByRetrievalCount(orderTerm), aiknowledgesegment.ByID(orderTerm)}
	case aiknowledgesegment.FieldUpdatedAt:
		return []aiknowledgesegment.OrderOption{aiknowledgesegment.ByUpdatedAt(orderTerm), aiknowledgesegment.ByID(orderTerm)}
	default:
		return []aiknowledgesegment.OrderOption{aiknowledgesegment.ByID(orderTerm)}
	}
}

func defaultSegmentStatus(status entmodule.Status) entmodule.Status {
	if status == "" {
		return entmodule.StatusActive
	}
	return status
}
