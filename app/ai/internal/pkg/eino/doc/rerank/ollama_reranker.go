package rerank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

type OllamaRerankerConfig struct {
	BaseURL        string // "http://localhost:11434"
	Model          string // 支持 rerank 的模型，如 "bge-reranker-v2-m3"
	TopN           int
	ScoreThreshold float64
}

type OllamaReranker struct {
	cfg    *OllamaRerankerConfig
	client *http.Client
}

func NewOllamaReranker(cfg *OllamaRerankerConfig) (document.Transformer, error) {
	return &OllamaReranker{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (r *OllamaReranker) Transform(
	ctx context.Context,
	docs []*schema.Document,
	opts ...document.TransformerOption,
) ([]*schema.Document, error) {
	options := &Options{
		TopN:           r.cfg.TopN,
		ScoreThreshold: r.cfg.ScoreThreshold,
	}
	options = document.GetTransformerImplSpecificOptions(options, opts...)

	if options.Query == "" {
		return nil, fmt.Errorf("reranker: query is required")
	}

	// Ollama rerank API
	type rerankRequest struct {
		Model     string   `json:"model"`
		Query     string   `json:"query"`
		Documents []string `json:"documents"`
	}

	texts := make([]string, len(docs))
	for i, d := range docs {
		texts[i] = d.Content
	}

	body, _ := json.Marshal(rerankRequest{
		Model:     r.cfg.Model,
		Query:     options.Query,
		Documents: texts,
	})

	req, _ := http.NewRequestWithContext(ctx, "POST",
		r.cfg.BaseURL+"/api/rerank",
		bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama rerank: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	scores := make([]float64, len(docs))
	for _, r := range result.Results {
		scores[r.Index] = r.RelevanceScore
	}

	return filterAndSort(docs, scores, options), nil
}
