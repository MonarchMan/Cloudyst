package data

import (
	"common/db"
	"context"
	"entmodule/ent/schema"
	"file/ent"
	"file/ent/fsevent"

	"github.com/gofrs/uuid"
	"github.com/samber/lo"
)

type FsEventClient interface {
	TxOperator
	// Create a new FsEvent
	Create(ctx context.Context, uid int, subscriberId uuid.UUID, events ...string) error
	// Delete all FsEvents by subscriber
	DeleteBySubscriber(ctx context.Context, subscriberId uuid.UUID) error
	// Delete all FsEvents
	DeleteAll(ctx context.Context) error
	// Get all FsEvents by subscriber and user
	TakeBySubscriber(ctx context.Context, subscriberId uuid.UUID, userId int) ([]*ent.FsEvent, error)
}

func NewFsEventClient(client *ent.Client, dbType db.DBType) FsEventClient {
	return &fsEventClient{client: client, maxSQlParam: db.SqlParamLimit(dbType)}
}

type fsEventClient struct {
	maxSQlParam int
	client      *ent.Client
}

func (c *fsEventClient) SetClient(newClient *ent.Client) TxOperator {
	return &fsEventClient{client: newClient, maxSQlParam: c.maxSQlParam}
}

func (c *fsEventClient) GetClient() *ent.Client {
	return c.client
}

func (c *fsEventClient) Create(ctx context.Context, uid int, subscriberId uuid.UUID, events ...string) error {
	stms := lo.Map(events, func(event string, index int) *ent.FsEventCreate {
		res := c.client.FsEvent.
			Create().
			SetUserID(uid).
			SetEvent(event).
			SetSubscriber(subscriberId).SetEvent(event)

		return res
	})

	_, err := c.client.FsEvent.CreateBulk(stms...).Save(ctx)
	return err
}

func (c *fsEventClient) DeleteBySubscriber(ctx context.Context, subscriberId uuid.UUID) error {
	_, err := c.client.FsEvent.Delete().Where(fsevent.Subscriber(subscriberId)).Exec(schema.SkipSoftDelete(ctx))
	return err
}

func (c *fsEventClient) DeleteAll(ctx context.Context) error {
	_, err := c.client.FsEvent.Delete().Exec(schema.SkipSoftDelete(ctx))
	return err
}

func (c *fsEventClient) TakeBySubscriber(ctx context.Context, subscriberId uuid.UUID, userId int) ([]*ent.FsEvent, error) {
	res, err := c.client.FsEvent.Query().Where(fsevent.Subscriber(subscriberId), fsevent.UserID(userId)).All(ctx)
	if err != nil {
		return nil, err
	}

	// Delete the FsEvents
	_, err = c.client.FsEvent.Delete().Where(fsevent.Subscriber(subscriberId), fsevent.UserID(userId)).Exec(schema.SkipSoftDelete(ctx))
	if err != nil {
		return nil, err
	}

	return res, nil
}
