package graphrag

import (
	"ai/ent"
	"ai/internal/biz/types"
	"ai/internal/data"
	"ai/internal/pkg/eino/doc/enhance"
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"
)

const (
	MetaSegmentID   = "rag.segment.id"
	MetaDocumentID  = "rag.segment.document_id"
	MetaKnowledgeID = "rag.segment.knowledge_id"
	MetaStatus      = "rag.segment.status"
	MetaContentLen  = "rag.segment.content_len"
	MetaTokens      = "rag.segment.tokens"
	MetaStartOffset = "rag.segment.start_offset"
	MetaEndOffset   = "rag.segment.end_offset"
)

type SegmentContentResolver interface {
	GetContentByIDs(ctx context.Context, ids []string) ([]string, error)
}

type SegmentNeighborExpanderConfig struct {
	SegmentClient   data.KnowledgeSegmentClient
	ContentResolver SegmentContentResolver
	Window          int
	IncludeOriginal bool
}

type SegmentNeighborExpander struct {
	conf SegmentNeighborExpanderConfig
}

func NewSegmentNeighborExpander(conf *SegmentNeighborExpanderConfig) (*SegmentNeighborExpander, error) {
	if conf == nil {
		return nil, fmt.Errorf("segment neighbor expander config is nil")
	}
	if conf.SegmentClient == nil {
		return nil, fmt.Errorf("segment neighbor expander segment client is nil")
	}
	if conf.ContentResolver == nil {
		return nil, fmt.Errorf("segment neighbor expander content resolver is nil")
	}
	cfg := *conf
	if cfg.Window < 0 {
		cfg.Window = 0
	}
	return &SegmentNeighborExpander{conf: cfg}, nil
}

func (e *SegmentNeighborExpander) Expand(ctx context.Context, docs []*schema.Document) ([]*schema.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	vectorIDs := make([]string, 0, len(docs))
	originals := make(map[string]*schema.Document, len(docs))
	for _, doc := range docs {
		if doc == nil || doc.ID == "" {
			continue
		}
		vectorIDs = append(vectorIDs, doc.ID)
		originals[doc.ID] = doc
	}
	if len(vectorIDs) == 0 {
		return docs, nil
	}

	hitSegments, err := e.conf.SegmentClient.GetByVectorIDs(ctx, vectorIDs)
	if err != nil {
		return nil, err
	}
	segmentsByVectorID := make(map[string]*ent.AiKnowledgeSegment, len(hitSegments))
	for _, segment := range hitSegments {
		segmentsByVectorID[segment.VectorID] = segment
	}

	expanded := make([]*ent.AiKnowledgeSegment, 0, len(hitSegments)*(e.conf.Window*2+1))
	seenSegments := make(map[int]struct{})
	for _, vectorID := range vectorIDs {
		segment := segmentsByVectorID[vectorID]
		if segment == nil {
			continue
		}
		neighbors, err := e.conf.SegmentClient.GetNeighbors(ctx, segment.DocumentID, segment.ChunkIndex, e.conf.Window)
		if err != nil {
			return nil, err
		}
		for _, neighbor := range neighbors {
			if neighbor == nil {
				continue
			}
			if !e.conf.IncludeOriginal && neighbor.VectorID == vectorID {
				continue
			}
			if _, ok := seenSegments[neighbor.ID]; ok {
				continue
			}
			seenSegments[neighbor.ID] = struct{}{}
			expanded = append(expanded, neighbor)
		}
	}

	if len(expanded) == 0 {
		return docs, nil
	}
	return e.segmentsToDocuments(ctx, expanded, originals)
}

func (e *SegmentNeighborExpander) segmentsToDocuments(ctx context.Context, segments []*ent.AiKnowledgeSegment, originals map[string]*schema.Document) ([]*schema.Document, error) {
	vectorIDs := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment != nil && segment.VectorID != "" {
			vectorIDs = append(vectorIDs, segment.VectorID)
		}
	}
	contents, err := e.conf.ContentResolver.GetContentByIDs(ctx, vectorIDs)
	if err != nil {
		return nil, err
	}
	contentByVectorID := make(map[string]string, len(vectorIDs))
	for i, vectorID := range vectorIDs {
		if i < len(contents) {
			contentByVectorID[vectorID] = contents[i]
		}
	}

	docs := make([]*schema.Document, 0, len(segments))
	for _, segment := range segments {
		if segment == nil {
			continue
		}
		doc := documentFromSegment(segment, contentByVectorID[segment.VectorID])
		if original := originals[segment.VectorID]; original != nil {
			doc.WithScore(original.Score())
			for key, value := range original.MetaData {
				if _, ok := doc.MetaData[key]; !ok {
					doc.MetaData[key] = value
				}
			}
			if doc.Content == "" {
				doc.Content = original.Content
			}
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

func documentFromSegment(segment *ent.AiKnowledgeSegment, content string) *schema.Document {
	metadata := cloneMeta(segment.Metadata)
	metadata[MetaSegmentID] = segment.ID
	metadata[MetaDocumentID] = segment.DocumentID
	metadata[MetaKnowledgeID] = segment.KnowledgeID
	metadata[MetaStatus] = segment.Status
	metadata[MetaContentLen] = segment.ContentLength
	metadata[MetaTokens] = segment.Tokens
	metadata[enhance.MetaChunkIndex] = segment.ChunkIndex
	metadata[enhance.MetaChunkSection] = segment.SectionPath
	metadata[MetaStartOffset] = segment.StartOffset
	metadata[MetaEndOffset] = segment.EndOffset

	return &schema.Document{
		ID:       segment.VectorID,
		Content:  content,
		MetaData: metadata,
	}
}

func SegmentToKnowledgeSegment(segment *ent.AiKnowledgeSegment, content string) *types.KnowledgeSegment {
	if segment == nil {
		return nil
	}
	return &types.KnowledgeSegment{
		ID:          segment.ID,
		DocumentID:  segment.DocumentID,
		KnowledgeID: segment.KnowledgeID,
		Content:     content,
		ContentLen:  segment.ContentLength,
		Tokens:      segment.Tokens,
		ChunkIndex:  segment.ChunkIndex,
		SectionPath: segment.SectionPath,
		StartOffset: segment.StartOffset,
		EndOffset:   segment.EndOffset,
		Metadata:    cloneMeta(segment.Metadata),
		VectorID:    segment.VectorID,
		Status:      segment.Status,
	}
}
