package runner_test

import (
	"context"
	"errors"
	"testing"

	"ai/internal/pkg/eino/agent/budget"
	"ai/internal/pkg/eino/agent/planner"
	"ai/internal/pkg/eino/agent/policy"
	"ai/internal/pkg/eino/agent/router"
	"ai/internal/pkg/eino/agent/runner"
	"ai/internal/pkg/eino/agent/safety"
)

func TestRunnerSkipsInvokerWhenRouteHasNoTool(t *testing.T) {
	called := false
	agent := runner.New(
		router.RouterFunc(func(ctx context.Context, input *router.RouteInput) (*router.RouteDecision, error) {
			return &router.RouteDecision{Target: router.TargetLLM, Reason: "unit_test"}, nil
		}),
		planner.NewSimplePlanner(),
		runner.ToolInvokerFunc(func(ctx context.Context, toolName string, input *runner.ToolInput) (any, error) {
			called = true
			return nil, nil
		}),
	)

	out, err := agent.Run(context.Background(), &runner.Input{Query: "hello"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if called {
		t.Fatal("invoker was called for a route without tools")
	}
	if out.ToolResult != nil {
		t.Fatalf("tool result = %#v, want nil", out.ToolResult)
	}
	if out.Observation != nil {
		t.Fatalf("observation = %#v, want nil", out.Observation)
	}
	if len(out.Trace.Events) != 2 {
		t.Fatalf("trace event count = %d, want 2", len(out.Trace.Events))
	}
}

func TestRunnerNormalizesToolError(t *testing.T) {
	toolErr := errors.New("tool temporarily unavailable")
	agent := runner.New(
		router.RouterFunc(func(ctx context.Context, input *router.RouteInput) (*router.RouteDecision, error) {
			return &router.RouteDecision{
				Target: router.TargetWebSearch,
				Tools:  []string{"web_search"},
				Reason: "unit_test",
			}, nil
		}),
		planner.NewSimplePlanner(),
		runner.ToolInvokerFunc(func(ctx context.Context, toolName string, input *runner.ToolInput) (any, error) {
			return "", toolErr
		}),
	)

	out, err := agent.Run(context.Background(), &runner.Input{Query: "search latest status"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if out.ToolResult == nil {
		t.Fatal("tool result is nil")
	}
	if out.ToolResult.Type != "error" {
		t.Fatalf("tool result type = %q, want error", out.ToolResult.Type)
	}
	if out.ToolResult.Error != toolErr.Error() {
		t.Fatalf("tool result error = %q, want %q", out.ToolResult.Error, toolErr.Error())
	}
	if out.Observation == nil {
		t.Fatal("observation is nil")
	}
	if out.Observation.Error != toolErr.Error() {
		t.Fatalf("observation error = %q, want %q", out.Observation.Error, toolErr.Error())
	}
	if out.Observation.Summary != toolErr.Error() {
		t.Fatalf("observation summary = %q, want %q", out.Observation.Summary, toolErr.Error())
	}
	if len(out.Trace.Events) != 4 {
		t.Fatalf("trace event count = %d, want 4", len(out.Trace.Events))
	}
	if out.Trace.Events[2].Error != toolErr.Error() {
		t.Fatalf("tool trace error = %q, want %q", out.Trace.Events[2].Error, toolErr.Error())
	}
}

func TestRunnerUsesToolSelectionPolicy(t *testing.T) {
	var calledTool string
	agent := runner.New(
		router.RouterFunc(func(ctx context.Context, input *router.RouteInput) (*router.RouteDecision, error) {
			return &router.RouteDecision{
				Target: router.TargetWebSearch,
				Tools:  []string{"web_search", "knowledge"},
				Reason: "unit_test",
			}, nil
		}),
		planner.NewSimplePlanner(),
		runner.ToolInvokerFunc(func(ctx context.Context, toolName string, input *runner.ToolInput) (any, error) {
			calledTool = toolName
			return "ok", nil
		}),
	)
	agent.ToolSelector = policy.RequiredToolPolicy{Name: "knowledge"}

	out, err := agent.Run(context.Background(), &runner.Input{Query: "search latest status"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if calledTool != "knowledge" {
		t.Fatalf("called tool = %q, want knowledge", calledTool)
	}
	if out.ToolResult == nil || out.ToolResult.Source != "knowledge" {
		t.Fatalf("tool result = %#v, want source knowledge", out.ToolResult)
	}
}

func TestRunnerRetriesToolCall(t *testing.T) {
	calls := 0
	agent := runner.New(
		router.RouterFunc(func(ctx context.Context, input *router.RouteInput) (*router.RouteDecision, error) {
			return &router.RouteDecision{
				Target: router.TargetWebSearch,
				Tools:  []string{"web_search"},
				Reason: "unit_test",
			}, nil
		}),
		planner.NewSimplePlanner(),
		runner.ToolInvokerFunc(func(ctx context.Context, toolName string, input *runner.ToolInput) (any, error) {
			calls++
			if calls == 1 {
				return "", errors.New("temporary")
			}
			return "ok after retry", nil
		}),
	)
	agent.RetryPolicy = policy.FixedRetryPolicy{MaxAttempts: 2}

	out, err := agent.Run(context.Background(), &runner.Input{Query: "search latest status"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if out.ToolResult == nil || out.ToolResult.Content != "ok after retry" {
		t.Fatalf("tool result = %#v, want retry output", out.ToolResult)
	}
	if out.Trace.Events[2].Metadata["attempts"] != 2 {
		t.Fatalf("attempt metadata = %v, want 2", out.Trace.Events[2].Metadata["attempts"])
	}
}

func TestRunnerUsesFallbackAfterToolFailure(t *testing.T) {
	agent := runner.New(
		router.RouterFunc(func(ctx context.Context, input *router.RouteInput) (*router.RouteDecision, error) {
			return &router.RouteDecision{
				Target: router.TargetWebSearch,
				Tools:  []string{"web_search"},
				Reason: "unit_test",
			}, nil
		}),
		planner.NewSimplePlanner(),
		runner.ToolInvokerFunc(func(ctx context.Context, toolName string, input *runner.ToolInput) (any, error) {
			return "", errors.New("down")
		}),
	)
	agent.FallbackPolicy = policy.ErrorMessageFallbackPolicy{Prefix: "fallback"}

	out, err := agent.Run(context.Background(), &runner.Input{Query: "search latest status"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if out.ToolResult == nil || out.ToolResult.Error != "" {
		t.Fatalf("tool result = %#v, want fallback without error", out.ToolResult)
	}
	if out.ToolResult.Content != "fallback: down" {
		t.Fatalf("tool result content = %q, want fallback: down", out.ToolResult.Content)
	}
}

func TestRunnerReturnsBudgetExceeded(t *testing.T) {
	tracker := budget.NewTracker(budget.Limits{MaxToolCalls: 1})
	if err := tracker.Consume(context.Background(), budget.ResourceToolCall, 1); err != nil {
		t.Fatalf("preconsume failed: %v", err)
	}
	agent := runner.New(
		router.RouterFunc(func(ctx context.Context, input *router.RouteInput) (*router.RouteDecision, error) {
			return &router.RouteDecision{
				Target: router.TargetWebSearch,
				Tools:  []string{"web_search"},
				Reason: "unit_test",
			}, nil
		}),
		planner.NewSimplePlanner(),
		runner.ToolInvokerFunc(func(ctx context.Context, toolName string, input *runner.ToolInput) (any, error) {
			return "unexpected", nil
		}),
	)
	agent.Budget = tracker

	_, err := agent.Run(context.Background(), &runner.Input{Query: "search latest status"})
	if err == nil {
		t.Fatal("expected budget exceeded error")
	}
	var exceeded *budget.ExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("error = %T, want *budget.ExceededError", err)
	}
	if exceeded.Resource != budget.ResourceToolCall {
		t.Fatalf("resource = %q, want tool_call", exceeded.Resource)
	}
}

func TestRunnerWithSecurityAllowsToolCall(t *testing.T) {
	called := false
	agent := runner.New(
		router.RouterFunc(func(ctx context.Context, input *router.RouteInput) (*router.RouteDecision, error) {
			return &router.RouteDecision{
				Target: router.TargetWebSearch,
				Tools:  []string{"web_search"},
				Reason: "unit_test",
			}, nil
		}),
		planner.NewSimplePlanner(),
		runner.ToolInvokerFunc(func(ctx context.Context, toolName string, input *runner.ToolInput) (any, error) {
			called = true
			return "ok", nil
		}),
		runner.WithSecurity(runner.SecurityOptions{
			Resolver: runner.SecurityResolverFunc(func(ctx context.Context, input *runner.Input) (*safety.SecurityContext, error) {
				return &safety.SecurityContext{UserID: "user-1", Role: "member"}, nil
			}),
			Permission: safety.NewStaticToolPermissionPolicy([]safety.ToolRule{
				{
					ToolName:       "web_search",
					AllowedRoles:   []string{"member"},
					AllowedActions: []string{"read"},
					RequireUser:    true,
				},
			}),
			Guard: safety.NewStaticGuardPolicy(safety.Limits{
				AllowedAgents:      []string{"web_agent"},
				AllowedTools:       []string{"web_search"},
				RequireAgent:       true,
				MaxDelegationDepth: 1,
				MaxContextItems:    1,
			}),
			AgentName:     "web_agent",
			DefaultAction: "read",
		}),
	)

	out, err := agent.Run(context.Background(), &runner.Input{
		Query:           "search latest status",
		DelegationDepth: 1,
		ContextItems:    1,
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !called {
		t.Fatal("invoker was not called")
	}
	if out.ToolResult == nil || out.ToolResult.Source != "web_search" {
		t.Fatalf("tool result = %#v, want web_search", out.ToolResult)
	}
}

func TestRunnerWithSecurityDeniesPermissionBeforeInvoker(t *testing.T) {
	called := false
	agent := runner.New(
		router.RouterFunc(func(ctx context.Context, input *router.RouteInput) (*router.RouteDecision, error) {
			return &router.RouteDecision{
				Target: router.TargetWebSearch,
				Tools:  []string{"web_search"},
				Reason: "unit_test",
			}, nil
		}),
		planner.NewSimplePlanner(),
		runner.ToolInvokerFunc(func(ctx context.Context, toolName string, input *runner.ToolInput) (any, error) {
			called = true
			return "unexpected", nil
		}),
		runner.WithSecurity(runner.SecurityOptions{
			Resolver: runner.SecurityResolverFunc(func(ctx context.Context, input *runner.Input) (*safety.SecurityContext, error) {
				return &safety.SecurityContext{UserID: "user-1", Role: "member"}, nil
			}),
			Permission: safety.NewStaticToolPermissionPolicy([]safety.ToolRule{
				{
					ToolName:       "web_search",
					AllowedRoles:   []string{"admin"},
					AllowedActions: []string{"read"},
					RequireUser:    true,
				},
			}),
			DefaultAction: "read",
		}),
	)

	_, err := agent.Run(context.Background(), &runner.Input{Query: "search latest status"})
	if err == nil {
		t.Fatal("run succeeded, want security denial")
	}
	var denied *runner.SecurityDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("error = %T, want *runner.SecurityDeniedError", err)
	}
	if denied.Stage != "permission" || denied.Reason != safety.ReasonNoAllowedTools {
		t.Fatalf("denied = %#v, want permission no_allowed_tools", denied)
	}
	if called {
		t.Fatal("invoker was called after permission denial")
	}
}

func TestRunnerWithSecurityDeniesGuardBeforeInvoker(t *testing.T) {
	called := false
	agent := runner.New(
		router.RouterFunc(func(ctx context.Context, input *router.RouteInput) (*router.RouteDecision, error) {
			return &router.RouteDecision{
				Target: router.TargetWebSearch,
				Tools:  []string{"web_search"},
				Reason: "unit_test",
			}, nil
		}),
		planner.NewSimplePlanner(),
		runner.ToolInvokerFunc(func(ctx context.Context, toolName string, input *runner.ToolInput) (any, error) {
			called = true
			return "unexpected", nil
		}),
		runner.WithSecurity(runner.SecurityOptions{
			Permission: safety.NewStaticToolPermissionPolicy([]safety.ToolRule{
				{
					ToolName:       "web_search",
					AllowedRoles:   []string{"member"},
					AllowedActions: []string{"read"},
				},
			}),
			Guard: safety.NewStaticGuardPolicy(safety.Limits{
				AllowedAgents: []string{"rag_agent"},
				AllowedTools:  []string{"web_search"},
				RequireAgent:  true,
			}),
			AgentName:     "web_agent",
			DefaultAction: "read",
		}),
	)

	_, err := agent.Run(context.Background(), &runner.Input{
		Query:    "search latest status",
		Security: &safety.SecurityContext{UserID: "user-1", Role: "member"},
	})
	if err == nil {
		t.Fatal("run succeeded, want guard denial")
	}
	var denied *runner.SecurityDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("error = %T, want *runner.SecurityDeniedError", err)
	}
	if denied.Stage != "guard" || denied.Reason != safety.ReasonAgentNotAllowed {
		t.Fatalf("denied = %#v, want guard agent_not_allowed", denied)
	}
	if called {
		t.Fatal("invoker was called after guard denial")
	}
}
