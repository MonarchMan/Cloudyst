package service

import (
	"ai/ent"
	"ai/internal/biz/knowledge"
	"ai/internal/biz/types"
	"ai/internal/conf"
	"ai/internal/data"
	pb "api/api/ai/knowledge/v1"
	commonpb "api/api/common/v1"
	"api/external/data/common"
	"api/external/trans"
	"common/hashid"
	"context"
	"entmodule"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
)

type KnowledgeService struct {
	pb.UnimplementedKnowledgeServer
	hasher hashid.Encoder
	kb     knowledge.KnowledgeBiz
	conf   *conf.Bootstrap
	l      *log.Helper
}

func NewKnowledgeService(hasher hashid.Encoder, kb knowledge.KnowledgeBiz, conf *conf.Bootstrap, logger log.Logger) *KnowledgeService {
	return &KnowledgeService{
		hasher: hasher,
		kb:     kb,
		conf:   conf,
		l:      log.NewHelper(logger, log.WithMessageKey("service-knowledge")),
	}
}

func (s *KnowledgeService) CreateKnowledge(ctx context.Context, req *pb.UpsertKnowledgeRequest) (*pb.GetKnowledgeResponse, error) {
	u := trans.FromContext(ctx)
	args := &data.UpsertKnowledgeArgs{
		Name:        req.Name,
		Description: req.Description,
		UserID:      u.ID,
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
		PaginationArgs: common.PaginationArgsFromProto(req.Pagination),
		Name:           req.Name,
		IsPublic:       req.IsPublic,
		Status:         entmodule.GetStatus(req.Status),
	}
	res, err := s.kb.ListKnowledge(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list knowledge: %w", err)
	}
	return &pb.ListKnowledgeResponse{
		Pagination: common.PaginationResultsToProto(res.PaginationResults),
		Knowledges: lo.Map(res.Knowledges, func(k *ent.AiKnowledge, _ int) *pb.GetKnowledgeResponse {
			return buildGetKnowledgeResponse(s.hasher, k)
		}),
	}, nil
}
func (s *KnowledgeService) KnowledgeStats(ctx context.Context, req *pb.SimpleRequest) (*pb.KnowledgeStatsResponse, error) {
	id, err := validateID(s.hasher, req.Id, hashid.KnowledgeID, false)
	if err != nil {
		return nil, err
	}
	stats, err := s.kb.GetKnowledgeStats(ctx, id)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get knowledge stats: %w", err)
	}
	return buildKnowledgeStatsResponse(stats), nil
}
func (s *KnowledgeService) CreateDocument(ctx context.Context, req *pb.UpsertDocumentRequest) (*pb.UpsertDocumentResponse, error) {
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
	res, err := s.kb.CreateKnowledgeDocument(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create document: %w", err)
	}
	return buildCreateDocumentResponse(s.hasher, res), nil
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
			Version:          doc.Version,
		}
	}
	docs, task, err := s.kb.BatchCreateKnowledgeDocuments(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to batch create knowledge documents: %w", err)
	}
	return buildBatchCreateDocumentResponse(s.hasher, docs, task), nil
}
func (s *KnowledgeService) UpdateDocument(ctx context.Context, req *pb.UpsertDocumentRequest) (*pb.UpsertDocumentResponse, error) {
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
	return buildUpdateDocumentResponse(s.hasher, doc), nil
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
func (s *KnowledgeService) BatchDeleteDocuments(ctx context.Context, req *pb.MultiRequest) (*emptypb.Empty, error) {
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
		PaginationArgs: common.PaginationArgsFromProto(req.Pagination),
		KnowledgeID:    kid,
		Name:           req.Name,
		Status:         entmodule.GetStatus(req.Status),
	}
	res, err := s.kb.ListKnowledgeDocument(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list documents: %w", err)
	}
	return &pb.ListDocumentsResponse{
		Pagination: common.PaginationResultsToProto(res.PaginationResults),
		Documents: lo.Map(res.Documents, func(doc *ent.AiKnowledgeDocument, index int) *pb.GetDocumentResponse {
			return buildGetDocumentResponse(s.hasher, doc)
		}),
	}, nil
}
func (s *KnowledgeService) GetDocumentProgress(ctx context.Context, req *pb.SimpleRequest) (*pb.GetDocumentProgressResponse, error) {
	id, err := validateID(s.hasher, req.Id, hashid.KnowledgeDocumentID, false)
	if err != nil {
		return nil, err
	}
	doc, err := s.kb.GetKnowledgeDocument(ctx, id)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get document: %w", err)
	}
	return buildGetDocumentProgressResponse(s.hasher, doc), nil
}
func (s *KnowledgeService) ReindexDocument(ctx context.Context, req *pb.SimpleRequest) (*pb.ReindexDocumentResponse, error) {
	id, err := validateID(s.hasher, req.Id, hashid.KnowledgeDocumentID, false)
	if err != nil {
		return nil, err
	}
	res, err := s.kb.ReindexDocument(ctx, id)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to reindex document: %w", err)
	}
	return buildReindexDocumentResponse(s.hasher, res), nil
}
func (s *KnowledgeService) BatchReindexDocument(ctx context.Context, req *pb.MultiRequest) (*pb.BatchReindexDocumentResponse, error) {
	if len(req.Ids) >= 500 {
		return nil, commonpb.ErrorParamInvalid("batch reindex documents: documents too many")
	}
	ids := make([]int, len(req.Ids))
	var err error
	for i, id := range req.Ids {
		ids[i], err = validateID(s.hasher, id, hashid.KnowledgeDocumentID, false)
		if err != nil {
			return nil, err
		}
	}
	docs, task, err := s.kb.BatchReindexDocuments(ctx, ids)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to batch reindex documents: %w", err)
	}
	return buildBatchReindexDocumentResponse(s.hasher, docs, task), nil
}
func (s *KnowledgeService) Search(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
	found, err := s.kb.Retrieve(ctx, &types.SegmentSearchArgs{
		Content:    req.Query,
		TopK:       int(s.conf.Server.Retrieve.TopK),
		Similarity: s.conf.Server.Retrieve.Similarity,
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
		UserID:      u.ID,
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
		userID = trans.FromContext(ctx).ID
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
