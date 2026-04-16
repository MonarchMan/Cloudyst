package chat

import (
	"ai/internal/biz/types"
	"ai/internal/conf"
	"ai/internal/data"
	"ai/internal/pkg/eino/tool/search/bocha"
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
)

type Searcher struct {
	bocha *bocha.BochaSearch
	wpc   data.WebPageClient
}

func NewBochaTool(conf *conf.Bocha, wpc data.WebPageClient) (tool.InvokableTool, error) {
	cfg := &bocha.Config{
		APIKey:    conf.ApiKey,
		Endpoint:  conf.Endpoint,
		Count:     int(conf.Count),
		Freshness: bocha.Freshness(conf.Freshness),
		Summary:   conf.Summary,
	}
	bs, err := bocha.NewBoCha(cfg)
	if err != nil {
		return nil, err
	}
	bt := &Searcher{
		bocha: bs,
		wpc:   wpc,
	}
	// 转换成 tool.InvokableTool
	tl, err := utils.InferTool(
		cfg.ToolName,
		cfg.ToolDesc,
		bt.SearchByBocha,
		utils.WithMarshalOutput(bs.MarshalOutput),
	)
	if err != nil {
		return nil, fmt.Errorf("bocha: InferTool failed: %w", err)
	}

	return tl, nil
}

func (m *Searcher) SearchByBocha(ctx context.Context, req *bocha.SearchRequest) (*bocha.BochaResponse, error) {
	// 1. 调用bocha api搜索网页
	res, err := m.bocha.Search(ctx, req)
	if err != nil {
		return nil, err
	}

	err = compose.ProcessState(ctx, func(ctx context.Context, s *types.ChatState) error {
		// 2. 插入网页记录
		var webSearchRes = types.WebSearchResult{
			Total: res.Data.WebPages.TotalEstimatedMatches,
		}
		pages := res.Data.WebPages.Value
		webSearchRes.WebPages = make([]*types.WebPage, len(pages))
		for i, page := range pages {
			webSearchRes.WebPages[i] = &types.WebPage{
				Name:      page.SiteName,
				Icon:      page.SiteIcon,
				Title:     page.Name,
				URL:       page.URL,
				Snippet:   page.Snippet,
				Summary:   page.Summary,
				MessageID: s.MsgID,
			}
		}
		_, err = m.wpc.BatchCreate(ctx, webSearchRes.WebPages)
		if err != nil {
			return fmt.Errorf("failed to batch create web pages: %w", err)
		}
		// 3. 更新全局状态
		s.Record.WebSearch = &webSearchRes
		return nil
	})
	return res, err
}
