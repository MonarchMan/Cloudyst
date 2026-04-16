package service

import (
	"ai/ent"
	"ai/internal/biz/knowledge"
	"ai/internal/biz/types"
	"ai/internal/conf"
	"ai/internal/data"
	pb "api/api/ai/knowledge/v1"
	commonpb "api/api/common/v1"
	"api/external/trans"
	"common/hashid"
	"context"
	"entmodule"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
)

type KnowledgeService struct {
	pb.UnimplementedKnowledgeServer
	hasher hashid.Encoder
	kb     knowledge.KnowledgeBiz
	conf   *conf.Bootstrap
}

func NewKnowledgeService() *KnowledgeService {
	return &KnowledgeService{}
}

func (s *KnowledgeService) CreateKnowledge(ctx context.Context, req *pb.UpsertKnowledgeRequest) (*pb.GetKnowledgeResponse, error) {
	u := trans.FromContext(ctx)
	args := &data.UpsertKnowledgeArgs{
		Name:        req.Name,
		Description: req.Description,
		UserID:      int(u.Id),
		IsPublic:    req.IsPublic,
	}
	k, err := s.kb.CreateKnowledge(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create knowledge: %w", err)
	}
	return buildGetKnowledgeResponse(s.hasher, k), nil
}
func (s *KnowledgeService) UpdateKnowledge(ctx context.Context, req *pb.UpsertKnowledgeRequest) (*pb.GetKnowledgeResponse, error) {
	id, err := validateID(s.hasher, req.Id, hashid.KnowledgeID, false)
	if err != nil {
		return nil, err
	}
	args := &data.UpsertKnowledgeArgs{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
	}
	k, err := s.kb.UpdateKnowledge(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to update knowledge: %w", err)
	}
	return buildGetKnowledgeResponse(s.hasher, k), nil
}
func (s *KnowledgeService) GetKnowledge(ctx context.Context, req *pb.SimpleRequest) (*pb.GetKnowledgeResponse, error) {
	id, err := validateID(s.hasher, req.Id, hashid.KnowledgeID, false)
	if err != nil {
		return nil, err
	}
	k, err := s.kb.GetKnowledge(ctx, id)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get knowledge: %w", err)
	}
	if k == nil {
		return nil, commonpb.ErrorNotFound("Knowledge not found")
	}
	return buildGetKnowledgeResponse(s.hasher, k), nil
}
func (s *KnowledgeService) DeleteKnowledge(ctx context.Context, req *pb.SimpleRequest) (*emptypb.Empty, error) {
	id, err := validateID(s.hasher, req.Id, hashid.KnowledgeID, false)
	if err != nil {
		return nil, err
	}
	err = s.kb.DeleteKnowledge(ctx, id)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to delete knowledge: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *KnowledgeService) ListKnowledge(ctx context.Context, req *pb.ListKnowledgeRequest) (*pb.ListKnowledgeResponse, error) {
	args := &data.ListKnowledgeArgs{
		PaginationArgs: &commonpb.PaginationArgs{
			Page:           req.Pagination.Page - 1,
			PageSize:       req.Pagination.PageSize,
			OrderBy:        req.Pagination.OrderBy,
			OrderDirection: req.Pagination.OrderDirection,
		},
		Name:   req.Name,
		Status: entmodule.StatusFromProto(req.Status),
	}
	res, err := s.kb.ListKnowledge(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list knowledge: %w", err)
	}
	return &pb.ListKnowledgeResponse{
		Pagination: res.PaginationResults,
		Knowledge: lo.Map(res.Knowledges, func(k *ent.AiKnowledge, _ int) *pb.GetKnowledgeResponse {
			return buildGetKnowledgeResponse(s.hasher, k)
		}),
	}, nil
}
func (s *KnowledgeService) CreateDocument(ctx context.Context, req *pb.UpsertDocumentRequest) (*pb.GetDocumentResponse, error) {
	kid, err := validateID(s.hasher, req.KnowledgeId, hashid.KnowledgeID, false)
	if err != nil {
		return nil, err
	}
	args := &data.UpsertDocumentArgs{
		KnowledgeID:      kid,
		Name:             req.Name,
		Url:              req.Url,
		SegmentMaxTokens: int(req.SegmentMaxTokens),
		Version:          req.Version,
	}
	doc, err := s.kb.CreateKnowledgeDocument(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create document: %w", err)
	}
	return buildGetDocumentResponse(s.hasher, doc), nil
}
func (s *KnowledgeService) BatchCreateDocuments(ctx context.Context, req *pb.BatchCreateDocumentRequest) (*pb.BatchCreateDocumentResponse, error) {
	args := make([]*data.UpsertDocumentArgs, len(req.Documents))
	for i, doc := range req.Documents {
		id, err := validateID(s.hasher, doc.KnowledgeId, hashid.KnowledgeID, false)
		if err != nil {
			return nil, err
		}
		args[i] = &data.UpsertDocumentArgs{
			KnowledgeID:      id,
			Name:             doc.Name,
			Url:              doc.Url,
			SegmentMaxTokens: int(doc.SegmentMaxTokens),
		}
	}
	docs, err := s.kb.BatchCreateKnowledgeDocuments(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to batch create knowledge documents: %w", err)
	}
	return &pb.BatchCreateDocumentResponse{
		Total: int64(len(docs)),
		Documents: lo.Map(docs, func(doc *ent.AiKnowledgeDocument, index int) *pb.GetDocumentResponse {
			return buildGetDocumentResponse(s.hasher, doc)
		}),
	}, nil
}
func (s *KnowledgeService) UpdateDocument(ctx context.Context, req *pb.UpsertDocumentRequest) (*pb.GetDocumentResponse, error) {
	id, err := validateID(s.hasher, req.Id, hashid.KnowledgeDocumentID, false)
	if err != nil {
		return nil, err
	}
	kid, err := validateID(s.hasher, req.KnowledgeId, hashid.KnowledgeID, true)
	if err != nil {
		return nil, err
	}
	// TODO: 设计知识库的用户权限
	args := &data.UpsertDocumentArgs{
		ID:               id,
		KnowledgeID:      kid,
		Name:             req.Name,
		Url:              req.Url,
		SegmentMaxTokens: int(req.SegmentMaxTokens),
	}
	doc, err := s.kb.UpdateKnowledgeDocument(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to update document: %w", err)
	}
	return buildGetDocumentResponse(s.hasher, doc), nil
}
func (s *KnowledgeService) GetDocument(ctx context.Context, req *pb.SimpleRequest) (*pb.GetDocumentResponse, error) {
	id, err := validateID(s.hasher, req.Id, hashid.KnowledgeDocumentID, false)
	if err != nil {
		return nil, err
	}
	doc, err := s.kb.GetKnowledgeDocument(ctx, id)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get document: %w", err)
	}
	return buildGetDocumentResponse(s.hasher, doc), nil
}
func (s *KnowledgeService) DeleteDocument(ctx context.Context, req *pb.SimpleRequest) (*emptypb.Empty, error) {
	id, err := validateID(s.hasher, req.Id, hashid.KnowledgeDocumentID, false)
	if err != nil {
		return nil, err
	}
	err = s.kb.DeleteKnowledgeDocument(ctx, id)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to delete document: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *KnowledgeService) BatchDeleteDocuments(ctx context.Context, req *pb.BatchDeleteRequest) (*emptypb.Empty, error) {
	ids := make([]int, len(req.Ids))
	var err error
	for i, id := range req.Ids {
		ids[i], err = validateID(s.hasher, id, hashid.KnowledgeDocumentID, false)
		if err != nil {
			return nil, err
		}
	}
	err = s.kb.BatchDeleteKnowledgeDocuments(ctx, ids)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to batch delete knowledge documents: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *KnowledgeService) ListDocuments(ctx context.Context, req *pb.ListDocumentsRequest) (*pb.ListDocumentsResponse, error) {
	kid, err := validateID(s.hasher, req.KnowledgeId, hashid.KnowledgeID, true)
	if err != nil {
		return nil, err
	}
	req.Pagination.Page -= 1
	args := &data.ListKnowledgeDocumentArgs{
		PaginationArgs: req.Pagination,
		KnowledgeID:    kid,
		Name:           req.Name,
		Status:         entmodule.StatusFromProto(req.Status),
	}
	res, err := s.kb.ListKnowledgeDocument(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list documents: %w", err)
	}
	return &pb.ListDocumentsResponse{
		Pagination: res.PaginationResults,
		Documents: lo.Map(res.Documents, func(doc *ent.AiKnowledgeDocument, index int) *pb.GetDocumentResponse {
			return buildGetDocumentResponse(s.hasher, doc)
		}),
	}, nil
}
func (s *KnowledgeService) Search(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
	found, err := s.kb.Retrieve(ctx, &types.SegmentSearchArgs{
		Content:    req.Query,
		TopK:       int(s.conf.Server.Sys.Retrieve.TopK),
		Similarity: s.conf.Server.Sys.Retrieve.Similarity,
	})
	if err != nil {
		return nil, commonpb.ErrorInternalSetting("search knowledge: %w", err)
	}
	docIDs := lo.Map(found, func(seg *types.KnowledgeSegment, index int) int {
		return seg.DocumentID
	})
	docs, err := s.kb.GetKnowledgeDocuments(ctx, docIDs)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get knowledge documents: %w", err)
	}
	docMap := make(map[int]*ent.AiKnowledgeDocument, len(docs))
	for _, doc := range docs {
		docMap[doc.ID] = doc
	}
	return buildSearchResponse(s.hasher, found, docMap), nil
}

func (s *KnowledgeService) CopyDocument(ctx context.Context, req *pb.CopyDocumentRequest) (*pb.GetDocumentResponse, error) {
	id, err := validateID(s.hasher, req.DocId, hashid.KnowledgeDocumentID, false)
	if err != nil {
		return nil, err
	}
	doc, err := s.kb.CopyKnowledgeDocument(ctx, id, req.Version)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to copy document: %w", err)
	}

	return buildGetDocumentResponse(s.hasher, doc), nil
}

func (s *KnowledgeService) CreateMasterKnowledge(ctx context.Context, req *pb.CreateMasterKnowledgeRequest) (*pb.GetKnowledgeResponse, error) {
	u := trans.FromContext(ctx)
	args := &data.UpsertKnowledgeArgs{
		Name:        req.Name,
		Description: req.Description,
		UserID:      int(u.Id),
		IsPublic:    req.IsPublic,
	}
	k, err := s.kb.CreateKnowledge(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create user master knowledge: %w", err)
	}
	return buildGetKnowledgeResponse(s.hasher, k), nil
}
func (s *KnowledgeService) GetMasterKnowledge(ctx context.Context, req *pb.GetMasterKnowledgeRequest) (*pb.GetKnowledgeResponse, error) {
	var userID int
	if req.UserId > 0 {
		userID = int(req.UserId)
	} else {
		userID = int(trans.FromContext(ctx).Id)
	}
	k, err := s.kb.GetUserMasterKnowledge(ctx, userID)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get user master knowledge: %w", err)
	}
	return buildGetKnowledgeResponse(s.hasher, k), nil
}
func (s *KnowledgeService) ChangeDocumentOwner(ctx context.Context, req *pb.ChangeDocumentOwnerRequest) (*pb.GetDocumentResponse, error) {
	id, err := validateID(s.hasher, req.DocId, hashid.KnowledgeDocumentID, false)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("document id invalid: %w", err)
	}
	oKID, err := validateID(s.hasher, req.OldKnowledgeId, hashid.KnowledgeID, true)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("old knowledge id invalid: %w", err)
	}
	nKID, err := validateID(s.hasher, req.NewKnowledgeId, hashid.KnowledgeID, true)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("new knowledge id invalid: %w", err)
	}

	k, err := s.kb.ChangeKnowledgeDocumentOwner(ctx, id, oKID, nKID)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to change knowledge document owner: %w", err)
	}
	return buildGetDocumentResponse(s.hasher, k), nil
}
func (s *KnowledgeService) GetSupportTextParse(ctx context.Context, req *emptypb.Empty) (*pb.GetSupportTextParseResponse, error) {
	support := s.kb.GetSupportTextParseTypes(ctx)
	tsStr := lo.Map(support.Types, func(t types.TextParseType, index int) string {
		return string(t)
	})
	return &pb.GetSupportTextParseResponse{
		Types:       tsStr,
		MaxFileSize: support.MaxFileSize,
	}, nil
}
