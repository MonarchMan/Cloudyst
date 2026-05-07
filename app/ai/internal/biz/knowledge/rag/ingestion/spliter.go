package ingestion

import (
	biztypes "ai/internal/biz/types"
	"ai/internal/data"
	"ai/internal/pkg/eino/doc/enhance"
	"ai/internal/pkg/utils"
	"context"
	"strconv"

	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/markdown"
	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/recursive"
	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/semantic"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

type (
	Node interface {
		Do(ctx context.Context, info *DocumentInfo) error
	}

	DynamicSplitter struct {
		ksc      data.KnowledgeSegmentClient
		Splitter document.Transformer
		conf     *DynamicSplitterConfig
	}

	DynamicSplitterConfig struct {
		Separators []string
	}

	Splitter struct {
		Splitter document.Transformer
		ksc      data.KnowledgeSegmentClient
	}
)

func NewMarkdownSplitter(ksc data.KnowledgeSegmentClient) (document.Transformer, error) {
	splitter, err := markdown.NewHeaderSplitter(context.Background(), &markdown.HeaderConfig{
		Headers:     map[string]string{"##": "headerNameOfLevel2"},
		TrimHeaders: false,
		IDGenerator: RandomIDGenerator(),
	})
	if err != nil {
		return nil, err
	}
	return &Splitter{
		Splitter: splitter,
		ksc:      ksc,
	}, nil
}

func NewSemanticSplitter(ksc data.KnowledgeSegmentClient, emb embedding.Embedder) (document.Transformer, error) {
	splitter, err := semantic.NewSplitter(context.Background(), &semantic.Config{
		Embedding:   emb,
		BufferSize:  1,
		IDGenerator: RandomIDGenerator(),
	})
	if err != nil {
		return nil, err
	}
	return &Splitter{
		Splitter: splitter,
		ksc:      ksc,
	}, nil
}

func (s *Splitter) Transform(ctx context.Context, src []*schema.Document, opts ...document.TransformerOption) ([]*schema.Document, error) {
	docInfo := GetDocumentInfo(ctx)
	// 1. 文本切分
	res, err := s.Splitter.Transform(ctx, src, opts...)
	if err != nil {
		return nil, err
	}
	return res, storeSegments(ctx, s.ksc, res, docInfo.ID)
}

func NewDynamicSplitter(ksc data.KnowledgeSegmentClient, conf *DynamicSplitterConfig) (document.Transformer, error) {
	if conf == nil {
		conf = &DynamicSplitterConfig{}
	}
	return &DynamicSplitter{
		ksc:  ksc,
		conf: conf,
	}, nil
}

func (n *DynamicSplitter) Transform(ctx context.Context, src []*schema.Document, opts ...document.TransformerOption) ([]*schema.Document, error) {
	docInfo := GetDocumentInfo(ctx)
	var (
		err      error
		splitter document.Transformer
	)
	strategy := docInfo.SplitStrategy
	switch strategy {
	case biztypes.StrategyParagraph:
		splitter, err = recursive.NewSplitter(ctx, &recursive.Config{
			ChunkSize:   docInfo.MaxTokens,
			OverlapSize: 0,
			Separators:  n.conf.Separators,
			IDGenerator: RandomIDGenerator(),
		})
	default:
		splitter, err = recursive.NewSplitter(ctx, &recursive.Config{
			ChunkSize:   docInfo.MaxTokens,
			OverlapSize: 50,
			Separators:  n.conf.Separators,
			IDGenerator: RandomIDGenerator(),
		})
	}
	if err != nil {
		return nil, err
	}
	// 1. 文本切分
	res, err := splitter.Transform(ctx, src, opts...)
	if err != nil {
		return nil, err
	}
	return res, storeSegments(ctx, n.ksc, res, docInfo.ID)
}

func storeSegments(ctx context.Context, ksc data.KnowledgeSegmentClient, docs []*schema.Document, docID int) error {
	// 切片记录入库
	docInfo := GetDocumentInfo(ctx)
	prepareChunkMetadata(docs)
	segs := make([]*biztypes.KnowledgeSegment, 0, len(docs))
	totalContentLen := 0
	totalTokens := 0
	for i, doc := range docs {
		tokens, err := utils.CountTokens(doc.Content, "")
		if err != nil {
			return err
		}
		totalContentLen += len(doc.Content)
		totalTokens += tokens
		knowledgeID := 0
		if docInfo != nil {
			knowledgeID = docInfo.KnowledgeID
		}
		segs = append(segs, &biztypes.KnowledgeSegment{
			DocumentID:  docID,
			KnowledgeID: knowledgeID,
			Content:     doc.Content,
			ContentLen:  len(doc.Content),
			Tokens:      tokens,
			ChunkIndex:  chunkIndex(doc.MetaData, i),
			SectionPath: segmentSectionPath(doc.MetaData),
			StartOffset: intMetaValue(doc.MetaData, enhance.MetaChunkStartOffset),
			EndOffset:   intMetaValue(doc.MetaData, enhance.MetaChunkEndOffset),
			Metadata:    doc.MetaData,
		})
	}
	if docInfo != nil {
		docInfo.IndexStats = buildDocumentIndexStats(docInfo, docs, totalContentLen, totalTokens)
	}
	segsRes, err := ksc.BatchCreate(ctx, segs)
	if err != nil {
		return err
	}
	for i, seg := range segsRes {
		docs[i].ID = strconv.Itoa(seg.ID)
	}
	return nil
}

func chunkIndex(meta map[string]any, fallback int) int {
	if meta == nil {
		return fallback
	}
	if index := intMetaValue(meta, enhance.MetaChunkIndex); index > 0 {
		return index
	}
	return fallback
}

func segmentSectionPath(meta map[string]any) string {
	if meta == nil {
		return ""
	}
	if section, ok := meta[enhance.MetaChunkSection].(string); ok {
		return section
	}
	if title, ok := meta[enhance.MetaTitle].(string); ok {
		return title
	}
	return ""
}

func intMetaValue(meta map[string]any, key string) int {
	if meta == nil {
		return 0
	}
	switch v := meta[key].(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func defaultSplitter() (document.Transformer, error) {
	return recursive.NewSplitter(context.Background(), &recursive.Config{
		ChunkSize:   1024,
		OverlapSize: 50,
	})
}

func RandomIDGenerator() func(ctx context.Context, originalID string, splitIndex int) string {
	return func(ctx context.Context, originalID string, splitIndex int) string {
		return uuid.New().String()
	}
}
