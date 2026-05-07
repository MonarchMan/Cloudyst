package ingestion

import (
	"ai/internal/biz/types"
	"ai/internal/pkg/eino/doc/enhance"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestPrepareChunkMetadata(t *testing.T) {
	docs := []*schema.Document{
		{ID: "a", Content: "alpha", MetaData: map[string]any{}},
		{ID: "b", Content: "beta", MetaData: map[string]any{}},
	}

	prepareChunkMetadata(docs)

	if got := docs[0].MetaData[enhance.MetaChunkIndex]; got != 0 {
		t.Fatalf("first chunk index = %v, want 0", got)
	}
	if got := docs[1].MetaData[enhance.MetaChunkPrevID]; got != "a" {
		t.Fatalf("second prev id = %v, want a", got)
	}
	if got := docs[0].MetaData[enhance.MetaChunkNextID]; got != "b" {
		t.Fatalf("first next id = %v, want b", got)
	}
	if got := docs[1].MetaData[enhance.MetaChunkStartOffset]; got != 5 {
		t.Fatalf("second start offset = %v, want 5", got)
	}
	if got := docs[1].MetaData[enhance.MetaChunkEndOffset]; got != 9 {
		t.Fatalf("second end offset = %v, want 9", got)
	}
}

func TestBuildDocumentIndexStats(t *testing.T) {
	info := &DocumentInfo{
		Name:          "guide.md",
		Type:          string(types.TextMarkdown),
		SplitStrategy: types.StrategyParagraph,
		MaxTokens:     256,
	}
	docs := []*schema.Document{
		{Content: "alpha", MetaData: map[string]any{enhance.MetaTitle: "Guide"}},
		{Content: "beta", MetaData: map[string]any{}},
	}

	stats := buildDocumentIndexStats(info, docs, 9, 3)

	if stats.ContentLen != 9 || stats.Tokens != 3 || stats.Chunks != 2 {
		t.Fatalf("stats = len:%d tokens:%d chunks:%d", stats.ContentLen, stats.Tokens, stats.Chunks)
	}
	if stats.ParseType != string(types.TextMarkdown) {
		t.Fatalf("parse type = %q, want markdown", stats.ParseType)
	}
	if stats.ContentHash == "" {
		t.Fatal("content hash is empty")
	}
	if got := stats.Metadata[enhance.MetaTitle]; got != "Guide" {
		t.Fatalf("title metadata = %v, want Guide", got)
	}
	if got := stats.Metadata[documentMetaSplitStrategy]; got != string(types.StrategyParagraph) {
		t.Fatalf("split strategy metadata = %v, want paragraph", got)
	}
}
