package service

import (
	"ai/internal/biz/model"
	"ai/internal/data"
	commonpb "api/api/common/v1"
	"api/external/data/common"
	"common/hashid"
	"context"
	"entmodule"

	pb "api/api/ai/model/v1"

	"github.com/go-kratos/kratos/v2/log"
)

type ModelService struct {
	pb.UnimplementedModelServer
	mb     model.ModelBiz
	hasher hashid.Encoder
	l      *log.Helper
}

func NewModelService(hasher hashid.Encoder, mb model.ModelBiz, logger log.Logger) *ModelService {
	return &ModelService{
		mb:     mb,
		hasher: hasher,
		l:      log.NewHelper(logger, log.WithMessageKey("service-model")),
	}
}

func (s *ModelService) ListModel(ctx context.Context, req *pb.ListModelRequest) (*pb.ListModelResponse, error) {
	args := &data.ListAiModelArgs{
		PaginationArgs: common.PaginationArgsFromProto(req.Pagination),
		Name:           req.Name,
		Platform:       req.Platform,
		Status:         entmodule.StatusActive,
	}
	result, err := s.mb.ListModels(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list models: %w", err)
	}
	return &pb.ListModelResponse{
		Pagination: common.PaginationResultsToProto(result.PaginationResults),
	}, nil
}
func (s *ModelService) GetModel(ctx context.Context, req *pb.SimpleRequest) (*pb.GetModelResponse, error) {
	id, err := validateID(s.hasher, req.Id, hashid.ModelID, false)
	if err != nil {
		return nil, err
	}
	m, err := s.mb.GetModel(ctx, id)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get model: %w", err)
	}
	return buildGetModelResponse(s.hasher, m), nil
}
func (s *ModelService) GetDefaultModel(ctx context.Context, req *pb.DefaultModelRequest) (*pb.GetModelResponse, error) {
	defaultModel, err := s.mb.GetDefaultModel(ctx, req.Type)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get default model for type %s: %w", req.Type, err)
	}
	return buildGetModelResponse(s.hasher, defaultModel), nil
}
