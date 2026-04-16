package rerank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

type CohereRerankerConfig struct {
	APIKey         string
	Model          string // "rerank-multilingual-v3.0"
	TopN           int
	ScoreThreshold float64
}

type CohereReranker struct {
	cfg    *CohereRerankerConfig
	client *http.Client
}

func NewCohereReranker(_ context.Context, cfg *CohereRerankerConfig) (document.Transformer, error) {
	if cfg.TopN == 0 {
		cfg.TopN = 5
	}
	return &CohereReranker{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// Transform 实现 document.Transformer 接口
func (r *CohereReranker) Transform(
	ctx context.Context,
	docs []*schema.Document,
	opts ...document.TransformerOption,
) ([]*schema.Document, error) {
	// 解析 Options
	options := &Options{
		TopN:           r.cfg.TopN,
		ScoreThreshold: r.cfg.ScoreThreshold,
	}
	options = document.GetTransformerImplSpecificOptions(options, opts...)

	if options.Query == "" {
		return nil, fmt.Errorf("reranker: query is required, use WithQuery()")
	}
	if len(docs) == 0 {
		return nil, nil
	}

	// 提取文档文本
	texts := make([]string, len(docs))
	for i, doc := range docs {
		texts[i] = doc.Content
	}

	// 调用 Cohere Rerank API
	scores, err := r.callCohereAPI(ctx, options.Query, texts)
	if err != nil {
		return nil, fmt.Errorf("cohere rerank: %w", err)
	}

	// 按分数排序，过滤阈值，截取 TopN
	return filterAndSort(docs, scores, options), nil
}

// callCohereAPI 调用 Cohere API 获取相关性分数
func (r *CohereReranker) callCohereAPI(ctx context.Context, query string, texts []string) ([]float64, error) {
	body := map[string]any{
		"model":     r.cfg.Model,
		"query":     query,
		"documents": texts,
		"top_n":     len(texts), // 全部返回，截断由我们控制
	}

	data, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST",
		"https://api.cohere.com/v2/rerank",
		bytes.NewReader(data),
	)
	req.Header.Set("Authorization", "Bearer "+r.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// 将分数映射回原始索引顺序
	scores := make([]float64, len(texts))
	for _, r := range result.Results {
		scores[r.Index] = r.RelevanceScore
	}
	return scores, nil
}

func filterAndSort(docs []*schema.Document, scores []float64, opts *Options) []*schema.Document {
	type scored struct {
		doc   *schema.Document
		score float64
	}

	var items []scored
	for i, doc := range docs {
		if scores[i] >= opts.ScoreThreshold {
			// 将 score 写入 MetaData，方便上层使用
			if doc.MetaData == nil {
				doc.MetaData = make(map[string]any)
			}
			doc.WithScore(scores[i])
			items = append(items, scored{doc: doc, score: scores[i]})
		}
	}

	// 按分数降序
	sort.Slice(items, func(i, j int) bool {
		return items[i].score > items[j].score
	})

	// 截取 TopN
	topN := opts.TopN
	if topN > len(items) {
		topN = len(items)
	}

	result := make([]*schema.Document, topN)
	for i := range result {
		result[i] = items[i].doc
	}
	return result
}
