package policy

import (
	"context"
	"fmt"

	"ai/internal/pkg/eino/agent/router"
)

type ToolSelectionInput struct {
	Decision *router.RouteDecision
}

type ToolSelectionPolicy interface {
	SelectTools(ctx context.Context, input *ToolSelectionInput) ([]string, error)
}

type ToolSelectionPolicyFunc func(ctx context.Context, input *ToolSelectionInput) ([]string, error)

func (f ToolSelectionPolicyFunc) SelectTools(ctx context.Context, input *ToolSelectionInput) ([]string, error) {
	return f(ctx, input)
}

type FirstToolPolicy struct{}

func (p FirstToolPolicy) SelectTools(ctx context.Context, input *ToolSelectionInput) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input == nil || input.Decision == nil {
		return nil, nil
	}
	for _, tool := range input.Decision.Tools {
		if tool != "" {
			return []string{tool}, nil
		}
	}
	return nil, nil
}

type AllToolsPolicy struct {
	Max int
}

func (p AllToolsPolicy) SelectTools(ctx context.Context, input *ToolSelectionInput) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input == nil || input.Decision == nil {
		return nil, nil
	}
	tools := make([]string, 0, len(input.Decision.Tools))
	for _, tool := range input.Decision.Tools {
		if tool == "" {
			continue
		}
		tools = append(tools, tool)
		if p.Max > 0 && len(tools) >= p.Max {
			break
		}
	}
	return tools, nil
}

type RequiredToolPolicy struct {
	Name string
}

func (p RequiredToolPolicy) SelectTools(ctx context.Context, input *ToolSelectionInput) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.Name == "" {
		return nil, fmt.Errorf("required tool name is empty")
	}
	if input == nil || input.Decision == nil {
		return nil, fmt.Errorf("required tool %q not available", p.Name)
	}
	for _, tool := range input.Decision.Tools {
		if tool == p.Name {
			return []string{tool}, nil
		}
	}
	return nil, fmt.Errorf("required tool %q not available", p.Name)
}
