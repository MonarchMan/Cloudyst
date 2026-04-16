package admin

import (
	commonpb "api/api/common/v1"
	pb "api/api/file/admin/v1"
	filepb "api/api/file/common/v1"
	"common/hashid"
	"context"
	"file/ent"
	"file/internal/biz/cluster/routes"
	"file/internal/data"
	"file/internal/pkg/utils"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	shareUserIDCondition = "share_user_id"
	shareFileIDCondition = "share_file_id"
	shareIDCondition     = "share_id"
)

func (s *AdminService) ListShares(ctx context.Context, req *filepb.ListRequest) (*pb.ListSharesResponse, error) {
	hasher := s.hasher
	shareClient := s.sc

	var (
		err      error
		userID   int
		fileID   int
		shareIDs []int
	)

	if req.Conditions[shareUserIDCondition] != "" {
		userID, err = strconv.Atoi(req.Conditions[shareUserIDCondition])
		if err != nil {
			return nil, commonpb.ErrorParamInvalid("Invalid share users ID: %w", err)
		}
	}

	if req.Conditions[shareFileIDCondition] != "" {
		fileID, err = strconv.Atoi(req.Conditions[shareFileIDCondition])
		if err != nil {
			return nil, commonpb.ErrorParamInvalid("Invalid share files ID: %w", err)
		}
	}

	if req.Conditions[shareIDCondition] != "" {
		shareIdStrs := strings.Split(req.Conditions[shareIDCondition], ",")
		for _, shareIdStr := range shareIdStrs {
			shareID, err := strconv.Atoi(shareIdStr)
			if err != nil {
				return nil, commonpb.ErrorParamInvalid("Invalid share ID: %w", err)
			}

			shareIDs = append(shareIDs, shareID)
		}
	}

	newCtx := context.WithValue(ctx, data.LoadShareFile{}, true)

	res, err := shareClient.List(newCtx, &data.ListShareArgs{
		PaginationArgs: &data.PaginationArgs{
			Page:     int(req.Page - 1),
			PageSize: int(req.PageSize),
			OrderBy:  req.OrderBy,
			Order:    data.OrderDirection(req.OrderDirection),
		},
		UserID:   userID,
		FileID:   fileID,
		ShareIDs: shareIDs,
	})

	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list shares: %w", err)
	}

	siteUrl := s.dep.SettingProvider().SiteURL(ctx)

	return &pb.ListSharesResponse{
		Pagination: res.PaginationResults,
		Shares: lo.Map(res.Shares, func(share *ent.Share, _ int) *pb.GetShareResponse {
			var (
				uid       string
				shareLink string
			)

			if share.OwnerInfo != nil {
				uid = hashid.EncodeUserID(hasher, share.OwnerID)
			}

			shareLink = routes.MasterShareUrl(siteUrl, hashid.EncodeShareID(hasher, share.ID), share.Password).String()

			return &pb.GetShareResponse{
				Share:      utils.EntShareToProto(share),
				UserHashId: uid,
				ShareLink:  shareLink,
			}
		}),
	}, nil
}
func (s *AdminService) GetShare(ctx context.Context, req *pb.SimpleShareRequest) (*pb.GetShareResponse, error) {
	hasher := s.hasher
	shareClient := s.sc

	newCtx := context.WithValue(ctx, data.LoadShareFile{}, true)
	share, err := shareClient.GetByID(newCtx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get share: %w", err)
	}

	var (
		uid       string
		shareLink string
	)

	uid = hashid.EncodeUserID(hasher, share.OwnerID)
	siteURL := s.dep.SettingProvider().SiteURL(ctx)
	shareLink = routes.MasterShareUrl(siteURL, hashid.EncodeShareID(hasher, share.ID), share.Password).String()

	return &pb.GetShareResponse{
		Share:      utils.EntShareToProto(share),
		UserHashId: uid,
		ShareLink:  shareLink,
	}, nil
}
func (s *AdminService) BatchDeleteShares(ctx context.Context, req *pb.BatchSharesRequest) (*emptypb.Empty, error) {
	shareClient := s.sc

	shareIds := lo.Map(req.Ids, func(id int32, _ int) int {
		return int(id)
	})
	if err := shareClient.DeleteBatch(ctx, shareIds); err != nil {
		return nil, commonpb.ErrorDb("Failed to delete shares: %w", err)
	}
	return &emptypb.Empty{}, nil
}
