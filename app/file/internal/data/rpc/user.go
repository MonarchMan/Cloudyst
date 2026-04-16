package rpc

import (
	pbadmin "api/api/user/admin/v1"
	pb "api/api/user/common/v1"
	pbuser "api/api/user/users/v1"
	"context"
	"file/internal/data/types"
	"fmt"

	"github.com/go-kratos/kratos/contrib/registry/consul/v2"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/hashicorp/consul/api"
	"github.com/samber/lo"
)

func NewUserClient(client *api.Client) (pbuser.UserClient, error) {
	// 1. 创建服务发现实例
	dis := consul.New(client)

	// 2. 通过服务名连接（不需要写死 IP）
	conn, err := grpc.DialInsecure(
		context.Background(),
		grpc.WithEndpoint("discovery:///cloudyst-user"), // 🔑 关键格式
		grpc.WithDiscovery(dis),                         // 🔑 启用服务发现
		grpc.WithMiddleware(
			recovery.Recovery(),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to dial user grpc: %w", err)
	}

	return pbuser.NewUserClient(conn), nil
}

func NewUserAdminClient(client *api.Client) (pbadmin.AdminClient, error) {
	// 1. 创建服务发现实例
	dis := consul.New(client)

	// 2. 通过服务名连接（不需要写死 IP）
	conn, err := grpc.DialInsecure(
		context.Background(),
		grpc.WithEndpoint("discovery:///cloudyst-user"), // 🔑 关键格式
		grpc.WithDiscovery(dis),                         // 🔑 启用服务发现
		grpc.WithMiddleware(
			recovery.Recovery(),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to dial user grpc: %w", err)
	}

	return pbadmin.NewAdminClient(conn), nil
}

func ApplyStorageDiff(ctx context.Context, diff types.StorageDiff, userClient pbuser.UserClient) error {
	req := &pbuser.ApplyStorageDiffRequest{
		StorageDiff: lo.MapKeys(diff, func(value int64, k int) int32 { return int32(k) }),
	}
	_, err := userClient.ApplyStorageDiff(ctx, req)

	return err
}

func GetUserInfo(ctx context.Context, uid int, userClient pbuser.UserClient) (*pb.User, error) {
	req := &pbuser.RawUserRequest{Id: int32(uid)}
	return userClient.GetUserInfo(ctx, req)
}
