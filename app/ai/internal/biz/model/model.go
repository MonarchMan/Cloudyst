package model

import (
	"ai/ent"
	"ai/internal/biz/types"
	"ai/internal/data"
	model2 "ai/internal/pkg/eino/model"
	"common/hashid"
	"context"
	"fmt"

	emodel "github.com/cloudwego/eino/components/model"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/samber/lo"
)

type (
	ModelBiz interface {
		GetActiveModel(ctx context.Context, mid int) (emodel.ToolCallingChatModel, error)
		CreateModel(ctx context.Context, model *ent.AiModel) (*ent.AiModel, error)
		UpdateModel(ctx context.Context, model *ent.AiModel) (*ent.AiModel, error)
		DeleteModel(ctx context.Context, id int) error
		BatchDeleteModels(ctx context.Context, ids []int) ([]int, error)
		GetModel(ctx context.Context, id int) (*ent.AiModel, error)
		ListModels(ctx context.Context, args *data.ListAiModelArgs) (*data.ListAiModelResult, error)

		CreateApiKey(ctx context.Context, req *ent.AiApiKey) (*ent.AiApiKey, error)
		UpdateApiKey(ctx context.Context, req *ent.AiApiKey) (*ent.AiApiKey, error)
		DeleteApiKey(ctx context.Context, id int) error
		BatchDeleteApiKeys(ctx context.Context, ids []int) ([]int, error)
		GetApiKey(ctx context.Context, id int) (*ent.AiApiKey, error)
		ListApiKeys(ctx context.Context, args *data.ListApiKeyArgs) (*data.ListApiKeyResult, error)
		//SendMessage(ctx context.Context, input []*schema.Message, info []*schema.ToolInfo) (*schema.Message, error)
	}

	modelBiz struct {
		mc     data.ModelClient
		hasher hashid.Encoder
		mm     model2.AiModelManager
	}
)

func NewModelBiz(mc data.ModelClient, hasher hashid.Encoder, mm model2.AiModelManager) ModelBiz {
	return &modelBiz{
		mc:     mc,
		hasher: hasher,
		mm:     mm,
	}
}

func (b *modelBiz) GetActiveModel(ctx context.Context, mid int) (emodel.ToolCallingChatModel, error) {
	m, err := b.mc.GetActiveModelByIDType(ctx, mid, types.ModelTypeChat)
	if err != nil || m == nil {
		return nil, fmt.Errorf("failed to get model: %w", err)
	}

	cfg := &model2.ModelConfig{
		Platform: model2.Platform(m.Platform),
		APIKey:   m.Edges.AiAPIKey.APIKey,
		Model:    m.Type,
	}
	if m.Edges.AiAPIKey.APIKey != "" {
		cfg.APIKey = m.Edges.AiAPIKey.APIKey
	}
	return b.mm.GetModel(cfg)
}

func (b *modelBiz) CreateModel(ctx context.Context, model *ent.AiModel) (*ent.AiModel, error) {
	return b.mc.UpsertModel(ctx, model)
}

func (b *modelBiz) UpdateModel(ctx context.Context, model *ent.AiModel) (*ent.AiModel, error) {
	_, err := b.validateModel(ctx, model.ID)
	if err != nil {
		return nil, err
	}
	return b.mc.UpsertModel(ctx, model)
}

func (b *modelBiz) DeleteModel(ctx context.Context, id int) error {
	_, err := b.validateModel(ctx, id)
	if err != nil {
		return err
	}
	return b.mc.DeleteModel(ctx, id)
}

func (b *modelBiz) BatchDeleteModels(ctx context.Context, ids []int) ([]int, error) {
	mids := make([]int, len(ids))
	if len(ids) == 0 {
		return mids, nil
	}
	// query models
	models, err := b.mc.GetModels(ctx, ids)
	if err != nil {
		return mids, fmt.Errorf("failed to get models: %w", err)
	}
	mids = lo.Map(models, func(item *ent.AiModel, index int) int {
		return item.ID
	})
	// batch delete models
	kc, tx, ctx, err := data.WithTx(ctx, b.mc)
	if err != nil {
		return mids, fmt.Errorf("failed to start transaction: %w", err)
	}
	if num, err := kc.DeleteModels(ctx, mids); err != nil {
		_ = data.Rollback(tx)
		return mids, fmt.Errorf("failed to batch delete model: %w, num: %d", err, num)
	}
	if err := data.Commit(tx); err != nil {
		return mids, fmt.Errorf("failed to commit: %w", err)
	}

	return mids, nil
}

func (b *modelBiz) GetModel(ctx context.Context, id int) (*ent.AiModel, error) {
	return b.mc.GetModelByID(ctx, id)
}

func (b *modelBiz) ListModels(ctx context.Context, args *data.ListAiModelArgs) (*data.ListAiModelResult, error) {
	return b.mc.ListModels(ctx, args)
}

func (b *modelBiz) CreateApiKey(ctx context.Context, args *ent.AiApiKey) (*ent.AiApiKey, error) {
	return b.mc.UpsertApiKey(ctx, args)
}

func (b *modelBiz) UpdateApiKey(ctx context.Context, args *ent.AiApiKey) (*ent.AiApiKey, error) {
	_, err := b.validateApiKey(ctx, args.ID)
	if err != nil {
		return nil, err
	}
	return b.mc.UpsertApiKey(ctx, args)
}

func (b *modelBiz) GetApiKey(ctx context.Context, id int) (*ent.AiApiKey, error) {
	return b.mc.GetApiKeyByID(ctx, id)
}

func (b *modelBiz) ListApiKeys(ctx context.Context, args *data.ListApiKeyArgs) (*data.ListApiKeyResult, error) {
	return b.mc.ListApiKeys(ctx, args)
}

func (b *modelBiz) DeleteApiKey(ctx context.Context, id int) error {
	_, err := b.validateApiKey(ctx, id)
	if err != nil {
		return err
	}
	return b.mc.DeleteApiKey(ctx, id)
}

func (b *modelBiz) BatchDeleteApiKeys(ctx context.Context, ids []int) ([]int, error) {
	kids := make([]int, len(ids))
	if len(ids) == 0 {
		return kids, nil
	}
	// query api keys
	apiKeys, err := b.mc.GetApiKeys(ctx, ids)
	if err != nil {
		return kids, fmt.Errorf("failed to get api keys: %w", err)
	}
	kids = lo.Map(apiKeys, func(item *ent.AiApiKey, index int) int {
		return item.ID
	})
	// batch delete api keys
	kc, tx, ctx, err := data.WithTx(ctx, b.mc)
	if err != nil {
		return kids, fmt.Errorf("failed to start transaction: %w", err)
	}
	if num, err := kc.DeleteApiKeys(ctx, kids); err != nil {
		_ = data.Rollback(tx)
		return kids, fmt.Errorf("failed to batch delete api key: %w, num: %d", err, num)
	}
	if err := data.Commit(tx); err != nil {
		return kids, fmt.Errorf("failed to commit: %w", err)
	}
	return kids, nil
}

func (b *modelBiz) validateModel(ctx context.Context, id int) (*ent.AiModel, error) {
	m, err := b.mc.GetModelByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, errors.NotFound("invalid model id", "model not found")
	}
	return m, nil
}

func (b *modelBiz) validateApiKey(ctx context.Context, id int) (*ent.AiApiKey, error) {
	key, err := b.validateApiKey(ctx, id)
	if err != nil {
		return nil, err
	}
	if key == nil {
		return nil, errors.NotFound("invalid api key id", "api key not found")
	}
	return key, nil
}
