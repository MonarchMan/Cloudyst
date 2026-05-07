package citation_test

import (
	"context"
	"strings"
	"testing"

	"ai/internal/pkg/eino/agent/citation"
	"ai/internal/pkg/eino/agent/memory"
	"ai/internal/pkg/eino/agent/observe"
)

func TestBuilderCreatesStableCitations(t *testing.T) {
	builder := citation.DefaultBuilder{MaxItems: 2, MaxSnippetChars: 18}
	sources := citation.SourcesFromObservations([]*observe.Observation{
		{Source: "web_search", Type: "text", Summary: "release notes mention normalized observations"},
		{Source: "knowledge", Type: "text", Summary: "internal document explains agent routing"},
	})

	first, err := builder.Build(context.Background(), sources)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	second, err := builder.Build(context.Background(), sources)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}

	if len(first) != 2 {
		t.Fatalf("citation count = %d, want 2", len(first))
	}
	if first[0].Index != 1 || first[1].Index != 2 {
		t.Fatalf("indexes = %d,%d want 1,2", first[0].Index, first[1].Index)
	}
	if first[0].ID == "" || first[0].ID != second[0].ID {
		t.Fatalf("unstable id: first=%q second=%q", first[0].ID, second[0].ID)
	}
	if first[0].Snippet != "release notes m..." {
		t.Fatalf("snippet = %q, want truncated snippet", first[0].Snippet)
	}
}

func TestBuilderSourcesAndFormatBlock(t *testing.T) {
	sources := citation.SourcesFromToolResults([]*observe.ToolResult{
		{Source: "web_search", Type: "error", Error: "tool temporarily unavailable"},
	})
	sources = append(sources, citation.SourcesFromMemories([]*memory.Item{
		{Type: memory.TypeLongTerm, Source: "profile", Content: "user prefers concise answers"},
	})...)

	citations, err := citation.NewDefaultBuilder().Build(context.Background(), sources)
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if len(citations) != 2 {
		t.Fatalf("citation count = %d, want 2", len(citations))
	}
	if citations[0].Snippet != "tool temporarily unavailable" || citations[0].Error == "" {
		t.Fatalf("error citation = %#v", citations[0])
	}

	block := citation.FormatBlock(citations)
	if !strings.Contains(block, "[1] source=web_search") {
		t.Fatalf("block missing first citation: %q", block)
	}
	if !strings.Contains(block, "[2] source=profile") {
		t.Fatalf("block missing second citation: %q", block)
	}
}
