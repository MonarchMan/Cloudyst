package subagent_test

import (
	"context"
	"testing"

	"ai/internal/pkg/eino/agent/router"
	"ai/internal/pkg/eino/agent/subagent"
)

func TestTargetDelegationPolicyDelegatesWebSearch(t *testing.T) {
	decision, err := subagent.NewTargetDelegationPolicy().Decide(context.Background(), &subagent.DelegationInput{
		Decision: &router.RouteDecision{
			Target:     router.TargetWebSearch,
			Confidence: 0.75,
		},
		AvailableAgents: []string{"web_agent"},
	})
	if err != nil {
		t.Fatalf("decide failed: %v", err)
	}
	if !decision.Delegate {
		t.Fatalf("delegate = false, want true: %#v", decision)
	}
	if decision.AgentName != "web_agent" || decision.Reason != subagent.ReasonTargetMatch {
		t.Fatalf("decision = %#v, want web target match", decision)
	}
	if decision.Confidence != 0.75 {
		t.Fatalf("confidence = %v, want 0.75", decision.Confidence)
	}
}

func TestTargetDelegationPolicySkipsUnavailableAgent(t *testing.T) {
	decision, err := subagent.NewTargetDelegationPolicy().Decide(context.Background(), &subagent.DelegationInput{
		Decision: &router.RouteDecision{Target: router.TargetRAG},
		AvailableAgents: []string{
			"web_agent",
		},
	})
	if err != nil {
		t.Fatalf("decide failed: %v", err)
	}
	if decision.Delegate {
		t.Fatalf("delegate = true, want false: %#v", decision)
	}
	if decision.AgentName != "rag_agent" || decision.Reason != subagent.ReasonAgentUnavailable {
		t.Fatalf("decision = %#v, want unavailable rag_agent", decision)
	}
}

func TestTargetDelegationPolicySkipsLLM(t *testing.T) {
	decision, err := subagent.NewTargetDelegationPolicy().Decide(context.Background(), &subagent.DelegationInput{
		Decision: &router.RouteDecision{Target: router.TargetLLM},
		AvailableAgents: []string{
			"web_agent",
			"rag_agent",
		},
	})
	if err != nil {
		t.Fatalf("decide failed: %v", err)
	}
	if decision.Delegate {
		t.Fatalf("delegate = true, want false: %#v", decision)
	}
	if decision.AgentName != "" || decision.Reason != subagent.ReasonNoDelegation {
		t.Fatalf("decision = %#v, want no delegation", decision)
	}
}

func TestExplicitDelegationPolicyUsesTaskMetadata(t *testing.T) {
	policy := subagent.ExplicitDelegationPolicy{}
	decision, err := policy.Decide(context.Background(), &subagent.DelegationInput{
		Task: &subagent.Task{
			Metadata: map[string]any{"subagent": "file_agent"},
		},
		AvailableAgents: []string{"file_agent"},
	})
	if err != nil {
		t.Fatalf("decide failed: %v", err)
	}
	if !decision.Delegate {
		t.Fatalf("delegate = false, want true: %#v", decision)
	}
	if decision.AgentName != "file_agent" || decision.Reason != subagent.ReasonExplicitAgent {
		t.Fatalf("decision = %#v, want explicit file_agent", decision)
	}
}
