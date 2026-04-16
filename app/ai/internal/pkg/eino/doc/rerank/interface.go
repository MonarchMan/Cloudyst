package rerank

import (
	"github.com/cloudwego/eino/components/document"
)

// Reranker 实现 document.Transformer，query 通过 Option 注入
// Transform(ctx, docs, WithQuery("用户问题")) → 重排后的 docs
type Reranker interface {
	document.Transformer
}

// Options 扩展：注入 query 和其他参数
type Options struct {
	Query          string
	TopN           int
	ScoreThreshold float64
}

type Option func(*Options)

func WithQuery(query string) document.TransformerOption {
	return document.WrapTransformerImplSpecificOptFn(func(o *Options) {
		o.Query = query
	})
}

func WithTopN(n int) document.TransformerOption {
	return document.WrapTransformerImplSpecificOptFn(func(o *Options) {
		o.TopN = n
	})
}

func WithScoreThreshold(threshold float64) document.TransformerOption {
	return document.WrapTransformerImplSpecificOptFn(func(o *Options) {
		o.ScoreThreshold = threshold
	})
}
