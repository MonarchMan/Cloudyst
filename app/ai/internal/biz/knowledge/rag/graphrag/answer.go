package graphrag

import (
	"context"
	"fmt"
	"strings"

	emodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type AnswerInput struct {
	Query          string
	RewrittenQuery string
	Queries        []string
	Context        string
	Citations      []Citation
	Metadata       map[string]any
}

type AnswerGenerator interface {
	Generate(ctx context.Context, input *AnswerInput) (string, error)
}

type AnswerGenerateFunc func(ctx context.Context, input *AnswerInput) (string, error)

func (f AnswerGenerateFunc) Generate(ctx context.Context, input *AnswerInput) (string, error) {
	return f(ctx, input)
}

type ChatModelAnswerGeneratorConfig struct {
	Model        emodel.BaseChatModel
	SystemPrompt string
}

type ChatModelAnswerGenerator struct {
	conf ChatModelAnswerGeneratorConfig
}

func NewChatModelAnswerGenerator(conf *ChatModelAnswerGeneratorConfig) (*ChatModelAnswerGenerator, error) {
	if conf == nil || conf.Model == nil {
		return nil, fmt.Errorf("chat model answer generator requires model")
	}
	cfg := *conf
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = "Answer the question using only the provided context. Cite sources with bracketed numbers like [1]. If the context is insufficient, say so."
	}
	return &ChatModelAnswerGenerator{conf: cfg}, nil
}

func (g *ChatModelAnswerGenerator) Generate(ctx context.Context, input *AnswerInput) (string, error) {
	if input == nil {
		return "", fmt.Errorf("answer input is nil")
	}
	msg, err := g.conf.Model.Generate(ctx, []*schema.Message{
		schema.SystemMessage(g.conf.SystemPrompt),
		schema.UserMessage(formatAnswerPrompt(input)),
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(msg.Content), nil
}

func formatAnswerPrompt(input *AnswerInput) string {
	var b strings.Builder
	b.WriteString("Question:\n")
	b.WriteString(input.Query)
	if input.RewrittenQuery != "" && input.RewrittenQuery != input.Query {
		b.WriteString("\n\nRewritten query:\n")
		b.WriteString(input.RewrittenQuery)
	}
	b.WriteString("\n\nContext:\n")
	b.WriteString(input.Context)
	return b.String()
}
