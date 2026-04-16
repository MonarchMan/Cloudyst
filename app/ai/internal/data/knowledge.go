package data

import (
	"ai/ent"
	"ai/ent/aiapikey"
	"ai/ent/aiknowledge"
	"ai/ent/aiknowledgedocument"
	"ai/ent/aiknowledgesegment"
	pb "api/api/common/v1"
	"common/db"
	"context"
	"entmodule"
	"fmt"

	"github.com/samber/lo"
)

type (
	LoadKnowledgeDocument struct{}
	LoadKnowledgeSegment  struct{}
	KnowledgeClient       interface {
		GetByID(ctx context.Context, id int) (*ent.AiKnowledge, error)
		GetByIDs(ctx context.Context, ids []int) ([]*ent.AiKnowledge, error)
		GetUserMaster(ctx context.Context, userID int) (*ent.AiKnowledge, error)
		GetActiveByID(ctx context.Context, id int) (*ent.AiKnowledge, error)
		List(ctx context.Context, args *ListKnowledgeArgs) (*ListKnowledgeResult, error)
		Upsert(ctx context.Context, args *UpsertKnowledgeArgs) (*ent.AiKnowledge, error)
		Delete(ctx context.Context, kid int) error
		BatchDelete(ctx context.Context, ids []int) (int, error)
	}

	knowledgeClient struct {
		client      *ent.Client
		maxSQLParam int
	}

	ListKnowledgeArgs struct {
		*pb.PaginationArgs
		Name    string
		ModelID int
		Status  entmodule.Status
	}

	ListKnowledgeResult struct {
		*pb.PaginationResults
		Knowledges []*ent.AiKnowledge
	}

	UpsertKnowledgeArgs struct {
		ID                  int
		Name                string
		Description         string
		EmbeddingModelID    int
		TopK                int
		SimilarityThreshold float64
		UserID              int
		IsPublic            bool
		IsMaster            bool
		Status              entmodule.Status
	}
)

func NewKnowledgeClient(client *ent.Client, dbType db.DBType) KnowledgeClient {
	return &knowledgeClient{
		client:      client,
		maxSQLParam: db.SqlParamLimit(dbType),
	}
}

func (c *knowledgeClient) GetByID(ctx context.Context, id int) (*ent.AiKnowledge, error) {
	return withKnowledgeEagerLoading(ctx, c.client.AiKnowledge.Query()).Where(aiknowledge.ID(id)).Only(ctx)
}

func (c *knowledgeClient) GetByIDs(ctx context.Context, ids []int) ([]*ent.AiKnowledge, error) {
	return withKnowledgeEagerLoading(ctx, c.client.AiKnowledge.Query()).Where(aiknowledge.IDIn(ids...)).All(ctx)
}

func (c *knowledgeClient) GetActiveByID(ctx context.Context, id int) (*ent.AiKnowledge, error) {
	k, err := withKnowledgeEagerLoading(ctx, c.client.AiKnowledge.Query()).
		Where(aiknowledge.ID(id), aiknowledge.StatusEQ(entmodule.StatusActive)).
		First(ctx)
	if err != nil {
		return nil, err
	}
	if k == nil {
		return nil, fmt.Errorf("knowledge not found")
	}
	if k.Status != entmodule.StatusActive {
		return nil, fmt.Errorf("knowledge is not active")
	}
	return k, nil
}

func (c *knowledgeClient) GetUserMaster(ctx context.Context, userID int) (*ent.AiKnowledge, error) {
	return withKnowledgeEagerLoading(ctx, c.client.AiKnowledge.Query()).
		Where(aiknowledge.UserID(userID), aiknowledge.IsMaster(true)).
		First(ctx)
}

func (c *knowledgeClient) List(ctx context.Context, args *ListKnowledgeArgs) (*ListKnowledgeResult, error) {
	q := c.client.AiKnowledge.Query()
	if args.Name != "" {
		q.Where(aiknowledge.NameContainsFold(args.Name))
	}

	if args.ModelID != 0 {
		q.Where(aiknowledge.EmbeddingModelID(args.ModelID))
	}

	if args.Status != "" {
		q.Where(aiknowledge.StatusEQ(args.Status))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, err
	}

	q.Order(getKnowledgeOrderOption(args)...)
	ks, err := withKnowledgeEagerLoading(ctx, q).Limit(int(args.PageSize)).Offset(int(args.Page * args.PageSize)).All(ctx)
	return &ListKnowledgeResult{
		PaginationResults: &pb.PaginationResults{
			Page:       args.Page,
			PageSize:   args.PageSize,
			TotalItems: int32(total),
		},
		Knowledges: ks,
	}, nil
}

func (c *knowledgeClient) Upsert(ctx context.Context, args *UpsertKnowledgeArgs) (*ent.AiKnowledge, error) {
	if args.ID == 0 {
		q := c.client.AiKnowledge.Create().
			SetName(args.Name).
			SetUserID(args.UserID).
			SetDescription(args.Description).
			SetEmbeddingModelID(args.EmbeddingModelID).
			SetTopK(args.TopK).
			SetSimilarityThreshold(args.SimilarityThreshold).
			SetIsMaster(args.IsMaster).
			SetIsPublic(args.IsPublic).
			SetStatus(args.Status)
		return q.Save(ctx)
	}

	q := c.client.AiKnowledge.UpdateOneID(args.ID).
		SetName(args.Name).
		SetDescription(args.Description).
		SetEmbeddingModelID(args.EmbeddingModelID).
		SetUserID(args.UserID).
		SetIsMaster(args.IsMaster).
		SetIsPublic(args.IsPublic).
		SetStatus(args.Status)

	if args.TopK >= 0 {
		q.SetTopK(args.TopK)
	}

	if args.SimilarityThreshold >= 0 {
		q.SetSimilarityThreshold(args.SimilarityThreshold)
	}

	return q.Save(ctx)
}

func (c *knowledgeClient) Delete(ctx context.Context, kid int) error {
	docs, err := c.client.AiKnowledgeDocument.Query().Where(aiknowledgedocument.KnowledgeID(kid)).All(ctx)
	if err != nil {
		return fmt.Errorf("failed to get knowledge documents: %w", err)
	}
	docIDs := lo.Map(docs, func(doc *ent.AiKnowledgeDocument, index int) int {
		return doc.ID
	})
	// AiKnowledgeSegment
	if _, err := c.client.AiKnowledgeSegment.Delete().Where(aiknowledgesegment.DocumentIDIn(docIDs...)).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete knowledge segments: %w", err)
	}

	// AiKnowledgeDocument
	if _, err := c.client.AiKnowledgeDocument.Delete().Where(aiknowledgedocument.KnowledgeID(kid)).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete knowledge documents: %w", err)
	}

	return c.client.AiKnowledge.DeleteOneID(kid).Exec(ctx)
}

func (c *knowledgeClient) BatchDelete(ctx context.Context, ids []int) (int, error) {
	docs, err := c.client.AiKnowledgeDocument.Query().Where(aiknowledgedocument.KnowledgeIDIn(ids...)).All(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get knowledge documents: %w", err)
	}
	docIDs := lo.Map(docs, func(doc *ent.AiKnowledgeDocument, index int) int {
		return doc.ID
	})
	// AiKnowledgeSegment
	if _, err := c.client.AiKnowledgeSegment.Delete().Where(aiknowledgesegment.DocumentIDIn(docIDs...)).Exec(ctx); err != nil {
		return 0, fmt.Errorf("failed to delete knowledge segments: %w", err)
	}
	// AiKnowledgeDocument
	if _, err := c.client.AiKnowledgeDocument.Delete().Where(aiknowledgedocument.KnowledgeIDIn(ids...)).Exec(ctx); err != nil {
		return 0, fmt.Errorf("failed to delete knowledge documents: %w", err)
	}

	return c.client.AiKnowledge.Delete().Where(aiknowledge.IDIn(ids...)).Exec(ctx)
}

func withKnowledgeEagerLoading(ctx context.Context, q *ent.AiKnowledgeQuery) *ent.AiKnowledgeQuery {
	if v, ok := ctx.Value(LoadKnowledgeDocument{}).(bool); ok && v {
		q.WithAiKnowledgeDocument()
	}
	return q
}

func getKnowledgeOrderOption(args *ListKnowledgeArgs) []aiknowledge.OrderOption {
	orderTerm := db.GetOrderTerm(db.OrderDirection(args.OrderDirection))
	switch args.OrderBy {
	case aiapikey.FieldUpdatedAt:
		return []aiknowledge.OrderOption{aiknowledge.ByUpdatedAt(orderTerm), aiknowledge.ByID(orderTerm)}
	default:
		return []aiknowledge.OrderOption{aiknowledge.ByID(orderTerm)}
	}
}
