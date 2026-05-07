package memory

import (
	"context"
	"sort"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

type Type string

const (
	TypeShortTerm Type = "short_term"
	TypeLongTerm  Type = "long_term"
	TypeKnowledge Type = "knowledge"
)

type Item struct {
	Type     Type           `json:"type"`
	Source   string         `json:"source,omitempty"`
	Content  string         `json:"content"`
	Score    float64        `json:"score,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type Query struct {
	Text  string `json:"text"`
	Types []Type `json:"types,omitempty"`
	TopK  int    `json:"top_k,omitempty"`
}

type Retriever interface {
	Retrieve(ctx context.Context, query *Query) ([]*Item, error)
}

type RetrieverFunc func(ctx context.Context, query *Query) ([]*Item, error)

func (f RetrieverFunc) Retrieve(ctx context.Context, query *Query) ([]*Item, error) {
	return f(ctx, query)
}

type MultiRetriever struct {
	Retrievers []Retriever
}

func NewMultiRetriever(retrievers ...Retriever) *MultiRetriever {
	return &MultiRetriever{Retrievers: retrievers}
}

func (r *MultiRetriever) Retrieve(ctx context.Context, query *Query) ([]*Item, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var out []*Item
	for _, ret := range r.Retrievers {
		if ret == nil {
			continue
		}
		items, err := ret.Retrieve(ctx, query)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	if query != nil && query.TopK > 0 && len(out) > query.TopK {
		out = out[:query.TopK]
	}
	return out, nil
}

type EinoRetriever struct {
	Type      Type
	Source    string
	Retriever retriever.Retriever
}

func (r EinoRetriever) Retrieve(ctx context.Context, query *Query) ([]*Item, error) {
	if r.Retriever == nil {
		return nil, nil
	}
	text := ""
	if query != nil {
		text = query.Text
	}
	docs, err := r.Retriever.Retrieve(ctx, text)
	if err != nil {
		return nil, err
	}
	return ItemsFromDocuments(r.Type, r.Source, docs), nil
}

func ItemsFromDocuments(memoryType Type, source string, docs []*schema.Document) []*Item {
	items := make([]*Item, 0, len(docs))
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		items = append(items, &Item{
			Type:     memoryType,
			Source:   source,
			Content:  doc.Content,
			Metadata: map[string]any(doc.MetaData),
		})
	}
	return items
}
