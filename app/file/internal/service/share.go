package service

import (
	commonpb "api/api/common/v1"
	pbshare "api/api/file/share/v1"
	userpb "api/api/user/common/v1"
	pbuser "api/api/user/users/v1"
	"api/external/trans"
	"common/boolset"
	"common/hashid"
	"context"
	"file/ent"
	"file/internal/biz/cluster/routes"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/manager"
	"file/internal/data"
	"file/internal/data/rpc"
	"file/internal/data/types"
	"fmt"
	"net/http"
	"time"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ShareService struct {
	pbshare.UnimplementedShareServer
	sc      data.ShareClient
	uc      pbuser.UserClient
	hasher  hashid.Encoder
	dep     filemanager.ManagerDep
	dbfsDep filemanager.DbfsDep
}

func NewShareService(sc data.ShareClient, uc pbuser.UserClient, hasher hashid.Encoder,
	dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep) *ShareService {
	return &ShareService{
		sc:      sc,
		uc:      uc,
		hasher:  hasher,
		dep:     dep,
		dbfsDep: dbfsDep,
	}
}

func (s *ShareService) CreateShare(ctx context.Context, req *pbshare.UpsertShareRequest) (*pbshare.UpsertShareResponse, error) {
	return s.UpsertShare(ctx, req, 0)
}
func (s *ShareService) EditShare(ctx context.Context, req *pbshare.UpsertShareRequest) (*pbshare.UpsertShareResponse, error) {
	shareId, err := s.hasher.Decode(req.Id, hashid.ShareID)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Invalid share id: %w", err)
	}
	return s.UpsertShare(ctx, req, shareId)
}
func (s *ShareService) GetShare(ctx context.Context, req *pbshare.GetShareRequest) (*pbshare.GetShareResponse, error) {
	user := trans.FromContext(ctx)

	newCtx := context.WithValue(ctx, data.LoadShareFile{}, true)
	shareId, err := s.hasher.Decode(req.Id, hashid.ShareID)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Invalid share id: %w", err)
	}
	share, err := s.sc.GetByID(newCtx, shareId)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("share not found")
		}
		return nil, commonpb.ErrorDb("Failed to get share: %w", err)
	}

	if err := data.IsValidShare(share); err != nil {
		return nil, commonpb.ErrorNotFound("Share link expired: %w", err)
	}

	if req.CountViews {
		_ = s.sc.Viewed(ctx, share)
	}

	unlocked := true
	// 分享需要密码
	if share.Password != "" && req.Password != share.Password && share.OwnerID != int(user.Id) {
		unlocked = false
	}

	base := s.dep.SettingProvider().SiteURL(ctx)
	res := buildShare(share, base, s.hasher, user.Id, int64(share.OwnerID), share.Edges.File.Name,
		share.Edges.File.Type, unlocked, false)

	if req.OwnerExtended && share.OwnerID == int(user.Id) {
		// 添加更多关于分享的信息
		m := manager.NewFileManager(s.dep, s.dbfsDep, user)
		defer m.Recycle()

		shareUri, err := fs.NewUriFromString(fs.NewShareUri(res.Id, req.Password))
		if err != nil {
			return nil, commonpb.ErrorInternalSetting("invalid share uri: %w", err)
		}
		root, err := m.Get(ctx, shareUri)
		if err != nil {
			return nil, commonpb.ErrorNotFound("files not found: %w", err)
		}

		res.SourceUri = root.Uri(true).String()
	}

	return res, nil

}
func (s *ShareService) ListShares(ctx context.Context, req *pbshare.ListSharesRequest) (*pbshare.ListSharesResponse, error) {
	user := trans.FromContext(ctx)
	hasher := s.hasher
	shareClient := s.sc

	args := &data.ListShareArgs{
		PaginationArgs: &data.PaginationArgs{
			UseCursorPagination: true,
			PageToken:           req.NextPageToken,
			PageSize:            int(req.PageSize),
			OrderBy:             req.OrderBy,
			Order:               data.OrderDirection(req.OrderDirection),
		},
		UserID: int(user.Id),
	}

	newCtx := context.WithValue(ctx, data.LoadShareFile{}, true)
	newCtx = context.WithValue(newCtx, data.LoadFileMetadata{}, true)
	res, err := shareClient.List(newCtx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list shares: %w", err)
	}

	base := s.dep.SettingProvider().SiteURL(ctx)
	return buildListShareResponse(res, hasher, base, user.Id, true), nil
}
func (s *ShareService) DeleteShare(ctx context.Context, req *pbshare.DeleteShareRequest) (*emptypb.Empty, error) {
	user := trans.FromContext(ctx)
	shareClient := s.sc

	newCtx := context.WithValue(ctx, data.LoadShareFile{}, true)
	var (
		share *ent.Share
		err   error
	)

	permissions := boolset.BooleanSet(user.Group.Permissions)
	if (&permissions).Enabled(types.GroupPermissionIsAdmin) {
		share, err = shareClient.GetByID(newCtx, int(req.Id))
	} else {
		share, err = shareClient.GetByIDUser(newCtx, int(req.Id), int(user.Id))
	}
	if err != nil {
		return nil, commonpb.ErrorNotFound("Share not found: %w", err)
	}

	if err := shareClient.Delete(ctx, share.ID); err != nil {
		return nil, commonpb.ErrorDb("Failed to delete share: %w", err)
	}

	return &emptypb.Empty{}, nil
}
func (s *ShareService) ListUserPublicShare(ctx context.Context, req *pbshare.ListSharesRequest) (*pbshare.ListSharesResponse, error) {
	uid, err := s.hasher.Decode(req.Id, hashid.UserID)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Invalid user id: %w", err)
	}
	user := trans.FromContext(ctx)
	hasher := s.hasher
	shareClient := s.sc

	targetUser, err := rpc.GetUserInfo(ctx, uid, s.uc)
	if err != nil {
		return nil, commonpb.ErrorRpcFailed("Failed to get users: %w", err)
	}

	if targetUser.Settings != nil && targetUser.Settings.ShareLinksInProfile == userpb.ShareLinksInProfileLevel_HIDE_SHARE {
		return nil, commonpb.ErrorParamInvalid("User has disabled share links in profile: %w", nil)
	}

	publicOnly := targetUser.Settings == nil || targetUser.Settings.ShareLinksInProfile == userpb.ShareLinksInProfileLevel_PUBLIC_SHARE_ONLY
	args := &data.ListShareArgs{
		PaginationArgs: &data.PaginationArgs{
			UseCursorPagination: true,
			PageToken:           req.NextPageToken,
			PageSize:            int(req.PageSize),
			OrderBy:             req.OrderBy,
			Order:               data.OrderDirection(req.OrderDirection),
		},
		UserID:     uid,
		PublicOnly: publicOnly,
	}

	newCtx := context.WithValue(ctx, data.LoadShareFile{}, true)
	newCtx = context.WithValue(newCtx, data.LoadFileMetadata{}, true)
	res, err := shareClient.List(newCtx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list share: %w", err)
	}

	base := s.dep.SettingProvider().SiteURL(ctx)
	return buildListShareResponse(res, hasher, base, user.Id, false), nil
}
func (s *ShareService) CountByTimeRange(ctx context.Context, req *pbshare.TimeRangeRequest) (*pbshare.CountByTimeRangeResponse, error) {
	var start, end time.Time
	if req.Start != nil {
		start = req.Start.AsTime()
	}
	if req.End != nil {
		end = req.End.AsTime()
	}
	count, err := s.sc.CountByTimeRange(ctx, &start, &end)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to count files: %w", err)
	}
	return &pbshare.CountByTimeRangeResponse{
		Count: int32(count),
	}, nil
}

func (s *ShareService) Redirect(ctx khttp.Context) error {
	var req *pbshare.RedirectShareRequest
	if err := ctx.BindVars(&req); err != nil {
		return commonpb.ErrorParamInvalid("Failed to bind request: %w", err)
	}

	url := routes.MasterShareLongUrl(req.Id, req.Password).String()
	http.Redirect(ctx.Response(), ctx.Request(), url, http.StatusFound)
	return nil
}

func (s *ShareService) UpsertShare(ctx context.Context, req *pbshare.UpsertShareRequest, existed int) (*pbshare.UpsertShareResponse, error) {
	user := trans.FromContext(ctx)
	m := manager.NewFileManager(s.dep, s.dbfsDep, user)
	defer m.Recycle()

	permissions := boolset.BooleanSet(user.Group.Permissions)
	if !(&permissions).Enabled(types.GroupPermissionShare) {
		return nil, commonpb.ErrorParamInvalid("Group permission is required")
	}

	uri, err := fs.NewUriFromString(req.Uri)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Unknown URI: %w", err)
	}

	var expires *time.Time
	if req.Expire > 0 {
		expires = new(time.Time)
		*expires = time.Now().Add(time.Duration(req.Expire) * time.Second)
	}

	share, err := m.CreateOrUpdateShare(ctx, uri, &manager.CreateShareArgs{
		ExistedShareID:  existed,
		IsPrivate:       req.IsPrivate,
		Password:        req.Password,
		RemainDownloads: int(req.RemainDownloads),
		Expire:          expires,
		ShareView:       req.ShareView,
		ShowReadMe:      req.ShowReadMe,
	})
	if err != nil {
		return nil, err
	}

	base := s.dep.SettingProvider().SiteURL(ctx)
	shareLink := buildShareLink(share, s.hasher, base, true)
	return &pbshare.UpsertShareResponse{Uri: shareLink}, nil
}
