package doc

import (
	"ai/internal/conf"
	"context"
	"fmt"
	"io"

	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/markdown"
	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/recursive"
	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/semantic"
	"github.com/cloudwego/eino-ext/components/embedding/ollama"
	"github.com/cloudwego/eino-ext/components/indexer/milvus2"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/document/parser"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

// SplitStrategy 文档切片策略
type SplitStrategy string

const (
	StrategyAuto           SplitStrategy = "auto"
	StrategyMarkdownHeader SplitStrategy = "markdown_header"
	StrategySemantic       SplitStrategy = "semantic"
	StrategyParagraph      SplitStrategy = "paragraph"
	StrategySentence       SplitStrategy = "sentence"
)

type (
	Splitter interface {
		SplitText(ctx context.Context, docs []*schema.Document, stg SplitStrategy) ([]*schema.Document, error)
	}
	splitter struct {
		selector map[SplitStrategy]document.Transformer
		l        *log.Helper
	}

	SplitConfig struct {
		MaxTokens  int
		Separators []string
	}
)

func NewSplitter(l log.Logger, cfg *SplitConfig) Splitter {
	h := log.NewHelper(l, log.WithMessageKey("doc-recursiveSplitter"))
	ctx := context.Background()
	selector := make(map[SplitStrategy]document.Transformer)
	mt, err := markdown.NewHeaderSplitter(ctx, &markdown.HeaderConfig{
		Headers:     map[string]string{"##": "headerNameOfLevel2"},
		TrimHeaders: false,
	})
	if err != nil {
		h.Errorf("failed to initialize markdown header recursiveSplitter: %v", err)
	}
	selector[StrategyMarkdownHeader] = mt

	embedder, err := ollama.NewEmbedder(ctx, &ollama.EmbeddingConfig{
		BaseURL: "http://localhost:11434",
		Model:   "",
	})
	if err != nil {
		h.Errorf("failed to initialize ollama embedder: %v", err)
	}
	smt, err := semantic.NewSplitter(ctx, &semantic.Config{
		Embedding:  embedder,
		BufferSize: 1,
		Separators: cfg.Separators,
	})
	if err != nil {
		h.Errorf("failed to initialize semantic recursiveSplitter: %v", err)
	}
	selector[StrategySemantic] = smt

	pt, err := recursive.NewSplitter(ctx, &recursive.Config{
		ChunkSize:   cfg.MaxTokens,
		OverlapSize: 0,
		Separators:  cfg.Separators,
	})
	if err != nil {
		h.Errorf("failed to initialize paragraph recursiveSplitter: %v", err)
	}
	selector[StrategyParagraph] = pt

	stt, err := recursive.NewSplitter(ctx, &recursive.Config{
		ChunkSize:   cfg.MaxTokens,
		OverlapSize: 50,
		Separators:  cfg.Separators,
	})
	if err != nil {
		h.Errorf("failed to initialize sentence recursiveSplitter: %v", err)
	}
	selector[StrategySentence] = stt

	return &splitter{selector: selector, l: h}
}

func (s *splitter) SplitText(ctx context.Context, docs []*schema.Document, stg SplitStrategy) ([]*schema.Document, error) {
	transformer, ok := s.selector[stg]
	if !ok {
		return nil, fmt.Errorf("recursiveSplitter %s not found", stg)
	}
	return transformer.Transform(ctx, docs)
}

func NewIndexer(cfg *conf.Bootstrap, emb embedding.Embedder) (indexer.Indexer, error) {
	ctx := context.Background()
	return milvus2.NewIndexer(ctx, &milvus2.IndexerConfig{
		ClientConfig: &milvusclient.ClientConfig{
			Address:  cfg.Extensions.Milvus.Addr,
			Username: cfg.Extensions.Milvus.Username,
			Password: cfg.Extensions.Milvus.Password,
		},
		Collection: cfg.Extensions.Milvus.Collection,
		Vector: &milvus2.VectorConfig{
			Dimension:    1024, // 与 embedding 模型维度匹配
			MetricType:   milvus2.COSINE,
			IndexBuilder: milvus2.NewHNSWIndexBuilder().WithM(16).WithEfConstruction(200),
		},
		Embedding: emb,
	})
}

type (
	Loader interface {
		Load(ctx context.Context, uri string) ([]*schema.Document, error)
	}
)

func AddParserNode[I, O any](g compose.Graph[I, O], key string, parser parser.Parser, opts ...compose.GraphAddNodeOpt) error {
	return g.AddLambdaNode(key, compose.InvokableLambda(func(ctx context.Context, input io.ReadCloser) (output []*schema.Document, err error) {
		return parser.Parse(ctx, input)
	}), opts...)
}
