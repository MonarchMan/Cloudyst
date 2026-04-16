package data

import (
	"ai/ent"
	"ai/ent/aiapikey"
	"ai/ent/aimodel"
	pb "api/api/common/v1"
	"common/db"
	"context"
	"entmodule"
	"fmt"
)

type (
	LoadApiKeyModel struct{}
	ModelClient     interface {
		TxOperator
		GetModelByID(ctx context.Context, id int) (*ent.AiModel, error)
		GetModels(ctx context.Context, ids []int) ([]*ent.AiModel, error)
		GetActiveModelByIDType(ctx context.Context, id int, modelType string) (*ent.AiModel, error)
		GetDefaultModel(ctx context.Context, modelType string) (*ent.AiModel, error)
		ListModels(ctx context.Context, args *ListAiModelArgs) (*ListAiModelResult, error)
		UpsertModel(ctx context.Context, model *ent.AiModel) (*ent.AiModel, error)
		DeleteModel(ctx context.Context, id int) error
		DeleteModels(ctx context.Context, ids []int) (int, error)

		GetApiKeyByID(ctx context.Context, id int) (*ent.AiApiKey, error)
		GetApiKeys(ctx context.Context, ids []int) ([]*ent.AiApiKey, error)
		ListApiKeys(ctx context.Context, args *ListApiKeyArgs) (*ListApiKeyResult, error)
		UpsertApiKey(ctx context.Context, key *ent.AiApiKey) (*ent.AiApiKey, error)
		DeleteApiKey(ctx context.Context, kid int) error
		DeleteApiKeys(ctx context.Context, ids []int) (int, error)
	}

	modelClient struct {
		maxSQLParam int
		client      *ent.Client
	}

	ListAiModelArgs struct {
		*pb.PaginationArgs
		Name     string
		Platform string
		Status   entmodule.Status
	}

	ListAiModelResult struct {
		*pb.PaginationResults
		Models []*ent.AiModel
	}

	ListApiKeyArgs struct {
		*pb.PaginationArgs
		Name     string
		Platform string
		Status   entmodule.Status
	}

	ListApiKeyResult struct {
		*pb.PaginationResults
		ApiKeys []*ent.AiApiKey
	}
)

func NewAIModelClient(client *ent.Client, dbType db.DBType) ModelClient {
	return &modelClient{maxSQLParam: db.SqlParamLimit(dbType), client: client}
}

func (c *modelClient) SetClient(newClient *ent.Client) TxOperator {
	return &modelClient{maxSQLParam: c.maxSQLParam, client: newClient}
}

func (c *modelClient) GetClient() *ent.Client {
	return c.client
}

func (c *modelClient) GetModelByID(ctx context.Context, id int) (*ent.AiModel, error) {
	return withModelEagerLoading(ctx, c.client.AiModel.Query()).Where(aimodel.ID(id)).Only(ctx)
}

func (c *modelClient) GetModels(ctx context.Context, ids []int) ([]*ent.AiModel, error) {
	ms, err := withModelEagerLoading(ctx, c.client.AiModel.Query()).Where(aimodel.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query models %v: %w", ids, err)
	}
	return ms, nil
}

func (c *modelClient) GetActiveModelByIDType(ctx context.Context, id int, modelType string) (*ent.AiModel, error) {
	return withModelEagerLoading(ctx, c.client.AiModel.Query()).
		Where(aimodel.ID(id)).
		Where(aimodel.Type(modelType)).
		Where(aimodel.StatusEQ(entmodule.StatusActive)).
		Only(ctx)
}

func (c *modelClient) GetDefaultModel(ctx context.Context, modelType string) (*ent.AiModel, error) {
	return withModelEagerLoading(ctx, c.client.AiModel.Query()).
		Where(aimodel.Type(modelType)).
		Where(aimodel.StatusEQ(entmodule.StatusActive)).
		First(ctx)
}

func (c *modelClient) ListModels(ctx context.Context, args *ListAiModelArgs) (*ListAiModelResult, error) {
	pageSize := db.CapPageSize(c.maxSQLParam, int(args.PageSize), 10)
	q := c.client.AiModel.Query()
	if args.Name != "" {
		q.Where(aimodel.NameContainsFold(args.Name))
	}

	if args.Platform != "" {
		q.Where(aimodel.Platform(args.Platform))
	}

	if args.Status != "" {
		q.Where(aimodel.StatusEQ(args.Status))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, err
	}

	q.Order(getModelOrderOption(args)...)
	ms, err := withModelEagerLoading(ctx, q).Limit(pageSize).Offset(int(args.Page) * pageSize).All(ctx)
	if err != nil {
		return nil, err
	}

	return &ListAiModelResult{
		PaginationResults: &pb.PaginationResults{
			TotalItems: int32(total),
			Page:       args.Page,
			PageSize:   int32(pageSize),
		},
		Models: ms,
	}, nil
}

func (c *modelClient) UpsertModel(ctx context.Context, model *ent.AiModel) (*ent.AiModel, error) {
	if model.ID == 0 {
		q := c.client.AiModel.Create().
			SetName(model.Name).
			SetType(model.Type).
			SetPlatform(model.Platform).
			SetSort(model.Sort).
			SetStatus(model.Status).
			SetTemperature(model.Temperature).
			SetMaxTokens(model.MaxTokens).
			SetMaxContext(model.MaxContext).
			SetKeyID(model.KeyID)
		return q.Save(ctx)
	}

	q := c.client.AiModel.UpdateOne(model).
		SetName(model.Name).
		SetPlatform(model.Platform).
		SetSort(model.Sort).
		SetStatus(model.Status).
		SetTemperature(model.Temperature)
	if model.Type != "" {
		q = q.SetType(model.Type)
	}

	if model.Temperature >= 0 {
		q = q.SetTemperature(model.Temperature)
	}

	if model.MaxTokens >= 0 {
		q = q.SetMaxTokens(model.MaxTokens)
	}

	if model.MaxContext >= 0 {
		q = q.SetMaxContext(model.MaxContext)
	}

	return q.Save(ctx)
}

func (c *modelClient) DeleteModel(ctx context.Context, id int) error {
	return c.client.AiModel.DeleteOneID(id).Exec(ctx)
}

func (c *modelClient) DeleteModels(ctx context.Context, ids []int) (int, error) {
	return c.client.AiModel.Delete().Where(aimodel.IDIn(ids...)).Exec(ctx)
}

func getModelOrderOption(args *ListAiModelArgs) []aimodel.OrderOption {
	orderTerm := db.GetOrderTerm(db.OrderDirection(args.OrderDirection))
	switch args.OrderBy {
	case aimodel.FieldUpdatedAt:
		return []aimodel.OrderOption{aimodel.ByUpdatedAt(orderTerm), aimodel.ByID(orderTerm)}
	case aimodel.FieldSort:
		return []aimodel.OrderOption{aimodel.BySort(orderTerm)}
	default:
		return []aimodel.OrderOption{aimodel.ByID(orderTerm)}
	}
}

func withModelEagerLoading(ctx context.Context, q *ent.AiModelQuery) *ent.AiModelQuery {
	if v, ok := ctx.Value(LoadApiKeyModel{}).(bool); ok && v {
		q.WithAiAPIKey()
	}
	return q
}

func (c *modelClient) GetApiKeyByID(ctx context.Context, id int) (*ent.AiApiKey, error) {
	return withApiKeyEagerLoading(ctx, c.client.AiApiKey.Query().Where(aiapikey.ID(id))).First(ctx)
}

func (c *modelClient) GetApiKeys(ctx context.Context, ids []int) ([]*ent.AiApiKey, error) {
	return withApiKeyEagerLoading(ctx, c.client.AiApiKey.Query().Where(aiapikey.IDIn(ids...))).All(ctx)
}

func (c *modelClient) ListApiKeys(ctx context.Context, args *ListApiKeyArgs) (*ListApiKeyResult, error) {
	q := c.client.AiApiKey.Query()
	if args.Name != "" {
		q.Where(aiapikey.NameContainsFold(args.Name))
	}

	if args.Platform != "" {
		q.Where(aiapikey.Platform(args.Platform))
	}

	if args.Status != "" {
		q.Where(aiapikey.StatusEQ(args.Status))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, err
	}

	q.Order(getApiKeyOrderOption(args)...)
	apiKeys, err := withApiKeyEagerLoading(ctx, q).Limit(int(args.PageSize)).Offset(int(args.Page * args.PageSize)).All(ctx)
	if err != nil {
		return nil, err
	}

	return &ListApiKeyResult{
		PaginationResults: &pb.PaginationResults{
			Page:       args.Page,
			PageSize:   args.PageSize,
			TotalItems: int32(total),
		},
		ApiKeys: apiKeys,
	}, nil
}

func (c *modelClient) UpsertApiKey(ctx context.Context, key *ent.AiApiKey) (*ent.AiApiKey, error) {
	if key.ID == 0 {
		query := c.client.AiApiKey.Create().
			SetName(key.Name).
			SetPlatform(key.Platform).
			SetAPIKey(key.APIKey).
			SetURL(key.URL).
			SetStatus(key.Status)
		return query.Save(ctx)
	}

	q := c.client.AiApiKey.UpdateOne(key).
		SetName(key.Name).
		SetPlatform(key.Platform).
		SetAPIKey(key.APIKey).
		SetURL(key.URL).
		SetStatus(key.Status)
	return q.Save(ctx)
}

func (c *modelClient) DeleteApiKey(ctx context.Context, kid int) error {
	// AiModel
	if _, err := c.client.AiModel.Delete().Where(aimodel.KeyID(kid)).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete ai model fot keyID %d: %w", kid, err)
	}
	// AiApiKey
	if _, err := c.client.AiApiKey.Delete().Where(aiapikey.ID(kid)).Exec(ctx); err != nil {
		return err
	}
	return nil
}

func (c *modelClient) DeleteApiKeys(ctx context.Context, ids []int) (int, error) {
	// AiModel
	if _, err := c.client.AiModel.Delete().Where(aimodel.KeyIDIn(ids...)).Exec(ctx); err != nil {
		return 0, fmt.Errorf("failed to delete ai model for keyIDs %v: %w", ids, err)
	}
	return c.client.AiApiKey.Delete().Where(aiapikey.IDIn(ids...)).Exec(ctx)
}

func getApiKeyOrderOption(args *ListApiKeyArgs) []aiapikey.OrderOption {
	orderTerm := db.GetOrderTerm(db.OrderDirection(args.OrderDirection))
	switch args.OrderBy {
	case aiapikey.FieldUpdatedAt:
		return []aiapikey.OrderOption{aiapikey.ByUpdatedAt(orderTerm), aiapikey.ByID(orderTerm)}
	default:
		return []aiapikey.OrderOption{aiapikey.ByID(orderTerm)}
	}
}

func withApiKeyEagerLoading(ctx context.Context, q *ent.AiApiKeyQuery) *ent.AiApiKeyQuery {
	if v, ok := ctx.Value(LoadApiKeyModel{}).(bool); ok && v {
		q.WithAiModel()
	}
	return q
}
