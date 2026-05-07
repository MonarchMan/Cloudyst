package rpc

import (
	pbadmin "api/api/user/admin/v1"
	userpb "api/api/user/common/v1"
	pbuser "api/api/user/users/v1"
	"context"
	"encoding/json"
	"fmt"
	"queue"
	"strconv"
	"time"

	"github.com/go-kratos/kratos/contrib/registry/consul/v2"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/hashicorp/consul/api"
	"github.com/samber/lo"
)

type UserClient interface {
	// Client get raw user client
	Client() pbuser.UserClient
	// GetUserInfos get user infos by ids
	GetUserInfos(ctx context.Context, ids []int) (map[int]*userpb.User, error)
	// GetUserInfo get user info by id
	GetUserInfo(ctx context.Context, id int) (*userpb.User, error)
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

func (c *userClient) GetUserInfo(ctx context.Context, id int) (*userpb.User, error) {
	req := &pbuser.UserInfoRequest{
		Id: int32(id),
	}
	return c.client.GetUserInfo(ctx, req)
}

func (c *userClient) GetUserInfos(ctx context.Context, ids []int) (map[int]*userpb.User, error) {
	req := &pbuser.MultiUserRequest{
		Ids: lo.Map(ids, func(id int, _ int) int32 {
			return int32(id)
		}),
	}
	res, err := c.client.GetMultiUserInfos(ctx, req)
	if err != nil {
		return nil, err
	}
	return lo.Associate(res.Users, func(u *userpb.User) (int, *userpb.User) {
		return int(u.Id), u
	}), nil
}

type SettingClient interface {
	// GetSettings get user settings
	GetSettings(ctx context.Context, keys []string) (*pbadmin.AdminSettingResponse, error)
	// SetSettings update user settings
	SetSettings(ctx context.Context, settings map[string]any) (*pbadmin.AdminSettingResponse, error)
	// Queue get queue setting by queue type
	Queue(ctx context.Context, queueType string) (*queue.QueueSetting, error)
}

func NewSettingClient(client *api.Client) (SettingClient, error) {
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
	return &settingClient{
		client: pbadmin.NewAdminClient(conn),
	}, nil
}

type settingClient struct {
	client pbadmin.AdminClient
}

func (c *settingClient) GetSettings(ctx context.Context, keys []string) (*pbadmin.AdminSettingResponse, error) {
	req := &pbadmin.AdminGetSettingRequest{
		Keys: keys,
	}
	return c.client.GetSettings(ctx, req)
}

func (c *settingClient) SetSettings(ctx context.Context, settings map[string]any) (*pbadmin.AdminSettingResponse, error) {
	req := &pbadmin.AdminSetSettingRequest{
		Settings: lo.MapEntries(settings, func(key string, value any) (string, string) {
			bytes, _ := json.Marshal(value)
			return key, string(bytes)
		}),
	}
	return c.client.SetSettings(ctx, req)
}

func (c *settingClient) Queue(ctx context.Context, queueType string) (*queue.QueueSetting, error) {
	keys := []string{
		queue.QueuePrefix + queueType + queue.WorkerNumSuffix,
		queue.QueuePrefix + queueType + queue.MaxExecutionSuffix,
		queue.QueuePrefix + queueType + queue.BackoffFactorSuffix,
		queue.QueuePrefix + queueType + queue.BackoffMaxDurationSuffix,
		queue.QueuePrefix + queueType + queue.MaxRetrySuffix,
		queue.QueuePrefix + queueType + queue.RetryDelaySuffix,
	}
	res, err := c.GetSettings(ctx, keys)
	if err != nil {
		return nil, err
	}
	return &queue.QueueSetting{
		WorkerNum:          getInt(res.Settings[keys[0]], 0),
		MaxExecution:       time.Duration(getInt(res.Settings[keys[1]], 0)),
		BackoffFactor:      getFloat64(res.Settings[keys[2]], 0),
		BackoffMaxDuration: time.Duration(getInt(res.Settings[keys[3]], 0)),
		MaxRetry:           getInt(res.Settings[keys[4]], 0),
		RetryDelay:         time.Duration(getInt(res.Settings[keys[5]], 0)),
	}, nil
}

func getInt(num string, defaultValue int) int {
	if intVal, err := strconv.Atoi(num); err == nil {
		return intVal
	}
	return defaultValue
}

func getFloat64(num string, defaultValue float64) float64 {
	if floatVal, err := strconv.ParseFloat(num, 64); err == nil {
		return floatVal
	}
	return defaultValue
}
