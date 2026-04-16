package service

import (
	"ai/ent"
	"ai/internal/biz/types"
	"ai/internal/data"
	pb "api/api/ai/image/v1"
	commonpb "api/api/common/v1"
	"api/external/trans"
	"common/hashid"
	"context"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ImageService struct {
	pb.UnimplementedImageServer
	rc     data.RoleClient
	ic     data.ImageClient
	hasher hashid.Encoder
}

func NewImageService() *ImageService {
	return &ImageService{}
}

func (s *ImageService) DrawImage(ctx context.Context, req *pb.DrawImageRequest) (*pb.DrawImageResponse, error) {
	return &pb.DrawImageResponse{}, nil
}
func (s *ImageService) DeleteImage(ctx context.Context, req *pb.SimpleImageRequest) (*emptypb.Empty, error) {
	// 1. 校验图片是否存在
	image, err := s.validateImage(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	// 2. 删除图片
	if err := s.ic.Delete(ctx, image.ID); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
func (s *ImageService) GetImage(ctx context.Context, req *pb.SimpleImageRequest) (*pb.GetImageResponse, error) {
	// 1. 校验图片是否存在
	image, err := s.validateImage(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return buildImageResponse(image, s.hasher), nil
}
func (s *ImageService) ListImage(ctx context.Context, req *pb.ListImageRequest) (*pb.ListImageResponse, error) {
	// 1. 校验状态
	if err := validateStatus(req.Status); err != nil {
		return nil, err
	}
	u := trans.FromContext(ctx)
	// 构建分页参数
	args := &data.ListAIImageArgs{
		PaginationArgs: req.Pagination,
		Platform:       req.Platform,
		Status:         types.ImageStatus(req.Status),
		UserID:         int(u.Id),
	}
	// 2. 列表查询
	images, err := s.ic.List(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list images %w", err)
	}
	return &pb.ListImageResponse{
		Images: lo.Map(images.Images, func(i *ent.AiImage, index int) *pb.GetImageResponse {
			return buildImageResponse(i, s.hasher)
		}),
	}, nil
}

func validateStatus(status string) error {
	if status != "processing" && status != "success" && status != "failed" {
		return commonpb.ErrorParamInvalid("invalid status value")
	}
	return nil
}
func (s *ImageService) ListImageByIds(ctx context.Context, req *pb.ListImageByIdsRequest) (*pb.ListImageResponse, error) {
	var ids []int
	for _, rawID := range req.Ids {
		id, err := s.hasher.Decode(rawID, hashid.ImageID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	images, err := s.ic.GetByIDs(ctx, ids)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list images %w", err)
	}
	return &pb.ListImageResponse{
		Images: lo.Map(images, func(i *ent.AiImage, index int) *pb.GetImageResponse {
			return buildImageResponse(i, s.hasher)
		}),
	}, nil
}

func (s *ImageService) validateImage(ctx context.Context, rawID string) (*ent.AiImage, error) {
	id, err := s.hasher.Decode(rawID, hashid.ImageID)
	if err != nil {
		return nil, err
	}
	return s.ic.GetByID(ctx, id)
}
