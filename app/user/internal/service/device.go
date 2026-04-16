package service

import (
	commonpb "api/api/common/v1"
	userpb "api/api/user/common/v1"
	pb "api/api/user/device/v1"
	"api/external/trans"
	"common/boolset"
	"common/constants"
	"common/hashid"
	"common/util"
	"context"
	"net/url"
	"user/internal/data"
	"user/internal/data/types"

	"google.golang.org/protobuf/types/known/emptypb"
)

type DeviceService struct {
	pb.UnimplementedDeviceServer
	davc   data.DavAccountClient
	hasher hashid.Encoder
}

func NewDeviceService(davc data.DavAccountClient, hasher hashid.Encoder) *DeviceService {
	return &DeviceService{
		davc:   davc,
		hasher: hasher,
	}
}

func (s *DeviceService) ListDavAccounts(ctx context.Context, req *pb.ListDavAccountsRequest) (*pb.ListDavAccountsResponse, error) {
	user := trans.FromContext(ctx)
	hasher := s.hasher
	davAccountClient := s.davc

	args := &data.ListDavAccountArgs{
		PaginationArgs: &data.PaginationArgs{
			UseCursorPagination: true,
			PageToken:           req.NextPageToken,
			PageSize:            int(req.PageSize),
		},
		UserID: int(user.Id),
	}

	res, err := davAccountClient.List(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list dav accounts: %w", err)
	}

	return buildListDavAccountsResponse(res, hasher), nil
}
func (s *DeviceService) CreateDavAccount(ctx context.Context, req *pb.UpsertDavAccountRequest) (*pb.GetDavAccountResponse, error) {
	user := trans.FromContext(ctx)
	bs, err := s.validateAndGetBs(user, req)
	if err != nil {
		return nil, err
	}

	davAccountClient := s.davc
	account, err := davAccountClient.Create(ctx, &data.CreateDavAccountParams{
		UserID:   int(user.Id),
		Name:     req.Name,
		URI:      req.Uri,
		Password: util.RandString(32, util.RandomLowerCases),
		Options:  bs,
	})
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create dav account: %w", err)
	}

	return buildDavAccountResponse(account, s.hasher), nil
}
func (s *DeviceService) UpdateDavAccount(ctx context.Context, req *pb.UpsertDavAccountRequest) (*pb.GetDavAccountResponse, error) {
	user := trans.FromContext(ctx)
	accountId, err := s.hasher.Decode(req.Id, hashid.DavAccountID)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to decode dav account id: %w", err)
	}
	// 查询帐户是否已存在
	davAccountClient := s.davc
	account, err := davAccountClient.GetByIDAndUserID(ctx, accountId, int(user.Id))
	if err != nil {
		return nil, commonpb.ErrorNotFound("Account not exist: %w", err)
	}

	bs, err := s.validateAndGetBs(user, req)
	if err != nil {
		return nil, err
	}

	// 更新账户
	account, err = davAccountClient.Update(ctx, accountId, &data.CreateDavAccountParams{
		Name:    req.Name,
		URI:     req.Uri,
		Options: bs,
	})
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to update dav account: %w", err)
	}

	return buildDavAccountResponse(account, s.hasher), nil
}
func (s *DeviceService) DeleteDavAccount(ctx context.Context, req *pb.SimpleDavAccountRequest) (*emptypb.Empty, error) {
	user := trans.FromContext(ctx)
	accountId, err := s.hasher.Decode(req.Id, hashid.DavAccountID)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to decode dav account id: %w", err)
	}

	// 检查账户是否已存在
	davAccountClient := s.davc
	_, err = davAccountClient.GetByIDAndUserID(ctx, accountId, int(user.Id))
	if err != nil {
		return nil, commonpb.ErrorNotFound("Account not exist: %w", err)
	}

	// 删除账户

	if err := davAccountClient.Delete(ctx, accountId); err != nil {
		return nil, commonpb.ErrorDb("Failed to delete dav account: %w", err)
	}

	return &emptypb.Empty{}, nil
}

func (s *DeviceService) validateAndGetBs(user *userpb.User, req *pb.UpsertDavAccountRequest) (*boolset.BooleanSet, error) {
	permissions := boolset.BooleanSet(user.Group.Permissions)
	if !(&permissions).Enabled(types.GroupPermissionWebDAV) {
		return nil, commonpb.ErrorParamInvalid("User does not have permission to create dav account")
	}

	uri, err := url.Parse(req.Uri)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Invalid URI")
	}

	// 只有 my 和 share 的 fs 在 WebDav 中允许
	if uriFs := constants.FileSystemType(uri.Host); uri.Query() != nil ||
		(uriFs != constants.FileSystemMy && uriFs != constants.FileSystemShare) {
		return nil, commonpb.ErrorParamInvalid("Invalid URI")
	}

	bs := boolset.BooleanSet{}
	if req.ReadOnly {
		boolset.Set(constants.DavAccountReadOnly, true, &bs)
	}

	if req.DisableSysFiles {
		boolset.Set(constants.DavAccountDisableSysFiles, true, &bs)
	}

	if req.Proxy && (&permissions).Enabled(int(types.GroupPermissionWebDAVProxy)) {
		boolset.Set(constants.DavAccountProxy, true, &bs)
	}
	return &bs, nil
}
