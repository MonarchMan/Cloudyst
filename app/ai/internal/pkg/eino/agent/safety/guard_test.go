package safety_test

import (
	"context"
	"testing"

	"ai/internal/pkg/eino/agent/budget"
	"ai/internal/pkg/eino/agent/safety"
)

func TestStaticGuardPolicyAllowsBoundedCall(t *testing.T) {
	policy := safety.NewStaticGuardPolicy(safety.Limits{
		AllowedAgents:      []string{"web_agent"},
		AllowedTools:       []string{"web_search"},
		RequireAgent:       true,
		MaxDelegationDepth: 1,
		MaxContextItems:    3,
	})

	result, err := policy.Guard(context.Background(), &safety.GuardInput{
		AgentName:       "web_agent",
		ToolNames:       []string{"web_search"},
		DelegationDepth: 1,
		ContextItems:    2,
	})
	if err != nil {
		t.Fatalf("guard failed: %v", err)
	}
	if !result.Allowed {
		t.Fatalf("allowed = false, want true: %#v", result)
	}
	if result.Reason != safety.ReasonAllowed {
		t.Fatalf("reason = %q, want allowed", result.Reason)
	}
}

func TestStaticGuardPolicyDeniesDisallowedAgent(t *testing.T) {
	policy := safety.NewStaticGuardPolicy(safety.Limits{
		AllowedAgents: []string{"rag_agent"},
	})

	result, err := policy.Guard(context.Background(), &safety.GuardInput{
		AgentName: "web_agent",
	})
	if err != nil {
		t.Fatalf("guard failed: %v", err)
	}
	if result.Allowed {
		t.Fatalf("allowed = true, want false: %#v", result)
	}
	if result.Reason != safety.ReasonAgentNotAllowed {
		t.Fatalf("reason = %q, want agent_not_allowed", result.Reason)
	}
	if len(result.Violations) != 1 || result.Violations[0].Subject != "web_agent" {
		t.Fatalf("violations = %#v, want web_agent violation", result.Violations)
	}
}

func TestStaticGuardPolicyDeniesDepthAndToolViolations(t *testing.T) {
	policy := safety.NewStaticGuardPolicy(safety.Limits{
		AllowedTools:       []string{"knowledge_retrieval"},
		MaxDelegationDepth: 1,
	})

	result, err := policy.Guard(context.Background(), &safety.GuardInput{
		ToolNames:       []string{"web_search"},
		DelegationDepth: 2,
	})
	if err != nil {
		t.Fatalf("guard failed: %v", err)
	}
	if result.Allowed {
		t.Fatalf("allowed = true, want false: %#v", result)
	}
	if result.Reason != safety.ReasonToolNotAllowed {
		t.Fatalf("reason = %q, want first violation tool_not_allowed", result.Reason)
	}
	if len(result.Violations) != 2 {
		t.Fatalf("violations = %d, want 2", len(result.Violations))
	}
}

func TestStaticGuardPolicyChecksBudget(t *testing.T) {
	tracker := budget.NewTracker(budget.Limits{MaxContextItems: 1})
	policy := safety.NewStaticGuardPolicy(safety.Limits{})

	result, err := policy.Guard(context.Background(), &safety.GuardInput{
		Budget:       tracker,
		ContextItems: 2,
	})
	if err != nil {
		t.Fatalf("guard failed: %v", err)
	}
	if result.Allowed {
		t.Fatalf("allowed = true, want false: %#v", result)
	}
	if result.Reason != safety.ReasonBudgetExceeded {
		t.Fatalf("reason = %q, want budget_exceeded", result.Reason)
	}
}
