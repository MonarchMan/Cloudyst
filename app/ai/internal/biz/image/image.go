package image

import (
	"ai/ent"
	"ai/internal/biz/types"
	"ai/internal/data"
	"context"
	"fmt"
)

type (
	ImageBiz interface {
		ListImages(ctx context.Context, args *data.ListAIImageArgs) (*data.ListAIImageResult, error)
		UpdateImageStatus(ctx context.Context, id int, status types.ImageStatus) (*ent.AiImage, error)
		DeleteImage(ctx context.Context, id int) error
		BatchDeleteImages(ctx context.Context, ids []int) ([]int, error)
	}
	imageBiz struct {
		ic data.ImageClient
	}
)

func NewImageBiz(ic data.ImageClient) ImageBiz {
	return &imageBiz{ic}
}

func (b *imageBiz) ListImages(ctx context.Context, args *data.ListAIImageArgs) (*data.ListAIImageResult, error) {
	return b.ic.List(ctx, args)
}

func (b *imageBiz) UpdateImageStatus(ctx context.Context, id int, status types.ImageStatus) (*ent.AiImage, error) {
	_, err := b.validateImage(ctx, id)
	if err != nil {
		return nil, err
	}
	return b.UpdateImageStatus(ctx, id, status)
}

func (b *imageBiz) validateImage(ctx context.Context, id int) (*ent.AiImage, error) {
	img, err := b.ic.GetByID(ctx, id)
	if err != nil || img == nil {
		return nil, fmt.Errorf("failed to get image by id %d: %v", id, err)
	}
	return img, nil
}

func (b *imageBiz) DeleteImage(ctx context.Context, id int) error {
	_, err := b.validateImage(ctx, id)
	if err != nil {
		return err
	}
	return b.ic.Delete(ctx, id)
}

func (b *imageBiz) BatchDeleteImages(ctx context.Context, ids []int) ([]int, error) {
	imageIDs := make([]int, len(ids))
	if len(ids) == 0 {
		return imageIDs, nil
	}

	// query the images
	imgs, err := b.ic.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i, img := range imgs {
		imageIDs[i] = img.ID
	}
	ic, tx, ctx, err := data.WithTx(ctx, b.ic)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	// Delete the images
	if num, err := ic.BatchDelete(ctx, ids); err != nil {
		_ = data.Rollback(tx)
		return nil, fmt.Errorf("failed to batch delete image: %d %w", num, err)
	}

	if err := data.Commit(tx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	return imageIDs, nil
}
