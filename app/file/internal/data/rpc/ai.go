package rpc

import (
	pbadmin "api/api/ai/admin/v1"
	pbknowledge "api/api/ai/knowledge/v1"
	"context"
	"fmt"

	"github.com/go-kratos/kratos/contrib/registry/consul/v2"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/hashicorp/consul/api"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
)

type KnowledgeClient struct {
	ac pbadmin.AdminClient
	kc pbknowledge.KnowledgeClient
}

func NewKnowledgeClient(client *api.Client) (*KnowledgeClient, error) {
	// 1. 创建服务发现实例
	dis := consul.New(client)

	// 2. 通过服务名连接（不需要写死 IP）
	conn, err := grpc.DialInsecure(
		context.Background(),
		grpc.WithEndpoint("discovery:///cloudyst-ai"), // 🔑 关键格式
		grpc.WithDiscovery(dis),                       // 🔑 启用服务发现
		grpc.WithMiddleware(
			recovery.Recovery(),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to dial ai grpc: %w", err)
	}

	return &KnowledgeClient{
		ac: pbadmin.NewAdminClient(conn),
		kc: pbknowledge.NewKnowledgeClient(conn),
	}, nil
}

func (c *KnowledgeClient) GetMasterKnowledge(ctx context.Context, userID int) (*pbknowledge.GetKnowledgeResponse, error) {
	req := &pbknowledge.GetMasterKnowledgeRequest{
		UserId: int64(userID),
	}
	resp, err := c.kc.GetMasterKnowledge(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *KnowledgeClient) CreateDocument(ctx context.Context, doc *CreateDocumentRequest) (*pbknowledge.GetDocumentResponse, error) {
	req := &pbknowledge.UpsertDocumentRequest{
		KnowledgeId: doc.KnowledgeId,
		Name:        doc.DocumentName,
		Url:         doc.Uri,
		Version:     doc.Version,
	}
	resp, err := c.kc.CreateDocument(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *KnowledgeClient) CreateDocuments(ctx context.Context, knowledgeID string, docs []*CreateDocumentRequest) ([]*pbknowledge.GetDocumentResponse, int64, error) {
	req := &pbknowledge.BatchCreateDocumentRequest{
		Documents: lo.Map(docs, func(doc *CreateDocumentRequest, index int) *pbknowledge.UpsertDocumentRequest {
			return &pbknowledge.UpsertDocumentRequest{
				KnowledgeId: knowledgeID,
				Name:        doc.DocumentName,
				Url:         doc.Uri,
			}
		}),
	}
	resp, err := c.kc.BatchCreateDocuments(ctx, req)
	if err != nil {
		return nil, 0, err
	}
	return resp.Documents, resp.Total, nil
}

func (c *KnowledgeClient) DeleteDocuments(ctx context.Context, ids ...string) error {
	req := &pbknowledge.BatchDeleteRequest{
		Ids: ids,
	}
	_, err := c.kc.BatchDeleteDocuments(ctx, req)
	return err
}

func (c *KnowledgeClient) Search(ctx context.Context, query string) ([]*pbknowledge.SearchResult, int64, error) {
	req := &pbknowledge.SearchRequest{
		Query: query,
	}
	resp, err := c.kc.Search(ctx, req)
	if err != nil {
		return nil, 0, err
	}
	return resp.Results, resp.Total, nil
}

func (c *KnowledgeClient) CopyDocument(ctx context.Context, docID string, version string) (*pbknowledge.GetDocumentResponse, error) {
	req := &pbknowledge.CopyDocumentRequest{
		DocId:   docID,
		Version: version,
	}
	resp, err := c.kc.CopyDocument(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *KnowledgeClient) ChangeDocumentOwner(ctx context.Context, docID string, oldOwnerID, newOwnerID int) (*pbknowledge.GetDocumentResponse, error) {
	oldKnowledge, err := c.kc.GetMasterKnowledge(ctx, &pbknowledge.GetMasterKnowledgeRequest{
		UserId: int64(oldOwnerID),
	})
	if err != nil {
		return nil, err
	}
	newKnowledge, err := c.kc.GetMasterKnowledge(ctx, &pbknowledge.GetMasterKnowledgeRequest{
		UserId: int64(newOwnerID),
	})
	if err != nil {
		return nil, err
	}

	req := &pbknowledge.ChangeDocumentOwnerRequest{
		DocId:          docID,
		OldKnowledgeId: oldKnowledge.Id,
		NewKnowledgeId: newKnowledge.Id,
	}
	resp, err := c.kc.ChangeDocumentOwner(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *KnowledgeClient) GetSupportTextParseTypes(ctx context.Context) ([]string, int64, error) {
	resp, err := c.kc.GetSupportTextParse(ctx, &emptypb.Empty{})
	if err != nil {
		return []string{}, 0, err
	}
	return resp.Types, resp.MaxFileSize, nil
}

func (c *KnowledgeClient) Rename(ctx context.Context, id int, id2 int, name string) error {
	return fmt.Errorf("rename not implemented")
}
