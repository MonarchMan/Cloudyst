package model

import (
	"context"
	"net/http"
	"time"

	"github.com/cloudwego/eino-ext/components/embedding/ollama"
	"github.com/cloudwego/eino-ext/libs/acl/openai"
	"github.com/cloudwego/eino/components/embedding"
)

func OllamaEmbedder() (embedding.Embedder, error) {
	embedder, err := ollama.NewEmbedder(context.Background(), &ollama.EmbeddingConfig{
		Timeout: 30 * time.Second,
		Model:   "qwen3-embedding:8b",
	})
	if err != nil {
		return nil, err
	}
	return embedder, nil
}

// OllamaEmbedderForOpenAI
// there is a bug in openai api, where "clientConf.HTTPClient = config.HTTPClient",
// when config.HTTPClient is nil, clientConf.HTTPClient will be nil with empty value and pointer, which has a bug
func OllamaEmbedderForOpenAI(dimensions int) (embedding.Embedder, error) {
	embedder, err := openai.NewEmbeddingClient(context.Background(), &openai.EmbeddingConfig{
		Model:      "qwen3-embedding:8b",
		BaseURL:    "http://localhost:11434/v1",
		APIKey:     " ",
		Dimensions: &dimensions,
		HTTPClient: http.DefaultClient,
	})
	if err != nil {
		return nil, err
	}
	return embedder, nil
}

func OpenAIEmbedder() (embedding.Embedder, error) {
	embedder, err := openai.NewEmbeddingClient(context.Background(), &openai.EmbeddingConfig{})
	if err != nil {
		return nil, err
	}
	return embedder, nil
}
