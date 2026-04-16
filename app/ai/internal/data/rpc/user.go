package rpc

import (
	pbadmin "api/api/user/admin/v1"
	pbuser "api/api/user/users/v1"
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-kratos/kratos/contrib/registry/consul/v2"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/hashicorp/consul/api"
	"github.com/samber/lo"
)

func RawUserClient(client *api.Client) (pbuser.UserClient, error) {
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
	return pbuser.NewUserClient(conn), nil
}

type UserClient interface {
	GetSettings(ctx context.Context, keys []string) (*pbadmin.AdminSettingResponse, error)
	SetSettings(ctx context.Context, settings map[string]any) (*pbadmin.AdminSettingResponse, error)
}

type userClient struct {
	ac pbadmin.AdminClient
	uc pbuser.UserClient
}

func NewUserClient(client *api.Client, rawUC pbuser.UserClient) (UserClient, error) {
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
		ac: pbadmin.NewAdminClient(conn),
		uc: rawUC,
	}, nil
}

func (c *userClient) GetSettings(ctx context.Context, keys []string) (*pbadmin.AdminSettingResponse, error) {
	req := &pbadmin.AdminGetSettingRequest{
		Keys: keys,
	}
	return c.ac.GetSettings(ctx, req)
}

func (c *userClient) SetSettings(ctx context.Context, settings map[string]any) (*pbadmin.AdminSettingResponse, error) {
	req := &pbadmin.AdminSetSettingRequest{
		Settings: lo.MapEntries(settings, func(key string, value any) (string, string) {
			bytes, _ := json.Marshal(value)
			return key, string(bytes)
		}),
	}
	return c.ac.SetSettings(ctx, req)
}
