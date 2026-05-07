package tool

import (
	"ai/ent"
	"ai/internal/data"
	"ai/internal/pkg/eino/tool/factory"
	"api/external/data/common"
	"context"
	"entmodule"

	"github.com/go-kratos/kratos/v2/log"
)

type toolBiz struct {
	tc data.ToolClient
	tr *factory.ToolRegistry
	l  *log.Helper
}

func (b *toolBiz) GetTool(ctx context.Context, id int) (*ent.AiTool, error) {
	return b.tc.GetByID(ctx, id)
}

func (b *toolBiz) GetTools(ctx context.Context, ids []int) ([]*ent.AiTool, error) {
	return b.tc.GetByIDs(ctx, ids)
}

func (b *toolBiz) Register(ctx context.Context) error {
	args := &data.ListToolArgs{
		PaginationArgs: &common.PaginationArgs{
			Page:     1,
			PageSize: 1000,
		},
		Status: entmodule.StatusActive,
	}
	for {
		res, err := b.tc.List(ctx, args)
		if err != nil {
			b.l.Errorf("failed to list tools: %v", err)
		}
		if res.PaginationResults.TotalItems == 0 {
			break
		}
		args.Page += 1
		for _, tool := range res.Tools {
			b.l.Infof("register tool %d", tool.ID)

		}
	}

	return nil
}
