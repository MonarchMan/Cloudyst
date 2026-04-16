package rpc

import (
	pbfile "api/api/file/files/v1"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/go-kratos/kratos/contrib/registry/consul/v2"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/hashicorp/consul/api"
)

const defaultPageSize = 1000

type FileClient interface {
	GetFileInfo(ctx context.Context, uri string) (*pbfile.FileResponse, error)
	GetFileUrl(ctx context.Context, uri []string) (*pbfile.FileUrlResponse, error)
	ListDirectory(ctx context.Context, uri string, page int) (*pbfile.ListFileResponse, error)
	CreateFile(ctx context.Context, uri string) (*pbfile.FileResponse, error)
	PutContent(ctx context.Context, uri string, prev string, reader io.Reader, fileSize int64) (*pbfile.FileResponse, error)
	PutContentString(ctx context.Context, uri string, prev string, content string) (*pbfile.FileResponse, error)
}

type fileClient struct {
	fc pbfile.FileClient
}

func NewFileClient(client *api.Client) (FileClient, error) {
	dis := consul.New(client)

	conn, err := grpc.DialInsecure(
		context.Background(),
		grpc.WithEndpoint("discovery:///cloudyst-file"), // 🔑 关键格式
		grpc.WithDiscovery(dis),                         // 🔑 启用服务发现
		grpc.WithMiddleware(
			recovery.Recovery(),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to dial file grpc: %w", err)
	}

	return &fileClient{fc: pbfile.NewFileClient(conn)}, nil
}

func (c *fileClient) GetFileInfo(ctx context.Context, uri string) (*pbfile.FileResponse, error) {
	req := &pbfile.GetFileInfoRequest{
		Uri:           uri,
		ExtendedInfo:  false,
		FolderSummary: false,
	}
	return c.fc.GetFileInfo(ctx, req)
}

func (c *fileClient) GetFileUrl(ctx context.Context, uri []string) (*pbfile.FileUrlResponse, error) {
	req := &pbfile.FileUrlRequest{
		Uris:   uri,
		Entity: "",
	}
	return c.fc.FileUrl(ctx, req)
}

func (c *fileClient) ListDirectory(ctx context.Context, uri string, page int) (*pbfile.ListFileResponse, error) {
	req := &pbfile.ListFileRequest{
		Uri:      uri,
		Page:     int32(page),
		PageSize: defaultPageSize,
	}
	return c.fc.ListDirectory(ctx, req)
}

func (c *fileClient) CreateFile(ctx context.Context, uri string) (*pbfile.FileResponse, error) {
	req := &pbfile.CreateFileRequest{
		Uri:           uri,
		Type:          "file",
		ErrOnConflict: true,
	}
	return c.fc.CreateFile(ctx, req)
}

func (c *fileClient) PutContentString(ctx context.Context, uri string, prev string, content string) (*pbfile.FileResponse, error) {
	//1. 将字符串包装为 io.Reader：操作极快，且不会复制字符串的内存
	reader := strings.NewReader(content)
	fileSize := int64(len(content))
	return c.PutContent(ctx, uri, prev, reader, fileSize)
}

func (c *fileClient) PutContent(ctx context.Context, uri string, prev string, reader io.Reader, fileSize int64) (*pbfile.FileResponse, error) {
	// 1. 调用 RPC 方法，获取一个 Stream 对象
	stream, err := c.fc.PutContentStream(ctx)
	if err != nil {
		return nil, err
	}

	// 2. 发送第一个包：文件元数据
	meta := &pbfile.UpdateFileInfo{
		Uri:      uri,
		Previous: prev,
		Size:     fileSize,
	}
	if err = stream.Send(&pbfile.StreamFileUpdateRequest{
		Payload: &pbfile.StreamFileUpdateRequest_FileInfo{FileInfo: meta},
	}); err != nil {
		return nil, err
	}

	// 3. 发送文件内容
	buf := make([]byte, 64*1024)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			// 发送数据包
			if err = stream.Send(&pbfile.StreamFileUpdateRequest{
				Payload: &pbfile.StreamFileUpdateRequest_ChunkData{ChunkData: buf[:n]},
			}); err != nil {
				return nil, err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}

	return stream.CloseAndRecv()
}
