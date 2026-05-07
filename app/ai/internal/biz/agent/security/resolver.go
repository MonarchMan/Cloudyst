package security

import (
	"context"
	"fmt"

	"ai/internal/pkg/eino/agent/runner"
	"ai/internal/pkg/eino/agent/safety"
)

type contextKey struct{}

type ResolveInput struct {
	UserID   string
	Role     string
	Scopes   []string
	Metadata map[string]any
}

func NewDirectResolver(input *ResolveInput) runner.SecurityResolverFunc {
	return func(ctx context.Context, _ *runner.Input) (*safety.SecurityContext, error) {
		return Resolve(ctx, input)
	}
}

func Resolve(ctx context.Context, input *ResolveInput) (*safety.SecurityContext, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, fmt.Errorf("security resolve input is nil")
	}
	return &safety.SecurityContext{
		UserID:   input.UserID,
		Role:     input.Role,
		Scopes:   append([]string(nil), input.Scopes...),
		Metadata: cloneMetadata(input.Metadata),
	}, nil
}

func NewContextResolver(fallback runner.SecurityResolver) runner.SecurityResolverFunc {
	return func(ctx context.Context, input *runner.Input) (*safety.SecurityContext, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if security := FromContext(ctx); security != nil {
			return Clone(security), nil
		}
		if input != nil && input.Security != nil {
			return Clone(input.Security), nil
		}
		if fallback != nil {
			return fallback.ResolveSecurity(ctx, input)
		}
		return nil, fmt.Errorf("security context not found")
	}
}

func WithContext(ctx context.Context, security *safety.SecurityContext) context.Context {
	return context.WithValue(ctx, contextKey{}, Clone(security))
}

func FromContext(ctx context.Context) *safety.SecurityContext {
	if ctx == nil {
		return nil
	}
	security, _ := ctx.Value(contextKey{}).(*safety.SecurityContext)
	return security
}

func Clone(security *safety.SecurityContext) *safety.SecurityContext {
	if security == nil {
		return nil
	}
	return &safety.SecurityContext{
		UserID:   security.UserID,
		Role:     security.Role,
		Scopes:   append([]string(nil), security.Scopes...),
		Metadata: cloneMetadata(security.Metadata),
	}
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}
