package model

import (
	"context"
	"testing"
)

func TestOllamaEmbedder(t *testing.T) {
	embedder, err := OllamaEmbedder()
	if err != nil {
		t.Fatalf("getOllamaEmbedder: %v", err)
	}
	vectors, err := embedder.EmbedStrings(context.Background(), []string{
		"What is the capital of China?",
		"Explain gravity",
	})
	if err != nil {
		t.Fatalf("embedder.EmbedStrings: %v", err)
	}
	t.Logf("len of vectors: %d", len(vectors[0]))
}

func TestOllamaEmbedderForOpenAI(t *testing.T) {
	embedder, err := OllamaEmbedderForOpenAI(32)
	if err != nil {
		t.Fatalf("getOllamaEmbedderForOpenAI: %v", err)
	}
	vectors, err := embedder.EmbedStrings(context.Background(), []string{
		"What is the capital of China?",
		"Explain gravity",
	})
	if err != nil {
		t.Fatalf("embedder.EmbedStrings: %v", err)
	}
	t.Logf("len of vectors: %d", len(vectors[0]))
}
