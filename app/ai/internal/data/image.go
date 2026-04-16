package data

import (
	"ai/ent"
	"ai/ent/aiimage"
	"ai/internal/biz/types"
	pb "api/api/common/v1"
	"common/db"
	"context"
)

type (
	ImageClient interface {
		TxOperator
		GetByID(ctx context.Context, id int) (*ent.AiImage, error)
		GetByIDs(ctx context.Context, ids []int) ([]*ent.AiImage, error)
		List(ctx context.Context, args *ListAIImageArgs) (*ListAIImageResult, error)
		Upsert(ctx context.Context, image *ent.AiImage) (*ent.AiImage, error)
		UpdateStatus(ctx context.Context, id int, status types.ImageStatus) (*ent.AiImage, error)
		Delete(ctx context.Context, id int) error
		BatchDelete(ctx context.Context, ids []int) (int, error)
	}

	imageClient struct {
		client      *ent.Client
		maxSQLParam int
	}

	ListAIImageArgs struct {
		*pb.PaginationArgs
		Platform string
		ModelID  int
		UserID   int
		Status   types.ImageStatus
	}

	ListAIImageResult struct {
		*pb.PaginationResults
		Images []*ent.AiImage
	}
)

func NewAIImageClient(client *ent.Client, dbType db.DBType) ImageClient {
	return &imageClient{client: client, maxSQLParam: db.SqlParamLimit(dbType)}
}

func (c *imageClient) SetClient(newClient *ent.Client) TxOperator {
	return &imageClient{client: newClient, maxSQLParam: c.maxSQLParam}
}

func (c *imageClient) GetClient() *ent.Client {
	return c.client
}

func (c *imageClient) GetByID(ctx context.Context, id int) (*ent.AiImage, error) {
	return c.client.AiImage.Get(ctx, id)
}

func (c *imageClient) GetByIDs(ctx context.Context, ids []int) ([]*ent.AiImage, error) {
	return c.client.AiImage.Query().Where(aiimage.IDIn(ids...)).All(ctx)
}

func (c *imageClient) List(ctx context.Context, args *ListAIImageArgs) (*ListAIImageResult, error) {
	pageSize := db.CapPageSize(c.maxSQLParam, int(args.PageSize), 10)
	q := c.client.AiImage.Query()
	if args.Platform != "" {
		q.Where(aiimage.Platform(args.Platform))
	}

	if args.ModelID != 0 {
		q.Where(aiimage.ModelID(args.ModelID))
	}

	if args.UserID != 0 {
		q.Where(aiimage.UserID(args.UserID))
	}

	if args.Status != "" {
		q.Where(aiimage.StatusEQ(args.Status))
	}

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, err
	}

	q.Order(getImageOrderOption(args)...)
	images, err := q.Limit(int(args.PageSize)).Offset(int(args.Page) * pageSize).All(ctx)
	if err != nil {
		return nil, err
	}

	return &ListAIImageResult{
		PaginationResults: &pb.PaginationResults{
			Page:       args.Page,
			PageSize:   int32(pageSize),
			TotalItems: int32(total),
		},
		Images: images,
	}, nil
}

func (c *imageClient) Upsert(ctx context.Context, image *ent.AiImage) (*ent.AiImage, error) {
	if image.ID == 0 {
		q := c.client.AiImage.Create().
			SetUserID(image.UserID).
			SetPlatform(image.Platform).
			SetModelID(image.ModelID).
			SetModel(image.Model).
			SetPrompt(image.Prompt).
			SetWidth(image.Width).
			SetHeight(image.Height).
			SetOptions(image.Options).
			SetStatus(image.Status).
			SetPicURL(image.PicURL).
			SetTaskID(image.TaskID).
			SetButtons(image.Buttons)
		return q.Save(ctx)
	}
	q := c.client.AiImage.UpdateOneID(image.ID).
		SetWidth(image.Width).
		SetHeight(image.Height).
		SetStatus(image.Status).
		SetPicURL(image.PicURL).
		SetTaskID(image.TaskID).
		SetButtons(image.Buttons)

	return q.Save(ctx)
}

func (c *imageClient) UpdateStatus(ctx context.Context, id int, status types.ImageStatus) (*ent.AiImage, error) {
	return c.client.AiImage.UpdateOneID(id).SetStatus(status).Save(ctx)
}

func (c *imageClient) Delete(ctx context.Context, id int) error {
	return c.client.AiImage.DeleteOneID(id).Exec(ctx)
}

func (c *imageClient) BatchDelete(ctx context.Context, ids []int) (int, error) {
	return c.client.AiImage.Delete().Where(aiimage.IDIn(ids...)).Exec(ctx)
}

func getImageOrderOption(args *ListAIImageArgs) []aiimage.OrderOption {
	orderTerm := db.GetOrderTerm(db.OrderDirection(args.OrderDirection))
	switch args.OrderBy {
	case aiimage.FieldUpdatedAt:
		return []aiimage.OrderOption{aiimage.ByUpdatedAt(orderTerm), aiimage.ByID(orderTerm)}
	default:
		return []aiimage.OrderOption{aiimage.ByID(orderTerm)}
	}
}
