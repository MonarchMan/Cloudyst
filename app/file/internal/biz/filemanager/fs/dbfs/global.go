package dbfs

import (
	"context"
	"file/ent"
	"file/internal/biz/filemanager/fs"
	"file/internal/data"

	"github.com/samber/lo"
)

func (f *DBFS) StaleEntities(ctx context.Context, entities ...int) ([]fs.Entity, error) {
	res, err := f.fileClient.StaleEntities(ctx, entities...)
	if err != nil {
		return nil, err
	}

	return lo.Map(res, func(e *ent.Entity, i int) fs.Entity {
		return fs.NewEntity(e)
	}), nil
}

func (f *DBFS) AllFilesInTrashBin(ctx context.Context, opts ...fs.Option) (*fs.ListFileResult, error) {
	o := newDbfsOption()
	for _, opt := range opts {
		o.apply(opt)
	}

	navigator, err := f.getNavigator(ctx, newTrashUri(""), NavigatorCapabilityListChildren)
	if err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, data.LoadFilePublicMetadata{}, true)
	children, err := navigator.Children(ctx, nil, &ListArgs{
		Page: &data.PaginationArgs{
			Page:                o.FsOption.Page,
			PageSize:            o.PageSize,
			OrderBy:             o.OrderBy,
			Order:               data.OrderDirection(o.OrderDirection),
			UseCursorPagination: o.useCursorPagination,
			PageToken:           o.pageToken,
		},
	})
	if err != nil {
		return nil, err
	}

	return &fs.ListFileResult{
		Files: lo.Map(children.Files, func(item *File, index int) fs.File {
			return item
		}),
		Pagination:            children.Pagination,
		RecursionLimitReached: children.RecursionLimitReached,
	}, nil
}
