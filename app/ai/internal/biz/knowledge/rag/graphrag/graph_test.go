package graphrag

import (
	"context"
	"strings"
	"testing"

	"ai/internal/pkg/eino/doc/enhance"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

type fakeRetriever struct {
	query   string
	queries []string
}

func (r *fakeRetriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	r.query = query
	r.queries = append(r.queries, query)
	return []*schema.Document{
		(&schema.Document{
			ID:      "seg-1",
			Content: "OAuth2 token rotation requires a refresh window.",
			MetaData: map[string]any{
				enhance.MetaTitle: "Auth Guide",
				enhance.MetaTerms: []string{"OAuth2"},
			},
		}).WithScore(0.92),
		(&schema.Document{
			ID:      "seg-2",
			Content: "This lower ranked segment should be removed by reranker.",
		}).WithScore(0.5),
	}, nil
}

type firstDocumentTransformer struct{}

func (firstDocumentTransformer) Transform(ctx context.Context, docs []*schema.Document, opts ...document.TransformerOption) ([]*schema.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}
	return docs[:1], nil
}

func TestGraphInvokeRewritesRetrievesReranksAndAssembles(t *testing.T) {
	fr := &fakeRetriever{}
	graph, err := New(&Config{
		Retriever: fr,
		Reranker:  firstDocumentTransformer{},
		QueryRewriter: QueryRewriteFunc(func(ctx context.Context, query string) (string, error) {
			return query + " rewritten", nil
		}),
		MaxContextChars: 200,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := graph.Invoke(context.Background(), &Request{Query: " token policy ", TopK: 3})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}

	if fr.query != "token policy rewritten" {
		t.Fatalf("retriever query = %q, want rewritten query", fr.query)
	}
	if result.Query != "token policy" || result.RewrittenQuery != "token policy rewritten" {
		t.Fatalf("unexpected queries: %#v", result)
	}
	if len(result.Documents) != 1 {
		t.Fatalf("len(result.Documents) = %d, want 1", len(result.Documents))
	}
	if !strings.Contains(result.Context, "[1] OAuth2 token rotation") {
		t.Fatalf("context = %q, want assembled citation content", result.Context)
	}
	if len(result.Citations) != 1 || result.Citations[0].Title != "Auth Guide" {
		t.Fatalf("citations = %#v, want Auth Guide citation", result.Citations)
	}
}

func TestGraphExpandsFusesAndGeneratesAnswer(t *testing.T) {
	fr := &fakeRetriever{}
	events := make([]TraceEvent, 0)
	graph, err := New(&Config{
		Retriever: fr,
		QueryExpander: QueryExpandFunc(func(ctx context.Context, query string) ([]string, error) {
			return []string{"oauth refresh token", "token policy"}, nil
		}),
		AnswerGenerator: AnswerGenerateFunc(func(ctx context.Context, input *AnswerInput) (string, error) {
			if !strings.Contains(input.Context, "OAuth2 token rotation") {
				t.Fatalf("answer input context = %q, want retrieved context", input.Context)
			}
			return "Rotate OAuth2 tokens with a refresh window [1].", nil
		}),
		TraceObserver: TraceObserverFunc(func(event TraceEvent) {
			events = append(events, event)
		}),
		MaxQueries:     3,
		GenerateAnswer: true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := graph.Invoke(context.Background(), &Request{
		Query:             "token policy",
		AdditionalQueries: []string{"OAuth2 token rotation"},
		TopK:              5,
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}

	if len(fr.queries) != 3 {
		t.Fatalf("retriever queries = %#v, want 3 planned queries", fr.queries)
	}
	if len(result.Retrievals) != 3 {
		t.Fatalf("len(result.Retrievals) = %d, want 3", len(result.Retrievals))
	}
	if len(result.Documents) != 2 {
		t.Fatalf("len(result.Documents) = %d, want deduped documents", len(result.Documents))
	}
	if result.Documents[0].MetaData[MetaHitCount] != 3 {
		t.Fatalf("hit count = %v, want 3", result.Documents[0].MetaData[MetaHitCount])
	}
	if result.Answer == "" {
		t.Fatalf("answer is empty")
	}
	if !containsQuery(result.Queries, "OAuth2 token rotation") {
		t.Fatalf("planned queries = %#v, want additional query", result.Queries)
	}
	if result.Trace == nil || len(result.Trace.Events) == 0 {
		t.Fatalf("trace missing: %#v", result.Trace)
	}
	if len(events) != len(result.Trace.Events) {
		t.Fatalf("observed events = %d, trace events = %d", len(events), len(result.Trace.Events))
	}
	if result.Evaluation == nil {
		t.Fatalf("evaluation missing")
	}
	if result.Evaluation.QueryCount != 3 || !result.Evaluation.HasCitationsInAnswer {
		t.Fatalf("evaluation = %#v, want query count and answer citations", result.Evaluation)
	}
	if result.Verification == nil || !result.Verification.Passed {
		t.Fatalf("verification = %#v, want passed", result.Verification)
	}
}

func TestGraphExpandsNeighborsCompressesAndFallsBack(t *testing.T) {
	fr := &emptyRetriever{}
	graph, err := New(&Config{
		Retriever: fr,
		Fallback: FallbackFunc(func(ctx context.Context, input *FallbackInput) (*FallbackOutput, error) {
			if input.Reason != FallbackReasonNoDocuments {
				t.Fatalf("fallback reason = %q, want no_documents", input.Reason)
			}
			return &FallbackOutput{
				Answer:  "No matching context was found.",
				Applied: true,
			}, nil
		}),
		GenerateAnswer: true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := graph.Invoke(context.Background(), &Request{Query: "missing topic"})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if result.Answer != "No matching context was found." {
		t.Fatalf("answer = %q, want fallback answer", result.Answer)
	}
	if result.Verification == nil || !result.Verification.Passed {
		t.Fatalf("verification = %#v, want fallback answer to pass without required citations", result.Verification)
	}
}

func TestNeighborExpanderAndKeywordCompressor(t *testing.T) {
	main := &schema.Document{
		ID:      "main",
		Content: "OAuth2 rotation details. Irrelevant filler sentence that should be removed.",
		MetaData: map[string]any{
			enhance.MetaChunkPrevID: "prev",
			enhance.MetaChunkNextID: "next",
		},
	}
	expander := &StaticNeighborExpander{
		Documents: map[string]*schema.Document{
			"prev": {ID: "prev", Content: "Previous context."},
			"next": {ID: "next", Content: "Next context."},
		},
	}
	docs, err := expander.Expand(context.Background(), []*schema.Document{main})
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}
	if len(docs) != 3 {
		t.Fatalf("len(docs) = %d, want main plus neighbors", len(docs))
	}

	compressor := NewKeywordContextCompressor(&KeywordContextCompressorConfig{MaxCharsPerDocument: 24})
	compressed, err := compressor.Compress(context.Background(), "OAuth2", docs)
	if err != nil {
		t.Fatalf("Compress() error = %v", err)
	}
	if !strings.Contains(compressed[0].Content, "OAuth2") {
		t.Fatalf("compressed content = %q, want keyword sentence", compressed[0].Content)
	}
	if strings.Contains(compressed[0].Content, "Irrelevant") {
		t.Fatalf("compressed content = %q, want irrelevant sentence removed", compressed[0].Content)
	}
}

func containsQuery(queries []string, query string) bool {
	for _, item := range queries {
		if item == query {
			return true
		}
	}
	return false
}

type emptyRetriever struct{}

func (emptyRetriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	return nil, nil
}
