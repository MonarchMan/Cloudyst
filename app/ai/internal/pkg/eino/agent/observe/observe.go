package observe

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
)

const DefaultSummaryLimit = 1200

type ToolResult struct {
	Source   string         `json:"source,omitempty"`
	Type     string         `json:"type,omitempty"`
	Content  string         `json:"content,omitempty"`
	Score    float64        `json:"score,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Error    string         `json:"error,omitempty"`
}

type Observation struct {
	Source   string         `json:"source,omitempty"`
	Type     string         `json:"type,omitempty"`
	Summary  string         `json:"summary,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Error    string         `json:"error,omitempty"`
}

type Normalizer interface {
	Normalize(ctx context.Context, source string, result any, err error) (*ToolResult, error)
}

type Observer interface {
	Summarize(ctx context.Context, result *ToolResult) (*Observation, error)
}

type NormalizerFunc func(ctx context.Context, source string, result any, err error) (*ToolResult, error)

func (f NormalizerFunc) Normalize(ctx context.Context, source string, result any, err error) (*ToolResult, error) {
	return f(ctx, source, result, err)
}

type ObserverFunc func(ctx context.Context, result *ToolResult) (*Observation, error)

func (f ObserverFunc) Summarize(ctx context.Context, result *ToolResult) (*Observation, error) {
	return f(ctx, result)
}

type DefaultNormalizer struct{}

func NewDefaultNormalizer() *DefaultNormalizer {
	return &DefaultNormalizer{}
}

func (n *DefaultNormalizer) Normalize(ctx context.Context, source string, result any, err error) (*ToolResult, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}

	normalized := &ToolResult{Source: source, Metadata: map[string]any{}}
	if err != nil {
		normalized.Type = "error"
		normalized.Error = err.Error()
		return normalized, nil
	}

	switch v := result.(type) {
	case nil:
		normalized.Type = "empty"
	case *ToolResult:
		if v == nil {
			normalized.Type = "empty"
			return normalized, nil
		}
		cp := *v
		if cp.Source == "" {
			cp.Source = source
		}
		return &cp, nil
	case ToolResult:
		if v.Source == "" {
			v.Source = source
		}
		return &v, nil
	case string:
		normalized.Type = "text"
		normalized.Content = v
	case []byte:
		normalized.Type = "text"
		normalized.Content = string(v)
	case *schema.ToolResult:
		normalized.Type = "eino_tool_result"
		normalized.Content = contentFromEinoToolResult(v)
	default:
		normalized.Type = "json"
		bs, marshalErr := json.Marshal(v)
		if marshalErr != nil {
			return nil, fmt.Errorf("normalize tool result: %w", marshalErr)
		}
		normalized.Content = string(bs)
	}

	return normalized, nil
}

type SummaryObserver struct {
	Limit int
}

func NewSummaryObserver(limit int) *SummaryObserver {
	if limit <= 0 {
		limit = DefaultSummaryLimit
	}
	return &SummaryObserver{Limit: limit}
}

func (o *SummaryObserver) Summarize(ctx context.Context, result *ToolResult) (*Observation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		return &Observation{Type: "empty", Error: "nil_tool_result"}, nil
	}

	obs := &Observation{
		Source:   result.Source,
		Type:     result.Type,
		Metadata: result.Metadata,
		Error:    result.Error,
	}
	if result.Error != "" {
		obs.Summary = result.Error
		return obs, nil
	}
	obs.Summary = trimSpace(result.Content, o.Limit)
	return obs, nil
}

func contentFromEinoToolResult(result *schema.ToolResult) string {
	if result == nil {
		return ""
	}
	parts := make([]string, 0, len(result.Parts))
	for _, part := range result.Parts {
		if part.Text != "" {
			parts = append(parts, part.Text)
			continue
		}
		parts = append(parts, string(part.Type))
	}
	return strings.Join(parts, "\n")
}

func trimSpace(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	if limit <= 0 || len(s) <= limit {
		return s
	}
	if limit <= 3 {
		return s[:limit]
	}
	return s[:limit-3] + "..."
}
