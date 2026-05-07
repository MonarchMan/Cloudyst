package rpc

import (
	userpb "api/api/user/common/v1"
	pbuser "api/api/user/users/v1"
	"api/external/data/userdata"
	"context"
	"file/internal/data/types"
	"fmt"

	"github.com/go-kratos/kratos/contrib/registry/consul/v2"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/hashicorp/consul/api"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
)

type UserClient interface {
	Client() pbuser.UserClient
	GetUserInfo(ctx context.Context, uid int) (*userdata.User, error)
	ApplyStorageDiff(ctx context.Context, diff types.StorageDiff) error
	GetAnonymousUser(ctx context.Context) (*userdata.User, error)
	UpdateAvatar(ctx context.Context, avatarType int) error
	GetUserByDavAccount(ctx context.Context, username, password string) (*userpb.User, error)
	GetActiveUserByDavAccount(ctx context.Context, username, password string) (*userpb.User, error)
	GetUserByEmail(ctx context.Context, email string) (*userpb.User, error)
}

type userClient struct {
	client pbuser.UserClient
}

func NewUserClient(client *api.Client) (UserClient, error) {
	dis := consul.New(client)

	conn, err := grpc.DialInsecure(
		context.Background(),
		grpc.WithEndpoint("discovery:///cloudyst-user"), // 🔑 关键格式
		grpc.WithDiscovery(dis),                         // 🔑 启用服务发现
		grpc.WithMiddleware(
			recovery.Recovery(),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to dial file grpc: %w", err)
	}
	return &userClient{
		client: pbuser.NewUserClient(conn),
	}, nil
}

func (c *userClient) Client() pbuser.UserClient {
	return c.client
}

func (c *userClient) GetUserInfo(ctx context.Context, uid int) (*userdata.User, error) {
	req := &pbuser.UserInfoRequest{Id: int32(uid)}
	user, err := c.client.GetUserInfo(ctx, req)
	if err != nil {
		return nil, err
	}
	return userdata.UserFromProto(user), nil
}

func (c *userClient) ApplyStorageDiff(ctx context.Context, diff types.StorageDiff) error {
	req := &pbuser.ApplyStorageDiffRequest{
		StorageDiff: lo.MapKeys(diff, func(value int64, k int) int32 { return int32(k) }),
	}
	_, err := c.client.ApplyStorageDiff(ctx, req)
	return err
}

func (c *userClient) GetAnonymousUser(ctx context.Context) (*userdata.User, error) {
	user, err := c.client.GetAnonymousUser(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	return userdata.AnonymousUserFromProto(user), nil
}

func (c *userClient) UpdateAvatar(ctx context.Context, avatarType int) error {
	req := &pbuser.UpdateAvatarRequest{
		Type: pbuser.AvatarType_FILE_AVATAR,
	}
	_, err := c.client.UpdateAvatar(ctx, req)
	return err
}

func (c *userClient) GetUserByDavAccount(ctx context.Context, username, password string) (*userpb.User, error) {
	req := &pbuser.GetActiveUserByDavAccountRequest{
		Username: username,
		Password: password,
	}
	return c.client.GetActiveUserByDavAccount(ctx, req)
}

func (c *userClient) GetActiveUserByDavAccount(ctx context.Context, username, password string) (*userpb.User, error) {
	req := &pbuser.GetActiveUserByDavAccountRequest{
		Username: username,
		Password: password,
	}
	return c.client.GetActiveUserByDavAccount(ctx, req)
}

func (c *userClient) GetUserByEmail(ctx context.Context, email string) (*userpb.User, error) {
	req := &pbuser.GetUserByEmailRequest{
		Email: email,
	}
	return c.client.GetUserByEmail(ctx, req)
}
