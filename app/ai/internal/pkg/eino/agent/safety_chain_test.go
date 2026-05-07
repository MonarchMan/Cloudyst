package agent_test

import (
	"context"
	"testing"

	agentsecurity "ai/internal/biz/agent/security"
	"ai/internal/pkg/eino/agent/policy"
	"ai/internal/pkg/eino/agent/router"
	"ai/internal/pkg/eino/agent/runner"
	"ai/internal/pkg/eino/agent/safety"
)

func TestAgentSafetyChainAllowsToolInvocation(t *testing.T) {
	ctx := context.Background()
	security, err := agentsecurity.NewDirectResolver(&agentsecurity.ResolveInput{
		UserID: "user-1",
		Role:   "member",
		Scopes: []string{"tool:web_search"},
	}).ResolveSecurity(ctx, nil)
	if err != nil {
		t.Fatalf("resolve security failed: %v", err)
	}

	decision, err := router.NewRuleRouter().Route(ctx, &router.RouteInput{
		Query: "search latest release notes",
		Capabilities: []router.Capability{
			{Name: "web_search", Target: router.TargetWebSearch},
		},
	})
	if err != nil {
		t.Fatalf("route failed: %v", err)
	}

	selection, err := safety.NewPermissionToolSelector(
		policy.FirstToolPolicy{},
		safety.NewStaticToolPermissionPolicy([]safety.ToolRule{
			{
				ToolName:       "web_search",
				AllowedRoles:   []string{"member"},
				AllowedActions: []string{"read"},
				RequireUser:    true,
			},
		}),
	).SelectTools(ctx, &safety.PermissionToolSelectionInput{
		Decision:      decision,
		Security:      security,
		DefaultAction: "read",
	})
	if err != nil {
		t.Fatalf("select tools failed: %v", err)
	}
	if !selection.Allowed {
		t.Fatalf("selection = %#v, want allowed", selection)
	}

	guard, err := safety.NewStaticGuardPolicy(safety.Limits{
		AllowedAgents:      []string{"web_agent"},
		AllowedTools:       []string{"web_search"},
		RequireAgent:       true,
		MaxDelegationDepth: 1,
		MaxContextItems:    2,
	}).Guard(ctx, &safety.GuardInput{
		AgentName:       "web_agent",
		ToolNames:       selection.Tools,
		DelegationDepth: 1,
		ContextItems:    1,
	})
	if err != nil {
		t.Fatalf("guard failed: %v", err)
	}
	if !guard.Allowed {
		t.Fatalf("guard = %#v, want allowed", guard)
	}

	invoked := false
	output, err := runner.ToolInvokerFunc(func(ctx context.Context, toolName string, input *runner.ToolInput) (any, error) {
		invoked = true
		return callDemoTool(toolName)
	}).Invoke(ctx, first(selection.Tools), &runner.ToolInput{
		Query:    "search latest release notes",
		Decision: decision,
	})
	if err != nil {
		t.Fatalf("invoke failed: %v", err)
	}
	if !invoked {
		t.Fatal("tool was not invoked")
	}
	if output == "" {
		t.Fatal("tool output is empty")
	}

	t.Logf("safety chain target=%s tools=%v output=%q", decision.Target, selection.Tools, output)
}

func TestAgentSafetyChainDeniesToolInvocation(t *testing.T) {
	ctx := context.Background()
	security, err := agentsecurity.NewDirectResolver(&agentsecurity.ResolveInput{
		UserID: "user-1",
		Role:   "member",
		Scopes: []string{"tool:web_search"},
	}).ResolveSecurity(ctx, nil)
	if err != nil {
		t.Fatalf("resolve security failed: %v", err)
	}

	decision, err := router.NewRuleRouter().Route(ctx, &router.RouteInput{
		Query: "search latest release notes",
		Capabilities: []router.Capability{
			{Name: "web_search", Target: router.TargetWebSearch},
		},
	})
	if err != nil {
		t.Fatalf("route failed: %v", err)
	}

	selection, err := safety.NewPermissionToolSelector(
		policy.FirstToolPolicy{},
		safety.NewStaticToolPermissionPolicy([]safety.ToolRule{
			{
				ToolName:       "web_search",
				AllowedRoles:   []string{"admin"},
				AllowedActions: []string{"read"},
				RequireUser:    true,
			},
		}),
	).SelectTools(ctx, &safety.PermissionToolSelectionInput{
		Decision:      decision,
		Security:      security,
		DefaultAction: "read",
	})
	if err != nil {
		t.Fatalf("select tools failed: %v", err)
	}
	if selection.Allowed {
		t.Fatalf("selection = %#v, want denied", selection)
	}
	if selection.Reason != safety.ReasonNoAllowedTools {
		t.Fatalf("selection reason = %q, want no_allowed_tools", selection.Reason)
	}

	invoked := false
	invoker := runner.ToolInvokerFunc(func(ctx context.Context, toolName string, input *runner.ToolInput) (any, error) {
		invoked = true
		return callDemoTool(toolName)
	})
	if selection.Allowed {
		_, _ = invoker.Invoke(ctx, first(selection.Tools), &runner.ToolInput{Decision: decision})
	}
	if invoked {
		t.Fatal("tool was invoked after permission denial")
	}

	t.Logf("safety chain denied target=%s reason=%s denied=%d", decision.Target, selection.Reason, len(selection.Denied))
}
