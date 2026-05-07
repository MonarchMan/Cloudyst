package enhance

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestDefaultPipelineNormalizesAndEnriches(t *testing.T) {
	pipeline := NewDefaultPipeline()
	docs, err := pipeline.Transform(context.Background(), []*schema.Document{
		{
			ID:      "doc-1",
			Content: "# API Guide\r\n\r\n\r\nThis HTTP API uses OAuth2 tokens. CPUQuota handles S3Upload jobs.\n",
			MetaData: map[string]any{
				"source": "unit-test",
			},
		},
	})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("len(docs) = %d, want 1", len(docs))
	}

	doc := docs[0]
	if strings.Contains(doc.Content, "\r") {
		t.Fatalf("content still contains carriage return: %q", doc.Content)
	}
	if strings.Contains(doc.Content, "\n\n\n") {
		t.Fatalf("content still contains collapsed blank lines: %q", doc.Content)
	}
	if got := doc.MetaData[MetaTitle]; got != "API Guide" {
		t.Fatalf("title = %v, want API Guide", got)
	}
	if got := doc.MetaData["source"]; got != "unit-test" {
		t.Fatalf("source metadata = %v, want unit-test", got)
	}
	if _, ok := doc.MetaData[MetaSummary].(string); !ok {
		t.Fatalf("summary metadata missing or not string")
	}
	terms, ok := doc.MetaData[MetaTerms].([]string)
	if !ok || !containsAny(terms, "OAuth2", "S3Upload", "CPUQuota") {
		t.Fatalf("terms = %#v, want one technical term", doc.MetaData[MetaTerms])
	}
	if _, ok := doc.MetaData[MetaOriginalChars].(int); !ok {
		t.Fatalf("original char count metadata missing")
	}
}

func TestDeduperAndTrimmer(t *testing.T) {
	trimmer, err := NewTrimmer(&TrimmerConfig{
		MaxChars:  16,
		HeadChars: 6,
		TailChars: 5,
	})
	if err != nil {
		t.Fatalf("NewTrimmer() error = %v", err)
	}

	pipeline := NewPipeline(NewDeduper(&DeduperConfig{PreferID: false}), trimmer)
	docs, err := pipeline.Transform(context.Background(), []*schema.Document{
		{ID: "a", Content: "same content value"},
		{ID: "b", Content: "same   content   value"},
		{ID: "c", Content: "0123456789abcdefghijklmnopqrstuvwxyz"},
	})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("len(docs) = %d, want 2 after content dedupe", len(docs))
	}
	if got := docs[1].MetaData[MetaTrimmed]; got != true {
		t.Fatalf("trimmed metadata = %v, want true", got)
	}
	if !strings.Contains(docs[1].Content, "...") {
		t.Fatalf("trimmed content = %q, want ellipsis", docs[1].Content)
	}
}

func TestQualityGateAndChunkPostProcessor(t *testing.T) {
	pipeline := NewPipeline(
		NewQualityGate(&QualityGateConfig{MinChars: 4}),
		NewChunkPostProcessor(nil),
	)
	docs, err := pipeline.Transform(context.Background(), []*schema.Document{
		{ID: "a", Content: "first chunk", MetaData: map[string]any{MetaTitle: "Section A"}},
		{ID: "b", Content: "second chunk"},
	})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if got := docs[0].MetaData[MetaQualityValid]; got != true {
		t.Fatalf("quality valid = %v, want true", got)
	}
	if got := docs[0].MetaData[MetaChunkNextID]; got != "b" {
		t.Fatalf("next id = %v, want b", got)
	}
	if got := docs[1].MetaData[MetaChunkPrevID]; got != "a" {
		t.Fatalf("prev id = %v, want a", got)
	}
	if got := docs[0].MetaData[MetaChunkSection]; got != "Section A" {
		t.Fatalf("section = %v, want Section A", got)
	}
}

func containsAny(values []string, needles ...string) bool {
	for _, value := range values {
		for _, needle := range needles {
			if value == needle {
				return true
			}
		}
	}
	return false
}
