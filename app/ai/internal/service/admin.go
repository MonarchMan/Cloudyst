package service

import (
	"ai/ent"
	"ai/ent/schema"
	"ai/internal/biz/chat"
	"ai/internal/biz/image"
	"ai/internal/biz/knowledge"
	"ai/internal/biz/model"
	"ai/internal/biz/queue"
	"ai/internal/biz/types"
	"ai/internal/data"
	"ai/internal/data/rpc"
	"ai/internal/pkg/utils"
	"api/external/data/common"
	"api/external/data/userdata"
	"api/external/trans"
	"common/hashid"
	"common/util"
	"context"
	"entmodule"
	mqueue "queue"
	"strconv"
	"time"

	pbadmin "api/api/ai/admin/v1"
	aipb "api/api/ai/common/v1"
	commonpb "api/api/common/v1"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	startCondition = "start"
	endCondition   = "end"

	apiKeyNameCondition     = "name"
	apiKeyPlatformCondition = "platform"
	apiKeyStatusCondition   = "status"

	modelNameCondition     = "name"
	modelPlatformCondition = "platform"
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

	conversationTitleCondition   = "title"
	conversationPinnedCondition  = "pinned"
	conversationUserIDCondition  = "user_id"
	conversationRoleIDCondition  = "role_id"
	conversationModelIDCondition = "model_id"

	messageConversationIDCondition = "conversation_id"
	messageUserIDCondition         = "user_id"
	messageRoleIDCondition         = "role_id"
	messageModelIDCondition        = "model_id"
	messageTypeCondition           = "type"

	toolTypeCondition   = "type"
	toolStatusCondition = "status"
	toolNameCondition   = "name"

	taskTypeCondition    = "task_type"
	taskStatusCondition  = "task_status"
	taskTraceIDCondition = "task_correlation_id"
	taskUserIDCondition  = "task_user_id"
)

type AdminService struct {
	pbadmin.UnimplementedAdminServer
	roleClient data.RoleClient
	kb         knowledge.KnowledgeBiz
	ib         image.ImageBiz
	mb         model.ModelBiz
	cb         chat.ChatBiz
	tc         data.ToolClient
	uc         rpc.UserClient
	taskClient data.TaskClient
	qm         *queue.QueueManager
	hasher     hashid.Encoder
}

func NewAdminService(kb knowledge.KnowledgeBiz, ib image.ImageBiz, mb model.ModelBiz, cb chat.ChatBiz, roleClient data.RoleClient,
	uc rpc.UserClient, hasher hashid.Encoder) *AdminService {
	return &AdminService{
		roleClient: roleClient,
		kb:         kb,
		ib:         ib,
		mb:         mb,
		cb:         cb,
		uc:         uc,
		hasher:     hasher,
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
func (s *AdminService) DeleteApikey(ctx context.Context, req *pbadmin.SimpleRequest) (*emptypb.Empty, error) {
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
		PaginationArgs: &common.PaginationArgs{
			Page:     int(req.Page - 1),
			PageSize: int(req.PageSize),
			OrderBy:  req.OrderBy,
			OrderDir: common.OrderDirection(req.OrderDirection),
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
		Pagination: common.PaginationResultsToProto(res.PaginationResults),
	}, nil
}
func (s *AdminService) BatchDeleteApiKey(ctx context.Context, req *pbadmin.BatchDeleteRequest) (*emptypb.Empty, error) {
	if len(req.Ids) == 0 {
		return &emptypb.Empty{}, nil
	}
	if req.Force {
		ctx = schema.SkipSoftDelete(ctx)
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
func (s *AdminService) DeleteModel(ctx context.Context, req *pbadmin.SimpleRequest) (*emptypb.Empty, error) {
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
		PaginationArgs: &common.PaginationArgs{
			Page:     int(req.Page - 1),
			PageSize: int(req.PageSize),
			OrderBy:  req.OrderBy,
			OrderDir: common.OrderDirection(req.OrderDirection),
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
		Pagination: common.PaginationResultsToProto(res.PaginationResults),
	}, nil
}
func (s *AdminService) BatchDeleteModel(ctx context.Context, req *pbadmin.BatchDeleteRequest) (*emptypb.Empty, error) {
	if len(req.Ids) == 0 {
		return &emptypb.Empty{}, nil
	}
	if req.Force {
		ctx = schema.SkipSoftDelete(ctx)
	}
	ids := lo.Map(req.Ids, func(item int64, index int) int {
		return int(item)
	})
	if _, err := s.mb.BatchDeleteModels(ctx, ids); err != nil {
		return nil, commonpb.ErrorDb("Failed to batch delete model: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *AdminService) CreateRole(ctx context.Context, req *aipb.AiChatRole) (*pbadmin.Role, error) {
	u := trans.FromContext(ctx)
	req.UserId = int64(u.ID)
	r, err := s.roleClient.Upsert(ctx, utils.ProtoRoleToEnt(req))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create role: %w", err)
	}
	return &pbadmin.Role{
		Role:      utils.EntRoleToProto(r),
		OwnerInfo: userdata.UserToProtoUserInfo(s.hasher, u),
	}, nil
}
func (s *AdminService) GetRole(ctx context.Context, req *pbadmin.SimpleRequest) (*pbadmin.Role, error) {
	// Check if the role exists
	newCtx := context.WithValue(ctx, data.LoadApiKeyModel{}, true)
	role, err := s.roleClient.GetByID(newCtx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get role: %w", err)
	}

	// Get the owner info
	u, err := s.uc.GetUserInfo(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get user info: %w", err)
	}

	return &pbadmin.Role{
		Role:      utils.EntRoleToProto(role),
		OwnerInfo: userdata.UserInfoFromProtoUser(s.hasher, u),
	}, nil
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

	// Get user infos
	uids := lo.Map(res.Roles, func(item *ent.AiChatRole, index int) int {
		return item.UserID
	})
	userMap, err := s.uc.GetUserInfos(ctx, uids)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list user infos: %w", err)
	}
	return &pbadmin.ListRoleResponse{
		Roles: lo.Map(res.Roles, func(item *ent.AiChatRole, index int) *pbadmin.Role {
			r := &pbadmin.Role{
				Role: utils.EntRoleToProto(item),
			}
			if u, ok := userMap[item.UserID]; ok {
				r.OwnerInfo = userdata.UserInfoFromProtoUser(s.hasher, u)
			} else {
				r.OwnerInfo = &commonpb.UserInfo{Id: hashid.EncodeID(s.hasher, item.UserID, hashid.UserID)}
			}
			return r
		}),
		Pagination: res.PaginationResults,
	}, nil
}
func (s *AdminService) BatchDeleteRole(ctx context.Context, req *pbadmin.BatchDeleteRoleRequest) (*emptypb.Empty, error) {
	if len(req.Ids) == 0 {
		return &emptypb.Empty{}, nil
	}
	if req.Force {
		ctx = schema.SkipSoftDelete(ctx)
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
func (s *AdminService) UpdateRole(ctx context.Context, req *aipb.AiChatRole) (*pbadmin.Role, error) {
	r, err := s.roleClient.Upsert(ctx, utils.ProtoRoleToEnt(req))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to update role: %w", err)
	}

	// Get the owner info
	u, err := s.uc.GetUserInfo(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get user info: %w", err)
	}
	return &pbadmin.Role{
		Role:      utils.EntRoleToProto(r),
		OwnerInfo: userdata.UserInfoFromProtoUser(s.hasher, u),
	}, nil
}
func (s *AdminService) GetKnowledge(ctx context.Context, req *pbadmin.SimpleRequest) (*pbadmin.Knowledge, error) {
	k, err := s.kb.GetKnowledge(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get knowledge: %w", err)
	}
	if k == nil {
		return nil, commonpb.ErrorNotFound("Knowledge not found")
	}
	u, err := s.uc.GetUserInfo(ctx, k.UserID)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get user info: %w", err)
	}
	return &pbadmin.Knowledge{
		Knowledge: utils.EntKnowledgeToProto(k),
		OwnerInfo: userdata.UserInfoFromProtoUser(s.hasher, u),
	}, nil
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
	// get user infos
	uids := lo.Map(res.Knowledges, func(k *ent.AiKnowledge, _ int) int {
		return k.UserID
	})
	userMap, err := s.uc.GetUserInfos(ctx, uids)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list user infos: %w", err)
	}
	return &pbadmin.ListKnowledgeResponse{
		Knowledges: lo.Map(res.Knowledges, func(item *ent.AiKnowledge, index int) *pbadmin.Knowledge {
			k := &pbadmin.Knowledge{
				Knowledge: utils.EntKnowledgeToProto(item),
			}
			if u, ok := userMap[item.UserID]; ok {
				k.OwnerInfo = userdata.UserInfoFromProtoUser(s.hasher, u)
			} else {
				k.OwnerInfo = &commonpb.UserInfo{Id: hashid.EncodeID(s.hasher, item.UserID, hashid.UserID)}
			}
			return k
		}),
		Pagination: res.PaginationResults,
	}, nil
}
func (s *AdminService) BatchDeleteKnowledge(ctx context.Context, req *pbadmin.BatchDeleteRequest) (*emptypb.Empty, error) {
	if len(req.Ids) == 0 {
		return &emptypb.Empty{}, nil
	}
	if req.Force {
		ctx = schema.SkipSoftDelete(ctx)
	}
	ids := lo.Map(req.Ids, func(item int64, index int) int {
		return int(item)
	})
	err := s.kb.BatchDeleteKnowledges(ctx, ids)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to batch delete knowledge: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *AdminService) CreateKnowledge(ctx context.Context, req *aipb.AiKnowledge) (*pbadmin.Knowledge, error) {
	u := trans.FromContext(ctx)
	args := &data.UpsertKnowledgeArgs{
		UserID:      u.ID,
		Name:        req.Name,
		Description: req.Description,
		IsPublic:    req.IsPublic,
		IsMaster:    req.IsMaster,
	}
	k, err := s.kb.CreateKnowledge(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create knowledge: %w", err)
	}
	return &pbadmin.Knowledge{
		Knowledge: utils.EntKnowledgeToProto(k),
		OwnerInfo: userdata.UserToProtoUserInfo(s.hasher, u),
	}, nil
}
func (s *AdminService) UpdateKnowledge(ctx context.Context, req *aipb.AiKnowledge) (*pbadmin.Knowledge, error) {
	args := &data.UpsertKnowledgeArgs{
		ID:          int(req.Id),
		Name:        req.Name,
		Description: req.Description,
		IsPublic:    req.IsPublic,
		IsMaster:    req.IsMaster,
	}
	k, err := s.kb.UpdateKnowledge(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to update knowledge: %w", err)
	}

	// Get user info
	u, err := s.uc.GetUserInfo(ctx, k.UserID)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get user info: %w", err)
	}
	return &pbadmin.Knowledge{
		Knowledge: utils.EntKnowledgeToProto(k),
		OwnerInfo: userdata.UserInfoFromProtoUser(s.hasher, u),
	}, nil
}
func (s *AdminService) CreateKnowledgeDocument(ctx context.Context, req *aipb.AiKnowledgeDocument) (*pbadmin.UpsertDocumentResponse, error) {
	args := &data.UpsertDocumentArgs{
		KnowledgeID:      int(req.KnowledgeId),
		Name:             req.Name,
		Url:              req.Url,
		Version:          req.Version,
		ContentLen:       int(req.ContentLength),
		SegmentMaxTokens: int(req.SegmentMaxTokens),
		RetrievalCount:   int(req.RetrievalCount),
	}
	res, err := s.kb.CreateKnowledgeDocument(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create knowledge document: %w", err)
	}
	return buildAdminUpsertDocumentResponse(res.Document, res.Task.ID()), nil
}
func (s *AdminService) UpdateKnowledgeDocument(ctx context.Context, req *aipb.AiKnowledgeDocument) (*pbadmin.UpsertDocumentResponse, error) {
	args := &data.UpsertDocumentArgs{
		ID:          int(req.Id),
		KnowledgeID: int(req.KnowledgeId),
		Name:        req.Name,
		Url:         req.Url,
	}
	res, err := s.kb.UpdateKnowledgeDocument(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to update knowledge document: %w", err)
	}
	return buildAdminUpsertDocumentResponse(res.Document, res.Task.ID()), nil
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
func (s *AdminService) GetImage(ctx context.Context, req *pbadmin.SimpleRequest) (*pbadmin.Image, error) {
	img, err := s.ib.GetImage(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get image: %w", err)
	}
	// Get user info
	u, err := s.uc.GetUserInfo(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get user info: %w", err)
	}
	return &pbadmin.Image{
		Image:     utils.EntImageToProto(img),
		OwnerInfo: userdata.UserInfoFromProtoUser(s.hasher, u),
	}, nil
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

	// get user infos
	uids := lo.Map(res.Images, func(k *ent.AiImage, _ int) int {
		return k.UserID
	})
	userMap, err := s.uc.GetUserInfos(ctx, uids)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list user infos: %w", err)
	}
	return &pbadmin.ListImageResponse{
		Images: lo.Map(res.Images, func(item *ent.AiImage, index int) *pbadmin.Image {
			img := &pbadmin.Image{
				Image: utils.EntImageToProto(item),
			}
			if u, ok := userMap[item.UserID]; ok {
				img.OwnerInfo = userdata.UserInfoFromProtoUser(s.hasher, u)
			} else {
				img.OwnerInfo = &commonpb.UserInfo{Id: hashid.EncodeID(s.hasher, item.UserID, hashid.UserID)}
			}
			return img
		}),
		Pagination: res.PaginationResults,
	}, nil
}
func (s *AdminService) UpdateImage(ctx context.Context, req *aipb.AiImage) (*pbadmin.Image, error) {
	args := &data.UpsertImageArgs{
		ID:     int(req.Id),
		Status: types.ImageStatus(req.Status),
	}
	img, err := s.ib.UpdateImage(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to update image: %w", err)
	}
	// Get user info
	u, err := s.uc.GetUserInfo(ctx, img.UserID)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get user info: %w", err)
	}
	return &pbadmin.Image{
		Image:     utils.EntImageToProto(img),
		OwnerInfo: userdata.UserInfoFromProtoUser(s.hasher, u),
	}, nil
}
func (s *AdminService) BatchDeleteImage(ctx context.Context, req *pbadmin.BatchDeleteRequest) (*emptypb.Empty, error) {
	if len(req.Ids) == 0 {
		return &emptypb.Empty{}, nil
	}

	if req.Force {
		ctx = schema.SkipSoftDelete(ctx)
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
func (s *AdminService) GetChatConversation(ctx context.Context, req *pbadmin.SimpleRequest) (*pbadmin.ChatConversation, error) {
	conversation, err := s.cb.GetConversation(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get conversation: %w", err)
	}
	// Get user info
	user, err := s.uc.GetUserInfo(ctx, conversation.UserID)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get user info: %w", err)
	}
	return &pbadmin.ChatConversation{
		Conversation: utils.EntConversationToProto(conversation),
		OwnerInfo:    userdata.UserInfoFromProtoUser(s.hasher, user),
	}, nil
}

func (s *AdminService) ListChatConversation(ctx context.Context, req *commonpb.ListRequest) (*pbadmin.ListChatConversationResponse, error) {
	args := getListChatConversationArgs(req)
	res, err := s.cb.ListConversations(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list conversation: %w", err)
	}

	// get user infos
	uids := lo.Map(res.Conversations, func(k *ent.AiChatConversation, _ int) int {
		return k.UserID
	})
	userMap, err := s.uc.GetUserInfos(ctx, uids)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list user infos: %w", err)
	}
	return &pbadmin.ListChatConversationResponse{
		Conversations: lo.Map(res.Conversations, func(item *ent.AiChatConversation, index int) *pbadmin.ChatConversation {
			c := &pbadmin.ChatConversation{
				Conversation: utils.EntConversationToProto(item),
			}
			if u, ok := userMap[item.UserID]; ok {
				c.OwnerInfo = userdata.UserInfoFromProtoUser(s.hasher, u)
			} else {
				c.OwnerInfo = &commonpb.UserInfo{Id: hashid.EncodeID(s.hasher, item.UserID, hashid.UserID)}
			}
			return c
		}),
		Pagination: common.PaginationResultsToProto(res.PaginationResults),
	}, nil
}

func (s *AdminService) BatchDeleteChatConversation(ctx context.Context, req *pbadmin.BatchDeleteRequest) (*emptypb.Empty, error) {
	if len(req.Ids) == 0 {
		return &emptypb.Empty{}, nil
	}
	if req.Force {
		ctx = schema.SkipSoftDelete(ctx)
	}
	ids := lo.Map(req.Ids, func(item int64, index int) int {
		return int(item)
	})
	err := s.cb.BatchDeleteConversations(ctx, ids)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to batch delete conversation: %w", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *AdminService) GetChatMessage(ctx context.Context, req *pbadmin.SimpleRequest) (*pbadmin.ChatMessage, error) {
	message, err := s.cb.GetMessage(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get message: %w", err)
	}
	// Get user info
	user, err := s.uc.GetUserInfo(ctx, message.UserID)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get user info: %w", err)
	}
	return &pbadmin.ChatMessage{
		Message:   utils.EntMessageToProto(message),
		OwnerInfo: userdata.UserInfoFromProtoUser(s.hasher, user),
	}, nil
}

func (s *AdminService) ListChatMessage(ctx context.Context, req *commonpb.ListRequest) (*pbadmin.ListChatMessageResponse, error) {
	args := getListChatMessageArgs(req)
	res, err := s.cb.ListMessages(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list message: %w", err)
	}
	// get user infos
	uids := lo.Map(res.ChatMessages, func(k *ent.AiChatMessage, _ int) int {
		return k.UserID
	})
	userMap, err := s.uc.GetUserInfos(ctx, uids)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list user infos: %w", err)
	}
	return &pbadmin.ListChatMessageResponse{
		Messages: lo.Map(res.ChatMessages, func(item *ent.AiChatMessage, index int) *pbadmin.ChatMessage {
			m := &pbadmin.ChatMessage{
				Message: utils.EntMessageToProto(item),
			}
			if u, ok := userMap[item.UserID]; ok {
				m.OwnerInfo = userdata.UserInfoFromProtoUser(s.hasher, u)
			} else {
				m.OwnerInfo = &commonpb.UserInfo{Id: hashid.EncodeID(s.hasher, item.UserID, hashid.UserID)}
			}
			return m
		}),
		Pagination: common.PaginationResultsToProto(res.PaginationResults),
	}, nil
}

func (s *AdminService) BatchDeleteChatMessage(ctx context.Context, req *pbadmin.BatchDeleteRequest) (*emptypb.Empty, error) {
	if len(req.Ids) == 0 {
		return &emptypb.Empty{}, nil
	}
	if req.Force {
		ctx = schema.SkipSoftDelete(ctx)
	}
	ids := lo.Map(req.Ids, func(item int64, index int) int {
		return int(item)
	})
	err := s.cb.BatchDeleteMessages(ctx, ids)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to batch delete message: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *AdminService) CreateTool(ctx context.Context, req *aipb.AiTool) (*aipb.AiTool, error) {
	tool, err := s.tc.Upsert(ctx, utils.ProtoToolToEnt(req))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to upsert tool: %w", err)
	}
	return utils.EntToolToProto(tool), nil
}
func (s *AdminService) UpdateTool(ctx context.Context, req *aipb.AiTool) (*aipb.AiTool, error) {
	tool, err := s.tc.Upsert(ctx, utils.ProtoToolToEnt(req))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to upsert tool: %w", err)
	}
	return utils.EntToolToProto(tool), nil
}
func (s *AdminService) DeleteTool(ctx context.Context, req *pbadmin.SimpleRequest) (*emptypb.Empty, error) {
	err := s.tc.Delete(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to delete tool: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *AdminService) BatchDeleteTool(ctx context.Context, req *pbadmin.BatchDeleteRequest) (*emptypb.Empty, error) {
	ids := lo.Map(req.Ids, func(item int64, index int) int {
		return int(item)
	})
	_, err := s.tc.BatchDelete(ctx, ids)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to batch delete tool: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *AdminService) GetTool(ctx context.Context, req *pbadmin.SimpleRequest) (*aipb.AiTool, error) {
	tool, err := s.tc.GetByID(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get tool: %w", err)
	}
	return utils.EntToolToProto(tool), nil
}
func (s *AdminService) ListTool(ctx context.Context, req *commonpb.ListRequest) (*pbadmin.ListToolResponse, error) {
	args := getListToolArgs(req)
	res, err := s.tc.List(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list tool: %w", err)
	}
	return &pbadmin.ListToolResponse{
		Tools: lo.Map(res.Tools, func(item *ent.AiTool, index int) *aipb.AiTool {
			return utils.EntToolToProto(item)
		}),
		Pagination: common.PaginationResultsToProto(res.PaginationResults),
	}, nil
}

func (s *AdminService) GetQueueMetrics(ctx context.Context, req *emptypb.Empty) (*commonpb.QueueMetricsResponse, error) {
	var res []*commonpb.QueueMetric

	ingest := s.qm.IngestQueue()
	reindex := s.qm.ReindexQueue()

	res = append(res, &commonpb.QueueMetric{
		Name:            queue.IngestTaskType,
		BusyWorkers:     ingest.BusyWorkers(),
		SuccessTasks:    ingest.SuccessTasks(),
		FailedTasks:     ingest.FailureTasks(),
		SubmittedTasks:  ingest.SubmittedTasks(),
		SuspendingTasks: ingest.SuspendingTasks(),
	})
	res = append(res, &commonpb.QueueMetric{
		Name:            queue.ReindexTaskType,
		BusyWorkers:     reindex.BusyWorkers(),
		SuccessTasks:    reindex.SuccessTasks(),
		FailedTasks:     reindex.FailureTasks(),
		SubmittedTasks:  reindex.SubmittedTasks(),
		SuspendingTasks: reindex.SuspendingTasks(),
	})

	return &commonpb.QueueMetricsResponse{
		Metrics: res,
	}, nil
}
func (s *AdminService) ListTasks(ctx context.Context, req *commonpb.ListRequest) (*commonpb.ListTaskResponse, error) {
	var (
		err      error
		userID   int
		traceID  string
		status   []mqueue.TaskStatus
		taskType []string
	)

	if req.Conditions[taskTypeCondition] != "" {
		taskType = []string{req.Conditions[taskTypeCondition]}
	}

	if req.Conditions[taskStatusCondition] != "" {
		status = []mqueue.TaskStatus{mqueue.TaskStatus(req.Conditions[taskStatusCondition])}
	}

	if req.Conditions[taskTraceIDCondition] != "" {
		traceID = util.TraceID(ctx)
	}

	if req.Conditions[taskUserIDCondition] != "" {
		userID, err = strconv.Atoi(req.Conditions[taskUserIDCondition])
		if err != nil {
			return nil, commonpb.ErrorParamInvalid("Invalid task users ID: %w", err)
		}
	}

	res, err := s.taskClient.List(ctx, &data.ListTaskArgs{
		PaginationArgs: &common.PaginationArgs{
			Page:     int(req.Page) - 1,
			PageSize: int(req.PageSize),
			OrderBy:  req.OrderBy,
			OrderDir: common.OrderDirection(req.OrderDirection),
		},
		UserID:  userID,
		TraceID: traceID,
		Types:   taskType,
		Status:  status,
	})

	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list tasks: %w", err)
	}
	return buildAdminListTasksResponse(s.hasher, res), nil
}
func (s *AdminService) GetTask(ctx context.Context, req *pbadmin.SimpleRequest) (*commonpb.GetTaskResponse, error) {
	task, err := s.taskClient.GetTaskByID(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to query task: %w", err)
	}

	var (
		userHashID string
	)

	if task.UserID > 0 {
		userHashID = hashid.EncodeUserID(s.hasher, task.UserID)
	}

	return &commonpb.GetTaskResponse{
		Task:       utils.EntTaskToProto(task),
		TaskHashId: hashid.EncodeTaskID(s.hasher, task.ID),
		UserHashId: userHashID,
	}, nil
}
func (s *AdminService) BatchDeleteTasks(ctx context.Context, req *pbadmin.BatchDeleteRequest) (*emptypb.Empty, error) {
	taskIds := lo.Map(req.Ids, func(id int64, index int) int {
		return int(id)
	})
	err := s.taskClient.DeleteByIDs(ctx, taskIds...)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to batch delete tasks: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *AdminService) CleanupTask(ctx context.Context, req *commonpb.CleanupTaskRequest) (*emptypb.Empty, error) {
	status := lo.Map(req.Status, func(status string, index int) mqueue.TaskStatus {
		return mqueue.TaskStatus(status)
	})
	if len(req.Status) == 0 {
		status = []mqueue.TaskStatus{mqueue.StatusCanceled, mqueue.StatusCompleted, mqueue.StatusError}
	}

	if err := s.taskClient.DeleteBy(ctx, &data.DeleteTaskArgs{
		NotAfter: req.NoAfter.AsTime(),
		Types:    req.Types,
		Status:   status,
	}); err != nil {
		return nil, commonpb.ErrorDb("Failed to cleanup tasks: %w", err)
	}

	return &emptypb.Empty{}, nil
}

func (s *AdminService) DeleteTaskByUserIds(ctx context.Context, req *commonpb.DeleteTaskByUserIdsRequest) (*emptypb.Empty, error) {
	if len(req.Ids) == 0 {
		return nil, commonpb.ErrorParamInvalid("IDs are empty")
	}
	uids := lo.Map(req.Ids, func(id int32, index int) int {
		return int(id)
	})
	if err := s.taskClient.DeleteBy(ctx, &data.DeleteTaskArgs{
		Uids: uids,
	}); err != nil {
		return nil, commonpb.ErrorDb("Failed to delete tasks: %w", err)
	}

	return &emptypb.Empty{}, nil
}

func getListChatConversationArgs(req *commonpb.ListRequest) *data.ListChatConversationArgs {
	args := &data.ListChatConversationArgs{
		PaginationArgs: common.ConvertListRequestPaginationArgs(req),
	}
	if req.Conditions[conversationPinnedCondition] == "true" || req.Conditions[conversationPinnedCondition] == "false" {
		args.Pinned = req.Conditions[conversationPinnedCondition] == "true"
	}
	if req.Conditions[conversationUserIDCondition] != "" {
		args.UserID, _ = strconv.Atoi(req.Conditions[conversationUserIDCondition])
	}
	if req.Conditions[conversationRoleIDCondition] != "" {
		args.RoleID, _ = strconv.Atoi(req.Conditions[conversationRoleIDCondition])
	}
	if req.Conditions[conversationModelIDCondition] != "" {
		args.ModelID, _ = strconv.Atoi(req.Conditions[conversationModelIDCondition])
	}
	if req.Conditions[startCondition] != "" {
		args.Start, _ = time.Parse(time.RFC3339, req.Conditions[startCondition])
	}
	if req.Conditions[endCondition] != "" {
		args.End, _ = time.Parse(time.RFC3339, req.Conditions[endCondition])
	}
	return args
}

func getListChatMessageArgs(req *commonpb.ListRequest) *data.ListChatMessageArgs {
	args := &data.ListChatMessageArgs{
		PaginationArgs: common.ConvertListRequestPaginationArgs(req),
	}
	if req.Conditions[messageConversationIDCondition] != "" {
		args.ConversationID, _ = strconv.Atoi(req.Conditions[messageConversationIDCondition])
	}
	if req.Conditions[messageUserIDCondition] != "" {
		args.UserID, _ = strconv.Atoi(req.Conditions[messageUserIDCondition])
	}
	if req.Conditions[messageRoleIDCondition] != "" {
		args.RoleID, _ = strconv.Atoi(req.Conditions[messageRoleIDCondition])
	}
	if req.Conditions[messageModelIDCondition] != "" {
		args.ModelID, _ = strconv.Atoi(req.Conditions[messageModelIDCondition])
	}
	if req.Conditions[startCondition] != "" {
		args.Start, _ = time.Parse(time.RFC3339, req.Conditions[startCondition])
	}
	if req.Conditions[endCondition] != "" {
		args.End, _ = time.Parse(time.RFC3339, req.Conditions[endCondition])
	}
	return args
}
func getListToolArgs(req *commonpb.ListRequest) *data.ListToolArgs {
	args := &data.ListToolArgs{
		PaginationArgs: common.ConvertListRequestPaginationArgs(req),
	}
	if req.Conditions[toolTypeCondition] != "" {
		args.Type = req.Conditions[toolTypeCondition]
	}
	if req.Conditions[toolNameCondition] != "" {
		args.Name = req.Conditions[toolNameCondition]
	}
	if req.Conditions[toolStatusCondition] != "" {
		args.Status = entmodule.Status(req.Conditions[toolStatusCondition])
	}
	return args
}
