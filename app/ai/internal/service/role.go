package service

import (
	"ai/ent"
	"ai/internal/data"
	pb "api/api/ai/role/v1"
	commonpb "api/api/common/v1"
	"api/external/trans"
	"common/hashid"
	"context"
	"entmodule"
	"strings"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
)

type RoleService struct {
	pb.UnimplementedRoleServer
	hasher hashid.Encoder
	rc     data.RoleClient
	kc     data.KnowledgeClient
	tc     data.ToolClient
}

func NewRoleService() *RoleService {
	return &RoleService{}
}

func (s *RoleService) CreateRole(ctx context.Context, req *pb.UpsertRoleRequest) (*pb.GetRoleResponse, error) {
	u := trans.FromContext(ctx)
	// 1.1 校验知识库是否存在
	kids, err := s.getKnowledgeIDs(ctx, req.KnowledgeIds)
	if err != nil {
		return nil, err
	}
	// 1.2 校验工具是否存在
	tids, err := s.getToolIDs(ctx, req.ToolIds)
	if err != nil {
		return nil, err
	}
	// 2. 创建角色
	role := &ent.AiChatRole{
		Name:           req.Name,
		Avatar:         req.Avatar,
		Description:    req.Description,
		Sort:           int(req.Sort),
		UserID:         int(u.Id),
		PublicStatus:   false,
		Category:       req.Category,
		SystemMessage:  req.SystemMessage,
		KnowledgeIds:   kids,
		ToolIds:        tids,
		McpClientNames: req.McpClientNames,
		Status:         entmodule.StatusActive,
	}
	created, err := s.rc.Upsert(ctx, role)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create role %w", err)
	}
	return buildRoleResponse(created, s.hasher), nil
}
func (s *RoleService) UpdateRole(ctx context.Context, req *pb.UpsertRoleRequest) (*pb.GetRoleResponse, error) {
	// 1.1 校验知识库是否存在
	kids, err := s.getKnowledgeIDs(ctx, req.KnowledgeIds)
	if err != nil {
		return nil, err
	}
	// 1.2 校验工具是否存在
	tids, err := s.getToolIDs(ctx, req.ToolIds)
	if err != nil {
		return nil, err
	}
	// 1.3 校验角色是否存在
	existed, err := s.validateRole(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	// 2. 更新角色
	role := &ent.AiChatRole{
		ID:             existed.ID,
		Name:           req.Name,
		Avatar:         req.Avatar,
		Description:    req.Description,
		Sort:           int(req.Sort),
		UserID:         0,
		PublicStatus:   false,
		Category:       req.Category,
		SystemMessage:  req.SystemMessage,
		KnowledgeIds:   kids,
		ToolIds:        tids,
		McpClientNames: req.McpClientNames,
		Status:         entmodule.StatusActive,
	}
	updated, err := s.rc.Upsert(ctx, role)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to update role %w", err)
	}
	return buildRoleResponse(updated, s.hasher), nil
}
func (s *RoleService) DeleteRole(ctx context.Context, req *pb.SimpleRoleRequest) (*emptypb.Empty, error) {
	// 1. 校验角色是否存在
	existed, err := s.validateRole(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	// 2. 删除角色
	if err := s.rc.Delete(ctx, existed.ID); err != nil {
		return nil, commonpb.ErrorDb("Failed to delete role %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *RoleService) GetRole(ctx context.Context, req *pb.SimpleRoleRequest) (*pb.GetRoleResponse, error) {
	// 1. 校验角色是否存在
	existed, err := s.validateRole(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return buildRoleResponse(existed, s.hasher), nil
}
func (s *RoleService) ListRole(ctx context.Context, req *emptypb.Empty) (*pb.ListRoleResponse, error) {
	u := trans.FromContext(ctx)
	roles, err := s.rc.List(ctx, &data.ListChatRoleArgs{UserID: int(u.Id)})
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list roles %w", err)
	}
	return &pb.ListRoleResponse{
		Roles: lo.Map(roles.Roles, func(r *ent.AiChatRole, index int) *pb.GetRoleResponse {
			return buildRoleResponse(r, s.hasher)
		}),
	}, nil
}
func (s *RoleService) PageRole(ctx context.Context, req *pb.PageRoleRequest) (*pb.PageRoleResponse, error) {
	u := trans.FromContext(ctx)
	// 构建分页参数
	args := &data.ListChatRoleArgs{
		PaginationArgs: req.Pagination,
		Name:           req.Name,
		UserID:         int(u.Id),
		PublicStatus:   req.IsPublic,
		Category:       req.Category,
		Status:         entmodule.StatusActive,
	}
	roles, err := s.rc.List(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list roles %w", err)
	}
	return &pb.PageRoleResponse{
		Roles: lo.Map(roles.Roles, func(r *ent.AiChatRole, index int) *pb.GetRoleResponse {
			return buildRoleResponse(r, s.hasher)
		}),
		Pagination: roles.PaginationResults,
	}, nil
}

func (s *RoleService) validateRole(ctx context.Context, id string) (*ent.AiChatRole, error) {
	if strings.TrimSpace(id) == "" {
		return nil, nil
	}
	roleID, err := s.hasher.Decode(id, hashid.RoleID)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("role id is invalid")
	}
	// Check if the role exists
	role, err := s.rc.GetActiveByID(ctx, roleID)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get role %s", err)
	}
	return role, nil
}

func (s *RoleService) validateKnowledges(ctx context.Context, rawIDs []string) ([]*ent.AiKnowledge, error) {
	var knowledgeList []*ent.AiKnowledge
	for _, rawID := range rawIDs {
		id, err := s.hasher.Decode(rawID, hashid.KnowledgeID)
		if err != nil {
			return nil, err
		}
		knowledge, err := s.kc.GetByID(ctx, id)
		if err != nil || knowledge == nil || knowledge.Status != entmodule.StatusActive {
			return nil, commonpb.ErrorParamInvalid("invalid knowledge id %w", err)
		}
		knowledgeList = append(knowledgeList, knowledge)
	}
	return knowledgeList, nil
}

func (s *RoleService) validateTools(ctx context.Context, ids []string) ([]*ent.AiTool, error) {
	var toolList []*ent.AiTool
	for _, id := range ids {
		id, err := s.hasher.Decode(id, hashid.ToolID)
		if err != nil {
			return nil, err
		}
		tool, err := s.tc.GetByID(ctx, id)
		if err != nil || tool == nil {
			return nil, err
		}
		toolList = append(toolList, tool)
	}
	return toolList, nil
}

func (s *RoleService) getKnowledgeIDs(ctx context.Context, rawIDs []string) ([]int, error) {
	if len(rawIDs) == 0 {
		return []int{}, nil
	}
	knowledges, err := s.validateKnowledges(ctx, rawIDs)
	if err != nil {
		return []int{}, commonpb.ErrorParamInvalid("invalid knowledge ids %w", err)
	}
	kidList := lo.Map(knowledges, func(k *ent.AiKnowledge, index int) int {
		return k.ID
	})
	return kidList, nil
}

func (s *RoleService) getToolIDs(ctx context.Context, rawIDs []string) ([]int, error) {
	if len(rawIDs) == 0 {
		return []int{}, nil
	}
	tools, err := s.validateTools(ctx, rawIDs)
	if err != nil {
		return []int{}, commonpb.ErrorParamInvalid("invalid tools ids %w", err)
	}
	toolList := lo.Map(tools, func(t *ent.AiTool, index int) int {
		return t.ID
	})
	return toolList, nil
}
