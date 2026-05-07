package graphrag

import (
	"context"
	"fmt"
	"strings"

	emodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type QueryRewriter interface {
	Rewrite(ctx context.Context, query string) (string, error)
}

type QueryExpander interface {
	Expand(ctx context.Context, query string) ([]string, error)
}

type QueryRewriteFunc func(ctx context.Context, query string) (string, error)

func (f QueryRewriteFunc) Rewrite(ctx context.Context, query string) (string, error) {
	return f(ctx, query)
}

type NoopQueryRewriter struct{}

func (NoopQueryRewriter) Rewrite(ctx context.Context, query string) (string, error) {
	return strings.TrimSpace(query), nil
}

type NoopQueryExpander struct{}

func (NoopQueryExpander) Expand(ctx context.Context, query string) ([]string, error) {
	return nil, nil
}

type QueryExpandFunc func(ctx context.Context, query string) ([]string, error)

func (f QueryExpandFunc) Expand(ctx context.Context, query string) ([]string, error) {
	return f(ctx, query)
}

type ChatModelQueryRewriterConfig struct {
	Model        emodel.BaseChatModel
	SystemPrompt string
	MaxChars     int
}

type ChatModelQueryRewriter struct {
	conf ChatModelQueryRewriterConfig
}

func NewChatModelQueryRewriter(conf *ChatModelQueryRewriterConfig) (*ChatModelQueryRewriter, error) {
	if conf == nil || conf.Model == nil {
		return nil, fmt.Errorf("chat model query rewriter requires model")
	}
	cfg := *conf
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = "Rewrite the user query for knowledge-base retrieval. Return only the rewritten query."
	}
	if cfg.MaxChars <= 0 {
		cfg.MaxChars = 512
	}
	return &ChatModelQueryRewriter{conf: cfg}, nil
}

func (r *ChatModelQueryRewriter) Rewrite(ctx context.Context, query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", nil
	}
	msg, err := r.conf.Model.Generate(ctx, []*schema.Message{
		schema.SystemMessage(r.conf.SystemPrompt),
		schema.UserMessage(query),
	})
	if err != nil {
		return "", err
	}
	rewritten := strings.TrimSpace(msg.Content)
	if rewritten == "" {
		return query, nil
	}
	if len([]rune(rewritten)) > r.conf.MaxChars {
		rewritten = string([]rune(rewritten)[:r.conf.MaxChars])
	}
	return rewritten, nil
}

type ChatModelQueryExpanderConfig struct {
	Model        emodel.BaseChatModel
	SystemPrompt string
	MaxQueries   int
	MaxChars     int
}

type ChatModelQueryExpander struct {
	conf ChatModelQueryExpanderConfig
}

func NewChatModelQueryExpander(conf *ChatModelQueryExpanderConfig) (*ChatModelQueryExpander, error) {
	if conf == nil || conf.Model == nil {
		return nil, fmt.Errorf("chat model query expander requires model")
	}
	cfg := *conf
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = "Generate alternative search queries for knowledge-base retrieval. Return one query per line, without numbering."
	}
	if cfg.MaxQueries <= 0 {
		cfg.MaxQueries = 3
	}
	if cfg.MaxChars <= 0 {
		cfg.MaxChars = 512
	}
	return &ChatModelQueryExpander{conf: cfg}, nil
}

func (e *ChatModelQueryExpander) Expand(ctx context.Context, query string) ([]string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	msg, err := e.conf.Model.Generate(ctx, []*schema.Message{
		schema.SystemMessage(e.conf.SystemPrompt),
		schema.UserMessage(query),
	})
	if err != nil {
		return nil, err
	}
	return normalizeQueries(parseQueryLines(msg.Content), e.conf.MaxQueries, e.conf.MaxChars), nil
}

func normalizeQueries(queries []string, maxQueries int, maxChars int) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(queries))
	for _, query := range queries {
		query = cleanQueryLine(query)
		if query == "" {
			continue
		}
		if maxChars > 0 && len([]rune(query)) > maxChars {
			query = string([]rune(query)[:maxChars])
		}
		key := strings.ToLower(query)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, query)
		if maxQueries > 0 && len(out) >= maxQueries {
			break
		}
	}
	return out
}

func parseQueryLines(content string) []string {
	lines := strings.FieldsFunc(content, func(r rune) bool {
		return r == '\n' || r == ';'
	})
	if len(lines) <= 1 {
		lines = strings.Split(content, ",")
	}
	return lines
}

func cleanQueryLine(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimLeft(line, "-*0123456789.、)） \t")
	line = strings.Trim(line, "\"'` ")
	return strings.TrimSpace(line)
}
