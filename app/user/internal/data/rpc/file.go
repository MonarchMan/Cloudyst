package rpc

import (
	pbadmin "api/api/file/admin/v1"
	pbfile "api/api/file/files/v1"
	pbshare "api/api/file/share/v1"
	pbsys "api/api/file/sys/v1"
	"context"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/contrib/registry/consul/v2"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	kgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/hashicorp/consul/api"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type FileClient interface {
	DeleteTasksByUser(ctx context.Context, ids ...int32) error
	CountByTimeRange(ctx context.Context, startTime, endTime time.Time) (int32, error)
}

type fileClient struct {
	client pbfile.FileClient
}

func NewFileClient(client *api.Client) (FileClient, error) {
	conn, err := getFileGrpcConn(client)
	if err != nil {
		return nil, fmt.Errorf("failed to dial file grpc: %w", err)
	}
	fc := pbfile.NewFileClient(conn)
	return &fileClient{client: fc}, nil
}

func (c *fileClient) DeleteTasksByUser(ctx context.Context, ids ...int32) error {
	if len(ids) == 0 {
		return nil
	}

	req := &pbfile.SimpleUserRequest{
		Ids: ids,
	}
	_, err := c.client.DeleteFilesByUserId(ctx, req)
	return err
}

func (c *fileClient) CountByTimeRange(ctx context.Context, startTime, endTime time.Time) (int32, error) {
	req := &pbfile.TimeRangeRequest{}
	if !startTime.IsZero() {
		req.Start = timestamppb.New(startTime)
	}
	if !endTime.IsZero() {
		req.End = timestamppb.New(endTime)
	}

	resp, err := c.client.CountByTimeRange(ctx, req)
	if err != nil {
		return 0, err
	}
	return resp.Count, nil
}

type ShareClient interface {
	CountByTimeRange(ctx context.Context, startTime, endTime time.Time) (int32, error)
}

type shareClient struct {
	client pbshare.ShareClient
}

func NewShareClient(client *api.Client) (ShareClient, error) {
	conn, err := getFileGrpcConn(client)
	if err != nil {
		return nil, fmt.Errorf("failed to dial share grpc: %w", err)
	}
	fc := pbshare.NewShareClient(conn)
	return &shareClient{client: fc}, nil
}

func (c *shareClient) CountByTimeRange(ctx context.Context, startTime, endTime time.Time) (int32, error) {
	req := &pbshare.TimeRangeRequest{}
	if !startTime.IsZero() {
		req.Start = timestamppb.New(startTime)
	}
	if !endTime.IsZero() {
		req.End = timestamppb.New(endTime)
	}
	resp, err := c.client.CountByTimeRange(ctx, req)
	if err != nil {
		return 0, err
	}
	return resp.Count, nil
}

type FileSysClient interface {
	ReloadDependency(ctx context.Context, keys []string) error
}
type fileSysClient struct {
	client pbsys.SysClient
}

func NewFileSysClient(client *api.Client) (FileSysClient, error) {
	conn, err := getFileGrpcConn(client)
	if err != nil {
		return nil, fmt.Errorf("failed to dial file grpc: %w", err)
	}
	sc := pbsys.NewSysClient(conn)
	return &fileSysClient{client: sc}, nil
}

func (c *fileSysClient) ReloadDependency(ctx context.Context, keys []string) error {
	req := &pbsys.ReloadDependencyRequest{
		Keys: keys,
	}
	_, err := c.client.ReloadDependency(ctx, req)
	return err
}

type FileAdminClient interface {
	DeleteTasksByUser(ctx context.Context, ids ...int32) error
}

type fileAdminClient struct {
	client pbadmin.AdminClient
}

func NewFileAdminClient(client *api.Client) (FileAdminClient, error) {
	conn, err := getFileGrpcConn(client)
	if err != nil {
		return nil, fmt.Errorf("failed to dial file admin grpc: %w", err)
	}
	ac := pbadmin.NewAdminClient(conn)
	return &fileAdminClient{client: ac}, nil
}

func (c *fileAdminClient) DeleteTasksByUser(ctx context.Context, ids ...int32) error {
	if len(ids) == 0 {
		return nil
	}
	req := &pbadmin.DeleteTaskByUserIdsRequest{
		Ids: ids,
	}
	_, err := c.client.DeleteTaskByUserIds(ctx, req)
	return err
}

func getFileGrpcConn(client *api.Client) (*grpc.ClientConn, error) {
	dis := consul.New(client)

	return kgrpc.DialInsecure(
		context.Background(),
		kgrpc.WithEndpoint("discovery:///cloudyst-file"), // 🔑 关键格式
		kgrpc.WithDiscovery(dis),                         // 🔑 启用服务发现
		kgrpc.WithMiddleware(
			recovery.Recovery(),
		),
	)
}
