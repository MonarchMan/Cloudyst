package service

import (
	"ai/ent"
	"ai/internal/biz/image"
	"ai/internal/biz/knowledge"
	"ai/internal/biz/model"
	"ai/internal/biz/types"
	"ai/internal/data"
	"ai/internal/pkg/utils"
	"api/external/trans"
	"context"
	"entmodule"
	mschema "entmodule/ent/schema"
	"strconv"

	pbadmin "api/api/ai/admin/v1"
	aipb "api/api/ai/common/v1"
	commonpb "api/api/common/v1"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	apiKeyNameCondition     = "name"
	apiKeyPlatformCondition = "platform"
	apiKeyStatusCondition   = "status"

	modelNameCondition     = "name"
	modelPlatformCondition = "model_id"
	modelStatusCondition   = "status"

	roleNameCondition         = "name"
	roleStatusCondition       = "status"
	rolePublicStatusCondition = "public_status"
	roleUserIDCondition       = "user_id"
	roleCategoryCondition     = "category"

	knowledgeNameCondition    = "name"
	knowledgeModelIDCondition = "model_id"
	knowledgeStatusCondition  = "status"

	documentKnowledgeIDCondition = "knowledge_id"
	documentNameCondition        = "name"

	imagePlatformCondition = "platform"
	imageStatusCondition   = "status"
	imageModelIDCondition  = "model_id"
	statusCondition        = "status"
)

type AdminService struct {
	pbadmin.UnimplementedAdminServer
	roleClient data.RoleClient
	kb         knowledge.KnowledgeBiz
	ib         image.ImageBiz
	mb         model.ModelBiz
}

func NewAdminService(kb knowledge.KnowledgeBiz, ib image.ImageBiz, mb model.ModelBiz, roleClient data.RoleClient) *AdminService {
	return &AdminService{
		roleClient: roleClient,
		kb:         kb,
		ib:         ib,
		mb:         mb,
	}
}

func (s *AdminService) CreateApikey(ctx context.Context, req *aipb.AiApiKey) (*aipb.AiApiKey, error) {
	key, err := s.mb.CreateApiKey(ctx, utils.ProtoApiKeyToEnt(req))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create apikey: %w", err)
	}
	return utils.EntApiKeyToProto(key), nil
}
func (s *AdminService) UpdateApikey(ctx context.Context, req *aipb.AiApiKey) (*aipb.AiApiKey, error) {
	key, err := s.mb.UpdateApiKey(ctx, utils.ProtoApiKeyToEnt(req))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to update apikey: %w", err)
	}
	return utils.EntApiKeyToProto(key), nil
}
func (s *AdminService) DeleteApikey(ctx context.Context, req *pbadmin.DeleteRequest) (*emptypb.Empty, error) {
	if req.Force {
		ctx = mschema.SkipSoftDelete(ctx)
	}
	if err := s.mb.DeleteApiKey(ctx, int(req.Id)); err != nil {
		return nil, commonpb.ErrorDb("Failed to delete apikey: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *AdminService) GetApikey(ctx context.Context, req *pbadmin.SimpleRequest) (*aipb.AiApiKey, error) {
	newCtx := context.WithValue(ctx, data.LoadApiKeyModel{}, true)
	key, err := s.mb.GetApiKey(newCtx, int(req.Id))
	return utils.EntApiKeyToProto(key), err
}
func (s *AdminService) ListApikey(ctx context.Context, req *commonpb.ListRequest) (*pbadmin.ListApikeyResponse, error) {
	newCtx := context.WithValue(ctx, data.LoadApiKeyModel{}, true)
	args := &data.ListApiKeyArgs{
		PaginationArgs: &commonpb.PaginationArgs{
			Page:           req.Page - 1,
			PageSize:       req.PageSize,
			OrderBy:        req.OrderBy,
			OrderDirection: req.OrderDirection,
		},
		Name:     req.Conditions[apiKeyNameCondition],
		Platform: req.Conditions[apiKeyPlatformCondition],
		Status:   entmodule.Status(req.Conditions[apiKeyStatusCondition]),
	}
	res, err := s.mb.ListApiKeys(newCtx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list apikey: %w", err)
	}
	return &pbadmin.ListApikeyResponse{
		ApiKeys: lo.Map(res.ApiKeys, func(item *ent.AiApiKey, index int) *aipb.AiApiKey {
			return utils.EntApiKeyToProto(item)
		}),
		Pagination: res.PaginationResults,
	}, nil
}
func (s *AdminService) BatchDeleteApiKey(ctx context.Context, req *pbadmin.BatchDeleteRequest) (*emptypb.Empty, error) {
	if len(req.Ids) == 0 {
		return &emptypb.Empty{}, nil
	}
	if req.Force {
		ctx = mschema.SkipSoftDelete(ctx)
	}
	ids := lo.Map(req.Ids, func(item int64, index int) int {
		return int(item)
	})
	if _, err := s.mb.BatchDeleteApiKeys(ctx, ids); err != nil {
		return nil, commonpb.ErrorDb("Failed to batch delete apikey: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *AdminService) CreateModel(ctx context.Context, req *aipb.AiModel) (*aipb.AiModel, error) {
	model, err := s.mb.CreateModel(ctx, utils.ProtoModelToEnt(req))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create model: %w", err)
	}
	return utils.EntModelToProto(model), nil
}
func (s *AdminService) UpdateModel(ctx context.Context, req *aipb.AiModel) (*aipb.AiModel, error) {
	model, err := s.mb.UpdateModel(ctx, utils.ProtoModelToEnt(req))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to update model: %w", err)
	}
	return utils.EntModelToProto(model), nil
}
func (s *AdminService) DeleteModel(ctx context.Context, req *pbadmin.DeleteRequest) (*emptypb.Empty, error) {
	if req.Force {
		ctx = mschema.SkipSoftDelete(ctx)
	}
	if err := s.mb.DeleteModel(ctx, int(req.Id)); err != nil {
		return nil, commonpb.ErrorDb("Failed to delete model: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *AdminService) GetModel(ctx context.Context, req *pbadmin.SimpleRequest) (*aipb.AiModel, error) {
	newCtx := context.WithValue(ctx, data.LoadApiKeyModel{}, true)
	model, err := s.mb.GetModel(newCtx, int(req.Id))
	return utils.EntModelToProto(model), err
}
func (s *AdminService) ListModel(ctx context.Context, req *commonpb.ListRequest) (*pbadmin.ListModelResponse, error) {
	newCtx := context.WithValue(ctx, data.LoadApiKeyModel{}, true)
	args := &data.ListAiModelArgs{
		PaginationArgs: &commonpb.PaginationArgs{
			Page:           req.Page - 1,
			PageSize:       req.PageSize,
			OrderBy:        req.OrderBy,
			OrderDirection: req.OrderDirection,
		},
		Name:     req.Conditions[modelNameCondition],
		Platform: req.Conditions[modelPlatformCondition],
		Status:   entmodule.Status(req.Conditions[modelStatusCondition]),
	}
	res, err := s.mb.ListModels(newCtx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list model: %w", err)
	}

	return &pbadmin.ListModelResponse{
		Models: lo.Map(res.Models, func(item *ent.AiModel, index int) *aipb.AiModel {
			return utils.EntModelToProto(item)
		}),
		Pagination: res.PaginationResults,
	}, nil
}
func (s *AdminService) BatchDeleteModel(ctx context.Context, req *pbadmin.BatchDeleteRequest) (*emptypb.Empty, error) {
	if len(req.Ids) == 0 {
		return &emptypb.Empty{}, nil
	}
	if req.Force {
		ctx = mschema.SkipSoftDelete(ctx)
	}
	ids := lo.Map(req.Ids, func(item int64, index int) int {
		return int(item)
	})
	if _, err := s.mb.BatchDeleteModels(ctx, ids); err != nil {
		return nil, commonpb.ErrorDb("Failed to batch delete model: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *AdminService) CreateRole(ctx context.Context, req *aipb.AiChatRole) (*aipb.AiChatRole, error) {
	return s.upsertRole(ctx, req)
}
func (s *AdminService) GetRole(ctx context.Context, req *pbadmin.SimpleRequest) (*aipb.AiChatRole, error) {
	// Check if the role exists
	newCtx := context.WithValue(ctx, data.LoadApiKeyModel{}, true)
	role, err := s.roleClient.GetByID(newCtx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get role: %w", err)
	}
	return utils.EntRoleToProto(role), nil
}
func (s *AdminService) ListRole(ctx context.Context, req *commonpb.ListRequest) (*pbadmin.ListRoleResponse, error) {
	args := &data.ListChatRoleArgs{
		PaginationArgs: &commonpb.PaginationArgs{
			Page:           req.Page - 1,
			PageSize:       req.PageSize,
			OrderBy:        req.OrderBy,
			OrderDirection: req.OrderDirection,
		},
		Name:         req.Conditions[roleNameCondition],
		PublicStatus: req.Conditions[rolePublicStatusCondition] == "true",
		Category:     req.Conditions[roleCategoryCondition],
		Status:       entmodule.Status(req.Conditions[roleStatusCondition]),
	}
	var err error
	args.UserID, err = strconv.Atoi(req.Conditions[roleUserIDCondition])
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Failed to parse user_id: %w", err)
	}
	res, err := s.roleClient.List(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list role: %w", err)
	}
	return &pbadmin.ListRoleResponse{
		Roles: lo.Map(res.Roles, func(item *ent.AiChatRole, index int) *aipb.AiChatRole {
			return utils.EntRoleToProto(item)
		}),
		Pagination: res.PaginationResults,
	}, nil
}
func (s *AdminService) BatchDeleteRole(ctx context.Context, req *pbadmin.BatchDeleteRoleRequest) (*emptypb.Empty, error) {
	if len(req.Ids) == 0 {
		return &emptypb.Empty{}, nil
	}
	if req.Force {
		ctx = mschema.SkipSoftDelete(ctx)
	}
	ids := lo.Map(req.Ids, func(item int64, index int) int {
		return int(item)
	})

	kc, tx, ctx, err := data.WithTx(ctx, s.roleClient)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to start transaction: %w", err)
	}
	if num, err := kc.BatchDelete(ctx, ids); err != nil {
		_ = data.Rollback(tx)
		return nil, commonpb.ErrorDb("Failed to batch delete role: %w, num: %d", err, num)
	}

	if err := data.Commit(tx); err != nil {
		return nil, commonpb.ErrorDb("Failed to commit role batch delete: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *AdminService) DeleteKnowledge(ctx context.Context, req *pbadmin.DeleteRequest) (*emptypb.Empty, error) {
	if req.Force {
		ctx = mschema.SkipSoftDelete(ctx)
	}
	err := s.kb.DeleteKnowledge(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to delete knowledge: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *AdminService) GetKnowledge(ctx context.Context, req *pbadmin.SimpleRequest) (*aipb.AiKnowledge, error) {
	k, err := s.kb.GetKnowledge(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get knowledge: %w", err)
	}
	if k == nil {
		return nil, commonpb.ErrorNotFound("Knowledge not found")
	}
	return utils.EntKnowledgeToProto(k), nil
}
func (s *AdminService) ListKnowledge(ctx context.Context, req *commonpb.ListRequest) (*pbadmin.ListKnowledgeResponse, error) {
	args := &data.ListKnowledgeArgs{
		PaginationArgs: &commonpb.PaginationArgs{
			Page:           req.Page - 1,
			PageSize:       req.PageSize,
			OrderBy:        req.OrderBy,
			OrderDirection: req.OrderDirection,
		},
		Name:   req.Conditions[knowledgeNameCondition],
		Status: entmodule.Status(req.Conditions[knowledgeStatusCondition]),
	}
	var err error
	args.ModelID, err = strconv.Atoi(req.Conditions[knowledgeModelIDCondition])
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Failed to parse model_id: %w", err)
	}
	res, err := s.kb.ListKnowledge(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list knowledge: %w", err)
	}
	return &pbadmin.ListKnowledgeResponse{
		Knowledges: lo.Map(res.Knowledges, func(item *ent.AiKnowledge, index int) *aipb.AiKnowledge {
			return utils.EntKnowledgeToProto(item)
		}),
		Pagination: res.PaginationResults,
	}, nil
}
func (s *AdminService) CreateKnowledgeDocument(ctx context.Context, req *aipb.AiKnowledgeDocument) (*aipb.AiKnowledgeDocument, error) {
	args := &data.UpsertDocumentArgs{
		KnowledgeID:      int(req.KnowledgeId),
		Name:             req.Name,
		Url:              req.Url,
		Version:          req.Version,
		ContentLen:       int(req.ContentLength),
		SegmentMaxTokens: int(req.SegmentMaxTokens),
		RetrievalCount:   int(req.RetrievalCount),
	}
	doc, err := s.kb.CreateKnowledgeDocument(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create knowledge document: %w", err)
	}
	return utils.EntKnowledgeDocumentToProto(doc), nil
}
func (s *AdminService) UpdateKnowledgeDocument(ctx context.Context, req *aipb.AiKnowledgeDocument) (*aipb.AiKnowledgeDocument, error) {
	args := &data.UpsertDocumentArgs{
		ID:          int(req.Id),
		KnowledgeID: int(req.KnowledgeId),
		Name:        req.Name,
		Url:         req.Url,
	}
	doc, err := s.kb.UpdateKnowledgeDocument(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to update knowledge document: %w", err)
	}
	return utils.EntKnowledgeDocumentToProto(doc), nil
}
func (s *AdminService) DeleteKnowledgeDocument(ctx context.Context, req *pbadmin.DeleteRequest) (*emptypb.Empty, error) {
	if req.Force {
		ctx = mschema.SkipSoftDelete(ctx)
	}
	err := s.kb.DeleteKnowledgeDocument(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to delete knowledge document: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *AdminService) BatchDeleteKnowledgeDocument(ctx context.Context, req *pbadmin.BatchDeleteRequest) (*emptypb.Empty, error) {
	ids := lo.Map(req.Ids, func(id int64, index int) int {
		return int(id)
	})
	err := s.kb.BatchDeleteKnowledgeDocuments(ctx, ids)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to batch delete knowledge documents: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *AdminService) GetKnowledgeDocument(ctx context.Context, req *pbadmin.SimpleRequest) (*aipb.AiKnowledgeDocument, error) {
	doc, err := s.kb.GetKnowledgeDocument(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get knowledge: %w", err)
	}
	return utils.EntKnowledgeDocumentToProto(doc), nil
}
func (s *AdminService) ListKnowledgeDocument(ctx context.Context, req *commonpb.ListRequest) (*pbadmin.ListKnowledgeDocumentResponse, error) {
	args := &data.ListKnowledgeDocumentArgs{
		PaginationArgs: &commonpb.PaginationArgs{
			Page:           req.Page - 1,
			PageSize:       req.PageSize,
			OrderBy:        req.OrderBy,
			OrderDirection: req.OrderDirection,
		},
		Name: req.Conditions[documentNameCondition],
	}
	status := entmodule.Status(req.Conditions[statusCondition])
	if status == entmodule.StatusActive || status == entmodule.StatusInactive {
		args.Status = status
	}
	var err error
	args.KnowledgeID, err = strconv.Atoi(req.Conditions[documentKnowledgeIDCondition])
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Failed to parse knowledge_id: %w", err)
	}
	res, err := s.kb.ListKnowledgeDocument(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list knowledge document: %w", err)
	}
	return &pbadmin.ListKnowledgeDocumentResponse{
		Documents: lo.Map(res.Documents, func(item *ent.AiKnowledgeDocument, index int) *aipb.AiKnowledgeDocument {
			return utils.EntKnowledgeDocumentToProto(item)
		}),
		Pagination: res.PaginationResults,
	}, nil
}
func (s *AdminService) UpdateDocumentStatus(ctx context.Context, req *pbadmin.UpdateDocumentStatusRequest) (*aipb.AiKnowledgeDocument, error) {
	status := entmodule.Status(req.Status)
	if status != entmodule.StatusActive && status != entmodule.StatusInactive {
		return nil, commonpb.ErrorParamInvalid("Invalid status value: %d", status)
	}
	// Update the document status
	doc, err := s.kb.UpdateDocumentStatus(ctx, int(req.Id), status)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to update document status: %w", err)
	}
	return utils.EntKnowledgeDocumentToProto(doc), nil
}
func (s *AdminService) GetKnowledgeSegment(ctx context.Context, req *pbadmin.SimpleRequest) (*aipb.AiKnowledgeSegment, error) {
	newCtx := context.WithValue(ctx, data.LoadKnowledgeSegment{}, true)
	newCtx = context.WithValue(ctx, data.LoadKnowledgeDocument{}, true)
	segment, err := s.kb.GetKnowledgeSegment(newCtx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get knowledge segment: %w", err)
	}
	return utils.EntKnowledgeSegmentToProto(segment), nil
}
func (s *AdminService) ListImage(ctx context.Context, req *commonpb.ListRequest) (*pbadmin.ListImageResponse, error) {
	args := &data.ListAIImageArgs{
		PaginationArgs: &commonpb.PaginationArgs{
			Page:           req.Page - 1,
			PageSize:       req.PageSize,
			OrderBy:        req.OrderBy,
			OrderDirection: req.OrderDirection,
		},
		Platform: req.Conditions[imagePlatformCondition],
		Status:   types.ImageStatus(req.Conditions[imageStatusCondition]),
	}
	var err error
	args.ModelID, err = strconv.Atoi(req.Conditions[imageModelIDCondition])
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Failed to parse model_id: %w", err)
	}
	res, err := s.ib.ListImages(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list image: %w", err)
	}
	return &pbadmin.ListImageResponse{
		Images: lo.Map(res.Images, func(item *ent.AiImage, index int) *aipb.AiImage {
			return utils.EntImageToProto(item)
		}),
		Pagination: res.PaginationResults,
	}, nil
}
func (s *AdminService) UpdateImagePublicStatus(ctx context.Context, req *pbadmin.UpdateImageStatusRequest) (*aipb.AiImage, error) {
	img, err := s.ib.UpdateImageStatus(ctx, int(req.Id), types.ImageStatus(req.Status))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to update image status: %w", err)
	}
	return utils.EntImageToProto(img), nil
}
func (s *AdminService) BatchDeleteImage(ctx context.Context, req *pbadmin.BatchDeleteRequest) (*emptypb.Empty, error) {
	if len(req.Ids) == 0 {
		return &emptypb.Empty{}, nil
	}

	if req.Force {
		ctx = mschema.SkipSoftDelete(ctx)
	}
	ids := lo.Map(req.Ids, func(item int64, index int) int {
		return int(item)
	})

	_, err := s.ib.BatchDeleteImages(ctx, ids)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to batch delete image: %w", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *AdminService) upsertRole(ctx context.Context, req *aipb.AiChatRole) (*aipb.AiChatRole, error) {
	u := trans.FromContext(ctx)
	req.UserId = u.Id
	r, err := s.roleClient.Upsert(ctx, utils.ProtoRoleToEnt(req))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create role: %w", err)
	}
	return utils.EntRoleToProto(r), nil
}
