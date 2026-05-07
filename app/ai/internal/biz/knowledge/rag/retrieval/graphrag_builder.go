package retrieval

import (
	"ai/internal/biz/knowledge/rag/graphrag"
	"ai/internal/biz/types"
	"ai/internal/data/vector"
	"ai/internal/pkg/eino/doc/rerank"
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/retriever/milvus2"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/retriever"
)

type RAGGraphBuilderConfig struct {
	QueryRewriter     graphrag.QueryRewriter
	QueryExpander     graphrag.QueryExpander
	NeighborExpander  graphrag.NeighborExpander
	ContentResolver   graphrag.SegmentContentResolver
	NeighborWindow    int
	IncludeOriginal   bool
	ContextCompressor graphrag.ContextCompressor
	AnswerGenerator   graphrag.AnswerGenerator
	AnswerVerifier    graphrag.AnswerVerifier
	Fallback          graphrag.FallbackHandler
	Evaluator         graphrag.Evaluator
	TraceObserver     graphrag.TraceObserver
	TopK              int
	ScoreThreshold    float64
	MaxQueries        int
	GenerateAnswer    bool
}

func (e *RetrieveEngine) BuildRAGGraph(ctx context.Context, emb embedding.Embedder, cfg *RAGGraphBuilderConfig) (*graphrag.Graph, error) {
	if cfg == nil {
		cfg = &RAGGraphBuilderConfig{}
	}
	r, err := NewMilvusRetriever(e.conf.Data.Milvus, emb)
	if err != nil {
		return nil, fmt.Errorf("build raggraph retriever: %w", err)
	}
	reranker, err := e.buildScoreReranker(cfg)
	if err != nil {
		return nil, err
	}
	neighborExpander, err := e.buildNeighborExpander(cfg)
	if err != nil {
		return nil, err
	}
	return graphrag.New(&graphrag.Config{
		Retriever:         r,
		Reranker:          reranker,
		QueryRewriter:     cfg.QueryRewriter,
		QueryExpander:     cfg.QueryExpander,
		NeighborExpander:  neighborExpander,
		ContextCompressor: cfg.ContextCompressor,
		AnswerGenerator:   cfg.AnswerGenerator,
		AnswerVerifier:    cfg.AnswerVerifier,
		Fallback:          cfg.Fallback,
		Evaluator:         cfg.Evaluator,
		TraceObserver:     cfg.TraceObserver,
		TopK:              cfg.TopK,
		ScoreThreshold:    cfg.ScoreThreshold,
		MaxQueries:        cfg.MaxQueries,
		GenerateAnswer:    cfg.GenerateAnswer,
	})
}

func (e *RetrieveEngine) BuildRAGGraphWithVectorStore(ctx context.Context, emb embedding.Embedder, vs vector.VectorStore, cfg *RAGGraphBuilderConfig) (*graphrag.Graph, error) {
	if vs == nil {
		return nil, fmt.Errorf("raggraph vector store is nil")
	}
	next := cloneRAGGraphBuilderConfig(cfg)
	if next.NeighborExpander == nil && next.ContentResolver == nil {
		next.ContentResolver = vs
	}
	return e.BuildRAGGraph(ctx, emb, next)
}

func (e *RetrieveEngine) buildNeighborExpander(cfg *RAGGraphBuilderConfig) (graphrag.NeighborExpander, error) {
	if cfg.NeighborExpander != nil {
		return cfg.NeighborExpander, nil
	}
	if cfg.ContentResolver == nil {
		return nil, nil
	}
	window := cfg.NeighborWindow
	if window <= 0 {
		window = 1
	}
	return graphrag.NewSegmentNeighborExpander(&graphrag.SegmentNeighborExpanderConfig{
		SegmentClient:   e.ksc,
		ContentResolver: cfg.ContentResolver,
		Window:          window,
		IncludeOriginal: cfg.IncludeOriginal,
	})
}

func cloneRAGGraphBuilderConfig(cfg *RAGGraphBuilderConfig) *RAGGraphBuilderConfig {
	if cfg == nil {
		return &RAGGraphBuilderConfig{}
	}
	next := *cfg
	return &next
}

func (e *RetrieveEngine) NewRAGGraphRequest(ctx context.Context, args *types.SegmentSearchArgs) (*graphrag.Request, error) {
	if args == nil {
		return nil, fmt.Errorf("segment search args is nil")
	}
	options := []retriever.Option{}
	if args.KnowledgeIDs != nil && len(args.KnowledgeIDs) > 0 {
		if err := e.validateKnowledges(ctx, args.KnowledgeIDs); err != nil {
			return nil, err
		}
		options = append(options, milvus2.WithFilter(vector.BuildKBFilter(args.KnowledgeIDs)))
	}
	return &graphrag.Request{
		Query:            args.Content,
		TopK:             args.TopK,
		ScoreThreshold:   args.Similarity,
		RetrieverOptions: options,
	}, nil
}

func (e *RetrieveEngine) buildScoreReranker(cfg *RAGGraphBuilderConfig) (document.Transformer, error) {
	topK := cfg.TopK
	if topK <= 0 {
		topK = 5
		if e.conf != nil && e.conf.Server != nil && e.conf.Server.Sys != nil && e.conf.Server.Retrieve != nil && e.conf.Server.Retrieve.TopK > 0 {
			topK = int(e.conf.Server.Retrieve.TopK)
		}
	}
	threshold := cfg.ScoreThreshold
	if threshold <= 0 {
		threshold = 0.6
		if e.conf != nil && e.conf.Server != nil && e.conf.Server.Sys != nil && e.conf.Server.Retrieve != nil && e.conf.Server.Retrieve.Similarity > 0 {
			threshold = e.conf.Server.Retrieve.Similarity
		}
	}
	return rerank.NewScoreReranker(&rerank.ScoreRerankerConfig{
		TopN:           topK,
		ScoreThreshold: threshold,
	})
}
