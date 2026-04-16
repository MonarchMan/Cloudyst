package data

import (
	"common/db"
	"common/hashid"
	"context"
	mschema "entmodule/ent/schema"
	"file/ent"
	"file/ent/directlink"
)

type (
	DirectLinkClient interface {
		TxOperator
		// GetByNameID get direct link by name and id
		GetByNameID(ctx context.Context, id int, name string) (*ent.DirectLink, error)
		// GetByID get direct link by id
		GetByID(ctx context.Context, id int) (*ent.DirectLink, error)
		// Delete delete direct link by id
		Delete(ctx context.Context, id int) error
	}
	LoadDirectLinkFile struct{}
)

func NewDirectLinkClient(client *ent.Client, dbType db.DBType, hasher hashid.Encoder) DirectLinkClient {
	return &directLinkClient{
		client:      client,
		hasher:      hasher,
		maxSQlParam: db.SqlParamLimit(dbType),
	}
}

type directLinkClient struct {
	maxSQlParam int
	client      *ent.Client
	hasher      hashid.Encoder
}

func (c *directLinkClient) SetClient(newClient *ent.Client) TxOperator {
	return &directLinkClient{client: newClient, hasher: c.hasher, maxSQlParam: c.maxSQlParam}
}

func (c *directLinkClient) GetClient() *ent.Client {
	return c.client
}

func (c *directLinkClient) GetByID(ctx context.Context, id int) (*ent.DirectLink, error) {
	return withDirectLinkEagerLoading(ctx, c.client.DirectLink.Query().Where(directlink.ID(id))).
		First(ctx)
}

func (c *directLinkClient) GetByNameID(ctx context.Context, id int, name string) (*ent.DirectLink, error) {
	res, err := withDirectLinkEagerLoading(ctx, c.client.DirectLink.Query().Where(directlink.ID(id), directlink.Name(name))).
		First(ctx)
	if err != nil {
		return nil, err
	}

	// Increase download counter
	_, _ = c.client.DirectLink.Update().Where(directlink.ID(res.ID)).SetDownloads(res.Downloads + 1).Save(ctx)

	return res, nil
}

func (c *directLinkClient) Delete(ctx context.Context, id int) error {
	ctx = mschema.SkipSoftDelete(ctx)
	_, err := c.client.DirectLink.Delete().Where(directlink.ID(id)).Exec(ctx)
	return err
}

func withDirectLinkEagerLoading(ctx context.Context, q *ent.DirectLinkQuery) *ent.DirectLinkQuery {
	if v, ok := ctx.Value(LoadDirectLinkFile{}).(bool); ok && v {
		q.WithFile(func(m *ent.FileQuery) {
			withFileEagerLoading(ctx, m)
		})
	}
	return q
}
