package rerank

import (
	"context"
	"sort"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

// ScoreRerankerConfig 基于分数的重排序器配置
type ScoreRerankerConfig struct {
	TopN           int     // 最终保留的文档数
	ScoreThreshold float64 // 分数阈值，低于此值的文档过滤掉
	ScoreKey       string  // MetaData 里存分数的 key，默认 "score"
}

type ScoreReranker struct {
	conf *ScoreRerankerConfig
}

func NewScoreReranker(cfg *ScoreRerankerConfig) (document.Transformer, error) {
	scoreKey := cfg.ScoreKey
	if scoreKey == "" {
		scoreKey = "score"
	}
	return &ScoreReranker{
		conf: cfg,
	}, nil
}

// Transform 实现 document.Transformer 接口
func (r *ScoreReranker) Transform(
	ctx context.Context,
	docs []*schema.Document,
	opts ...document.TransformerOption,
) ([]*schema.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	// 1. 过滤低分文档
	filtered := make([]*schema.Document, 0, len(docs))
	for _, doc := range docs {
		if doc.Score() >= r.conf.ScoreThreshold {
			filtered = append(filtered, doc)
		}
	}

	// 2. 按分数降序排列
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Score() > filtered[j].Score()
	})

	// 3. 截取 TopN
	if r.conf.TopN > 0 && len(filtered) > r.conf.TopN {
		filtered = filtered[:r.conf.TopN]
	}

	return filtered, nil
}
