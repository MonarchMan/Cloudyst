package safety

import (
	"context"

	"ai/internal/pkg/eino/agent/budget"
)

const (
	ReasonAllowed                 = "allowed"
	ReasonInvalidInput            = "invalid_input"
	ReasonMissingAgent            = "missing_agent"
	ReasonAgentNotAllowed         = "agent_not_allowed"
	ReasonToolNotAllowed          = "tool_not_allowed"
	ReasonDelegationDepthExceeded = "delegation_depth_exceeded"
	ReasonContextItemsExceeded    = "context_items_exceeded"
	ReasonBudgetExceeded          = "budget_exceeded"
)

type GuardInput struct {
	AgentName       string
	ToolNames       []string
	DelegationDepth int
	ContextItems    int
	Budget          *budget.Tracker
	Metadata        map[string]any
}

type GuardResult struct {
	Allowed    bool
	Reason     string
	Violations []Violation
	Metadata   map[string]any
}

type Violation struct {
	Code     string
	Message  string
	Value    int
	Limit    int
	Subject  string
	Metadata map[string]any
}

type GuardPolicy interface {
	Guard(ctx context.Context, input *GuardInput) (*GuardResult, error)
}

type GuardPolicyFunc func(ctx context.Context, input *GuardInput) (*GuardResult, error)

func (f GuardPolicyFunc) Guard(ctx context.Context, input *GuardInput) (*GuardResult, error) {
	return f(ctx, input)
}

type Limits struct {
	AllowedAgents      []string
	AllowedTools       []string
	RequireAgent       bool
	MaxDelegationDepth int
	MaxContextItems    int
}

type StaticGuardPolicy struct {
	Limits Limits
}

func NewStaticGuardPolicy(limits Limits) StaticGuardPolicy {
	return StaticGuardPolicy{Limits: limits}
}

func (p StaticGuardPolicy) Guard(ctx context.Context, input *GuardInput) (*GuardResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input == nil {
		return denied(ReasonInvalidInput, Violation{Code: ReasonInvalidInput, Message: "guard input is nil"}), nil
	}

	var violations []Violation
	if p.Limits.RequireAgent && input.AgentName == "" {
		violations = append(violations, Violation{
			Code:    ReasonMissingAgent,
			Message: "agent name is required",
		})
	}
	if len(p.Limits.AllowedAgents) > 0 && input.AgentName != "" && !contains(p.Limits.AllowedAgents, input.AgentName) {
		violations = append(violations, Violation{
			Code:    ReasonAgentNotAllowed,
			Message: "agent is not allowed",
			Subject: input.AgentName,
		})
	}
	for _, toolName := range input.ToolNames {
		if toolName == "" {
			continue
		}
		if len(p.Limits.AllowedTools) > 0 && !contains(p.Limits.AllowedTools, toolName) {
			violations = append(violations, Violation{
				Code:    ReasonToolNotAllowed,
				Message: "tool is not allowed",
				Subject: toolName,
			})
		}
	}
	if p.Limits.MaxDelegationDepth > 0 && input.DelegationDepth > p.Limits.MaxDelegationDepth {
		violations = append(violations, Violation{
			Code:    ReasonDelegationDepthExceeded,
			Message: "delegation depth exceeded",
			Value:   input.DelegationDepth,
			Limit:   p.Limits.MaxDelegationDepth,
		})
	}
	if p.Limits.MaxContextItems > 0 && input.ContextItems > p.Limits.MaxContextItems {
		violations = append(violations, Violation{
			Code:    ReasonContextItemsExceeded,
			Message: "context items exceeded",
			Value:   input.ContextItems,
			Limit:   p.Limits.MaxContextItems,
		})
	}
	if input.Budget != nil && input.ContextItems > 0 {
		if err := input.Budget.Check(ctx, budget.ResourceContextItem, input.ContextItems); err != nil {
			violations = append(violations, Violation{
				Code:    ReasonBudgetExceeded,
				Message: err.Error(),
				Value:   input.ContextItems,
			})
		}
	}

	if len(violations) > 0 {
		return denied(violations[0].Code, violations...), nil
	}
	return &GuardResult{
		Allowed:  true,
		Reason:   ReasonAllowed,
		Metadata: input.Metadata,
	}, nil
}

func denied(reason string, violations ...Violation) *GuardResult {
	return &GuardResult{
		Allowed:    false,
		Reason:     reason,
		Violations: violations,
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
