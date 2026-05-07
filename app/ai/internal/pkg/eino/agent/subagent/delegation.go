package subagent

import (
	"context"

	"ai/internal/pkg/eino/agent/router"
)

const (
	ReasonNoDelegation     = "no_delegation"
	ReasonTargetMatch      = "target_match"
	ReasonAgentUnavailable = "agent_unavailable"
	ReasonExplicitAgent    = "explicit_agent"
)

type DelegationInput struct {
	Task            *Task
	Decision        *router.RouteDecision
	AvailableAgents []string
	Metadata        map[string]any
}

type DelegationDecision struct {
	Delegate   bool
	AgentName  string
	Reason     string
	Confidence float64
	Metadata   map[string]any
}

type DelegationPolicy interface {
	Decide(ctx context.Context, input *DelegationInput) (*DelegationDecision, error)
}

type DelegationPolicyFunc func(ctx context.Context, input *DelegationInput) (*DelegationDecision, error)

func (f DelegationPolicyFunc) Decide(ctx context.Context, input *DelegationInput) (*DelegationDecision, error) {
	return f(ctx, input)
}

type NoDelegationPolicy struct {
	Reason string
}

func (p NoDelegationPolicy) Decide(ctx context.Context, input *DelegationInput) (*DelegationDecision, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reason := p.Reason
	if reason == "" {
		reason = ReasonNoDelegation
	}
	return &DelegationDecision{Reason: reason}, nil
}

type TargetDelegationPolicy struct {
	TargetAgents map[router.Target]string
}

func NewTargetDelegationPolicy() TargetDelegationPolicy {
	return TargetDelegationPolicy{TargetAgents: DefaultTargetAgents()}
}

func DefaultTargetAgents() map[router.Target]string {
	return map[router.Target]string{
		router.TargetRAG:       "rag_agent",
		router.TargetWebSearch: "web_agent",
		router.TargetFile:      "file_agent",
		router.TargetMCP:       "tool_agent",
	}
}

func (p TargetDelegationPolicy) Decide(ctx context.Context, input *DelegationInput) (*DelegationDecision, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input == nil || input.Decision == nil {
		return &DelegationDecision{Reason: ReasonNoDelegation}, nil
	}
	agents := p.TargetAgents
	if len(agents) == 0 {
		agents = DefaultTargetAgents()
	}
	agentName := agents[input.Decision.Target]
	if agentName == "" {
		return &DelegationDecision{
			Reason:     ReasonNoDelegation,
			Confidence: input.Decision.Confidence,
		}, nil
	}
	if !isAgentAvailable(agentName, input.AvailableAgents) {
		return &DelegationDecision{
			AgentName:  agentName,
			Reason:     ReasonAgentUnavailable,
			Confidence: input.Decision.Confidence,
		}, nil
	}
	return &DelegationDecision{
		Delegate:   true,
		AgentName:  agentName,
		Reason:     ReasonTargetMatch,
		Confidence: input.Decision.Confidence,
	}, nil
}

type ExplicitDelegationPolicy struct {
	MetadataKeys []string
}

func (p ExplicitDelegationPolicy) Decide(ctx context.Context, input *DelegationInput) (*DelegationDecision, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	agentName := explicitAgentName(input, p.MetadataKeys)
	if agentName == "" {
		return &DelegationDecision{Reason: ReasonNoDelegation}, nil
	}
	if !isAgentAvailable(agentName, input.AvailableAgents) {
		return &DelegationDecision{
			AgentName: agentName,
			Reason:    ReasonAgentUnavailable,
		}, nil
	}
	return &DelegationDecision{
		Delegate:   true,
		AgentName:  agentName,
		Reason:     ReasonExplicitAgent,
		Confidence: 1,
	}, nil
}

func explicitAgentName(input *DelegationInput, keys []string) string {
	if input == nil {
		return ""
	}
	if len(keys) == 0 {
		keys = []string{"agent", "subagent"}
	}
	for _, metadata := range []map[string]any{input.Metadata, taskMetadata(input.Task)} {
		for _, key := range keys {
			if value, ok := metadata[key].(string); ok && value != "" {
				return value
			}
		}
	}
	return ""
}

func taskMetadata(task *Task) map[string]any {
	if task == nil {
		return nil
	}
	return task.Metadata
}

func isAgentAvailable(agentName string, availableAgents []string) bool {
	if agentName == "" {
		return false
	}
	if len(availableAgents) == 0 {
		return true
	}
	for _, available := range availableAgents {
		if available == agentName {
			return true
		}
	}
	return false
}
