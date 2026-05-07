package policy

import (
	"context"
	"fmt"
)

type FallbackInput struct {
	ToolName string
	Attempts int
	Err      error
}

type FallbackPolicy interface {
	Fallback(ctx context.Context, input *FallbackInput) (any, bool, error)
}

type FallbackPolicyFunc func(ctx context.Context, input *FallbackInput) (any, bool, error)

func (f FallbackPolicyFunc) Fallback(ctx context.Context, input *FallbackInput) (any, bool, error) {
	return f(ctx, input)
}

type NoFallbackPolicy struct{}

func (p NoFallbackPolicy) Fallback(ctx context.Context, input *FallbackInput) (any, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	return nil, false, nil
}

type StaticFallbackPolicy struct {
	Result any
}

func (p StaticFallbackPolicy) Fallback(ctx context.Context, input *FallbackInput) (any, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	return p.Result, true, nil
}

type ErrorMessageFallbackPolicy struct {
	Prefix string
}

func (p ErrorMessageFallbackPolicy) Fallback(ctx context.Context, input *FallbackInput) (any, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	prefix := p.Prefix
	if prefix == "" {
		prefix = "tool unavailable"
	}
	if input == nil || input.Err == nil {
		return prefix, true, nil
	}
	return fmt.Sprintf("%s: %s", prefix, input.Err.Error()), true, nil
}
