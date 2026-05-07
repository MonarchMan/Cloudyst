package graphrag

import (
	"context"

	"ai/internal/pkg/eino/doc/enhance"

	"github.com/cloudwego/eino/schema"
)

type NeighborExpander interface {
	Expand(ctx context.Context, docs []*schema.Document) ([]*schema.Document, error)
}

type NeighborExpandFunc func(ctx context.Context, docs []*schema.Document) ([]*schema.Document, error)

func (f NeighborExpandFunc) Expand(ctx context.Context, docs []*schema.Document) ([]*schema.Document, error) {
	return f(ctx, docs)
}

type NoopNeighborExpander struct{}

func (NoopNeighborExpander) Expand(ctx context.Context, docs []*schema.Document) ([]*schema.Document, error) {
	return docs, nil
}

type StaticNeighborExpander struct {
	Documents map[string]*schema.Document
}

func (e *StaticNeighborExpander) Expand(ctx context.Context, docs []*schema.Document) ([]*schema.Document, error) {
	if e == nil || len(e.Documents) == 0 {
		return docs, nil
	}
	out := make([]*schema.Document, 0, len(docs)*3)
	seen := map[string]struct{}{}
	for _, doc := range docs {
		appendDoc(&out, seen, doc)
		if doc == nil || doc.MetaData == nil {
			continue
		}
		if prevID, ok := doc.MetaData[enhance.MetaChunkPrevID].(string); ok {
			appendDoc(&out, seen, e.Documents[prevID])
		}
		if nextID, ok := doc.MetaData[enhance.MetaChunkNextID].(string); ok {
			appendDoc(&out, seen, e.Documents[nextID])
		}
	}
	return out, nil
}

func appendDoc(out *[]*schema.Document, seen map[string]struct{}, doc *schema.Document) {
	if doc == nil {
		return
	}
	key := documentKey(doc)
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*out = append(*out, cloneDocument(doc))
}
