package graphrag

import (
	"context"

	"github.com/cloudwego/eino/schema"
)

const (
	FallbackReasonNoDocuments  = "no_documents"
	FallbackReasonEmptyContext = "empty_context"
	FallbackReasonBadAnswer    = "bad_answer"
)

type FallbackInput struct {
	Query          string
	RewrittenQuery string
	Queries        []string
	Reason         string
	Documents      []*schema.Document
	Context        string
	Citations      []Citation
	Answer         string
	Metadata       map[string]any
}

type FallbackOutput struct {
	Documents []*schema.Document
	Context   string
	Answer    string
	Metadata  map[string]any
	Applied   bool
}

type FallbackHandler interface {
	Handle(ctx context.Context, input *FallbackInput) (*FallbackOutput, error)
}

type FallbackFunc func(ctx context.Context, input *FallbackInput) (*FallbackOutput, error)

func (f FallbackFunc) Handle(ctx context.Context, input *FallbackInput) (*FallbackOutput, error) {
	return f(ctx, input)
}

type NoopFallbackHandler struct{}

func (NoopFallbackHandler) Handle(ctx context.Context, input *FallbackInput) (*FallbackOutput, error) {
	return &FallbackOutput{}, nil
}
