package model

import (
	"ai/ent"
	"ai/internal/biz/types"
	"ai/internal/data"
	"ai/internal/pkg/eino/model"
	"api/external/trans"
	"common/constants"
	"common/hashid"
	"context"
	"entmodule"
	"fmt"

	emodel "github.com/cloudwego/eino/components/model"
	"github.com/samber/lo"
)

type (
	ModelBiz interface {
		GetActiveModel(ctx context.Context, mid int) (*ChatModel, error)
		CreateModel(ctx context.Context, model *ent.AiModel) (*ent.AiModel, error)
		UpdateModel(ctx context.Context, model *ent.AiModel) (*ent.AiModel, error)
		DeleteModel(ctx context.Context, id int) error
		BatchDeleteModels(ctx context.Context, ids []int) ([]int, error)
		GetModel(ctx context.Context, id int) (*ent.AiModel, error)
		GetDefaultModel(ctx context.Context, modelType string) (*ent.AiModel, error)
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
		mm     model.AiModelManager
	}

	ChatModel struct {
		DBModel   *ent.AiModel
		ChatModel emodel.ToolCallingChatModel
	}
)

func NewModelBiz(mc data.ModelClient, hasher hashid.Encoder, mm model.AiModelManager) ModelBiz {
	return &modelBiz{
		mc:     mc,
		hasher: hasher,
		mm:     mm,
	}
}

func (b *modelBiz) GetActiveModel(ctx context.Context, mid int) (*ChatModel, error) {
	ctx = context.WithValue(ctx, data.LoadApiKeyModel{}, true)
	m, err := b.mc.GetActiveModelByIDType(ctx, mid, types.ModelTypeChat)
	if err != nil {
		return nil, fmt.Errorf("failed to get model: %w", err)
	}

	cfg := &model.ModelConfig{
		Platform: model.Platform(m.Platform),
		APIKey:   m.Edges.AiAPIKey.APIKey,
		Model:    m.Model,
	}
	if m.Edges.AiAPIKey.APIKey != "" {
		cfg.APIKey = m.Edges.AiAPIKey.APIKey
	}
	chatModel, err := b.mm.GetModel(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get model: %w", err)
	}
	return &ChatModel{
		DBModel:   m,
		ChatModel: chatModel,
	}, nil
}

func (b *modelBiz) CreateModel(ctx context.Context, model *ent.AiModel) (*ent.AiModel, error) {
	model.Status = entmodule.StatusActive
	apiKey, err := b.mc.GetApiKeyByID(ctx, model.KeyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get api key: %w", err)
	}
	model.Platform = apiKey.Platform
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
	m, err := b.mc.GetModelByID(ctx, id)
	if err != nil {
		return nil, err
	}
	u := trans.FromContext(ctx)
	if m.Status != entmodule.StatusActive && !u.Group.Permissions.Enabled(int(constants.GroupPermissionIsAdmin)) {
		return nil, fmt.Errorf("model is not available")
	}
	return m, nil
}

func (b *modelBiz) GetDefaultModel(ctx context.Context, modelType string) (*ent.AiModel, error) {
	return b.mc.GetDefaultModel(ctx, modelType)
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
	return m, nil
}

func (b *modelBiz) validateApiKey(ctx context.Context, id int) (*ent.AiApiKey, error) {
	key, err := b.validateApiKey(ctx, id)
	if err != nil {
		return nil, err
	}
	return key, nil
}
