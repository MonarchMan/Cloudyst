package retrieval

import (
	"ai/ent"
	"ai/internal/biz/types"
	"ai/internal/conf"
	"ai/internal/data"
	"ai/internal/data/vector"
	"ai/internal/pkg/eino/doc/rerank"
	"ai/internal/pkg/eino/tool/factory"
	"context"
	"entmodule"
	"fmt"

	"github.com/cloudwego/eino-ext/components/retriever/milvus2"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/go-kratos/kratos/v2/log"
)

type RetrieveEngine struct {
	kc   data.KnowledgeClient
	kdc  data.KnowledgeDocumentClient
	ksc  data.KnowledgeSegmentClient
	conf *conf.Bootstrap
	tr   *factory.ToolRegistry
	l    *log.Helper

	embedder    embedding.Embedder
	vectorStore vector.VectorStore
	chain       compose.Runnable[string, []*schema.Document]
}

func NewRetrieveEngine(kc data.KnowledgeClient, kdc data.KnowledgeDocumentClient, ksc data.KnowledgeSegmentClient, tr *factory.ToolRegistry,
	embedder embedding.Embedder, vectorStore vector.VectorStore, conf *conf.Bootstrap, l log.Logger) (*RetrieveEngine, error) {
	e := &RetrieveEngine{
		kc:          kc,
		kdc:         kdc,
		ksc:         ksc,
		tr:          tr,
		conf:        conf,
		l:           log.NewHelper(l, log.WithMessageKey("biz-knowledge-retrieveEngine")),
		embedder:    embedder,
		vectorStore: vectorStore,
	}
	var err error
	e.chain, err = e.buildRetrieveChain(context.Background(), embedder)
	if err != nil {
		return nil, err
	}
	e.registerTools()
	return e, nil
}

func (e *RetrieveEngine) buildRetrieveChain(ctx context.Context, emb embedding.Embedder) (compose.Runnable[string, []*schema.Document], error) {
	retriever, err := NewMilvusRetriever(e.conf.Data.Milvus, emb)
	if err != nil {
		e.l.Errorf("failed to initialize retriever: %v", err)
	}

	reranker, err := rerank.NewScoreReranker(&rerank.ScoreRerankerConfig{
		TopN:           5,
		ScoreThreshold: 0.6,
	})

	chain := compose.NewChain[string, []*schema.Document]()

	chain.AppendRetriever(retriever)
	chain.AppendDocumentTransformer(reranker)
	return chain.Compile(ctx)
}

func (e *RetrieveEngine) Retrieve(ctx context.Context, args *types.SegmentSearchArgs) ([]*types.KnowledgeSegment, error) {
	if args == nil {
		return nil, fmt.Errorf("segment search args is nil")
	}
	if args.UseGraphRAG {
		return e.RetrieveWithGraphRAG(ctx, args, nil)
	}
	return e.retrieveLegacy(ctx, args)
}

func (e *RetrieveEngine) retrieveLegacy(ctx context.Context, args *types.SegmentSearchArgs) ([]*types.KnowledgeSegment, error) {
	options := []retriever.Option{
		retriever.WithTopK(args.TopK * 3),
		retriever.WithScoreThreshold(args.Similarity),
	}
	if args.KnowledgeIDs != nil && len(args.KnowledgeIDs) > 0 {
		if err := e.validateKnowledges(ctx, args.KnowledgeIDs); err != nil {
			return nil, err
		}
		options = append(options, milvus2.WithFilter(vector.BuildKBFilter(args.KnowledgeIDs)))
	}
	// 1. 检索文档
	// TODO: 根据knowledge配置模型用 retriver router 去检索文档
	docs, err := e.chain.Invoke(ctx, args.Content,
		compose.WithRetrieverOption(options...),
	)
	if err != nil {
		return nil, err
	}
	vectorIDs := make([]string, 0, len(docs))
	for _, doc := range docs {
		vectorIDs = append(vectorIDs, doc.ID)
	}
	segments, err := e.ksc.GetByVectorIDs(ctx, vectorIDs)
	if err != nil {
		return nil, err
	}
	segIDs := make([]int, 0, len(segments))
	for _, segment := range segments {
		segIDs = append(segIDs, segment.ID)
	}
	err = e.ksc.UpdateRetrievalCountByIDs(ctx, segIDs, 1)
	if err != nil {
		return nil, err
	}

	// 3. 按 vectorIDs 顺序排序 segments
	// 3.1 创建 vectorID 到 segment 的映射
	segmentMap := make(map[string]*ent.AiKnowledgeSegment)
	for _, seg := range segments {
		segmentMap[seg.VectorID] = seg
	}
	// 3.2 检查 segments 与 vectorIDs 是否长度一致
	if len(segments) != len(vectorIDs) {
		e.l.Warnf("len(segments) != len(vectorIDs), segments: %v, vectorIDs: %v", segments, vectorIDs)
		segments = make([]*ent.AiKnowledgeSegment, len(vectorIDs))
	}

	// 3.2 按照 vectorIDs 顺序填充 segments
	for i, vectorID := range vectorIDs {
		if seg, ok := segmentMap[vectorID]; ok {
			segments[i] = seg
		} else {
			// 处理不存在的情况，例如设置为 nil 或跳过
			segments[i] = nil
		}
	}

	segsResp := make([]*types.KnowledgeSegment, 0, len(segments))
	for i, seg := range segments {
		if seg == nil {
			continue
		}
		segsResp = append(segsResp, &types.KnowledgeSegment{
			ID:          seg.ID,
			DocumentID:  seg.DocumentID,
			KnowledgeID: seg.KnowledgeID,
			Content:     docs[i].Content,
			ContentLen:  seg.ContentLength,
			Tokens:      seg.Tokens,
			Score:       docs[i].Score(),
			VectorID:    seg.VectorID,
		})
	}

	compose.ProcessState(ctx, func(ctx context.Context, state *types.ChatState) error {
		state.Record.Segs = segsResp
		return nil
	})
	return segsResp, nil
}

func (e *RetrieveEngine) RetrieveWithGraphRAG(ctx context.Context, args *types.SegmentSearchArgs, cfg *GraphRAGBuilderConfig) ([]*types.KnowledgeSegment, error) {
	if args == nil {
		return nil, fmt.Errorf("segment search args is nil")
	}
	if e.embedder == nil {
		return nil, fmt.Errorf("graphrag embedder is nil")
	}
	if e.vectorStore == nil {
		return nil, fmt.Errorf("graphrag vector store is nil")
	}

	graphCfg := cloneGraphRAGBuilderConfig(cfg)
	if graphCfg.TopK <= 0 {
		graphCfg.TopK = args.TopK
	}
	if graphCfg.ScoreThreshold <= 0 {
		graphCfg.ScoreThreshold = args.Similarity
	}
	if graphCfg.NeighborWindow <= 0 {
		graphCfg.NeighborWindow = args.NeighborWindow
	}
	graphCfg.IncludeOriginal = !args.ExcludeOriginal

	graph, err := e.BuildGraphRAGWithVectorStore(ctx, e.embedder, e.vectorStore, graphCfg)
	if err != nil {
		return nil, err
	}
	req, err := e.NewGraphRAGRequest(ctx, args)
	if err != nil {
		return nil, err
	}
	result, err := graph.Invoke(ctx, req)
	if err != nil {
		return nil, err
	}
	return e.graphRAGDocumentsToKnowledgeSegments(ctx, result.Documents)
}

func (e *RetrieveEngine) graphRAGDocumentsToKnowledgeSegments(ctx context.Context, docs []*schema.Document) ([]*types.KnowledgeSegment, error) {
	vectorIDs := make([]string, 0, len(docs))
	docByVectorID := make(map[string]*schema.Document, len(docs))
	for _, doc := range docs {
		if doc == nil || doc.ID == "" {
			continue
		}
		if _, ok := docByVectorID[doc.ID]; ok {
			continue
		}
		vectorIDs = append(vectorIDs, doc.ID)
		docByVectorID[doc.ID] = doc
	}
	if len(vectorIDs) == 0 {
		return nil, nil
	}

	segments, err := e.ksc.GetByVectorIDs(ctx, vectorIDs)
	if err != nil {
		return nil, err
	}
	segIDs := make([]int, 0, len(segments))
	segmentByVectorID := make(map[string]*ent.AiKnowledgeSegment, len(segments))
	for _, segment := range segments {
		if segment == nil {
			continue
		}
		segIDs = append(segIDs, segment.ID)
		segmentByVectorID[segment.VectorID] = segment
	}
	if len(segIDs) > 0 {
		if err := e.ksc.UpdateRetrievalCountByIDs(ctx, segIDs, 1); err != nil {
			return nil, err
		}
	}

	segsResp := make([]*types.KnowledgeSegment, 0, len(vectorIDs))
	for _, vectorID := range vectorIDs {
		seg := segmentByVectorID[vectorID]
		doc := docByVectorID[vectorID]
		if seg == nil || doc == nil {
			continue
		}
		segsResp = append(segsResp, &types.KnowledgeSegment{
			ID:          seg.ID,
			DocumentID:  seg.DocumentID,
			KnowledgeID: seg.KnowledgeID,
			Content:     doc.Content,
			ContentLen:  seg.ContentLength,
			Tokens:      seg.Tokens,
			ChunkIndex:  seg.ChunkIndex,
			SectionPath: seg.SectionPath,
			StartOffset: seg.StartOffset,
			EndOffset:   seg.EndOffset,
			Metadata:    seg.Metadata,
			Score:       doc.Score(),
			VectorID:    seg.VectorID,
			Status:      seg.Status,
		})
	}

	compose.ProcessState(ctx, func(ctx context.Context, state *types.ChatState) error {
		state.Record.Segs = segsResp
		return nil
	})
	return segsResp, nil
}

func (e *RetrieveEngine) validateKnowledges(ctx context.Context, ids []int) error {
	ks, err := e.kc.GetByIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("failed to get knowledges: %w", err)
	}
	if len(ks) != len(ids) {
		return fmt.Errorf("some knowledge is invalid")
	}

	for _, k := range ks {
		if k.Status != entmodule.StatusActive {
			return fmt.Errorf("knowledge %d is inactive", k.ID)
		}
	}
	return nil
}
