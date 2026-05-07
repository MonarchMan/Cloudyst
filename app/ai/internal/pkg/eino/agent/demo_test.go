package agent_test

import (
	"context"
	"errors"
	"testing"

	"ai/internal/pkg/eino/agent/observe"
	"ai/internal/pkg/eino/agent/planner"
	"ai/internal/pkg/eino/agent/router"
	"ai/internal/pkg/eino/agent/runner"
)

func TestRouteNormalizeSummarize(t *testing.T) {
	ctx := context.Background()
	r := router.NewRuleRouter()

	decision, err := r.Route(ctx, &router.RouteInput{
		Query: "search latest product release notes",
		Capabilities: []router.Capability{
			{Name: "web_search", Target: router.TargetWebSearch},
			{Name: "knowledge_retrieval", Target: router.TargetRAG},
		},
		KnowledgeAvailable: true,
	})
	if err != nil {
		t.Fatalf("route failed: %v", err)
	}
	if decision.Target != router.TargetWebSearch {
		t.Fatalf("target = %q, want %q", decision.Target, router.TargetWebSearch)
	}
	if len(decision.Tools) != 1 || decision.Tools[0] != "web_search" {
		t.Fatalf("tools = %v, want [web_search]", decision.Tools)
	}

	toolOutput, toolErr := callDemoTool(decision.Tools[0])
	normalizer := observe.NewDefaultNormalizer()
	normalized, err := normalizer.Normalize(ctx, decision.Tools[0], toolOutput, toolErr)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if normalized.Error != "" {
		t.Fatalf("normalized error = %q", normalized.Error)
	}

	observer := observe.NewSummaryObserver(32)
	observation, err := observer.Summarize(ctx, normalized)
	if err != nil {
		t.Fatalf("summarize failed: %v", err)
	}
	if observation.Source != "web_search" {
		t.Fatalf("source = %q, want web_search", observation.Source)
	}
	if len(observation.Summary) > 32 {
		t.Fatalf("summary length = %d, want <= 32", len(observation.Summary))
	}
}

func TestAgentDemoRoutePlanToolObserveGroundTrace(t *testing.T) {
	agent := runner.New(
		router.NewRuleRouter(),
		planner.NewSimplePlanner(),
		runner.ToolInvokerFunc(func(ctx context.Context, toolName string, input *runner.ToolInput) (any, error) {
			return callDemoTool(toolName)
		}),
	)
	agent.Observer = observe.NewSummaryObserver(96)
	agent.Answerer = runner.AnswererFunc(func(ctx context.Context, input *runner.AnswerInput) (string, error) {
		return "The demo found release notes about web search routing and normalized observations [1].", nil
	})

	run, err := agent.Run(context.Background(), &runner.Input{
		Query: "search latest product release notes",
		Route: &router.RouteInput{
			Capabilities: []router.Capability{
				{Name: "web_search", Target: router.TargetWebSearch},
				{Name: "knowledge_retrieval", Target: router.TargetRAG},
			},
			KnowledgeAvailable: true,
		},
		RequireCitations: true,
	})
	if err != nil {
		t.Fatalf("run demo agent failed: %v", err)
	}

	if run.Decision.Target != router.TargetWebSearch {
		t.Fatalf("target = %q, want %q", run.Decision.Target, router.TargetWebSearch)
	}
	if !hasStep(run.Plan, planner.StepTool) {
		t.Fatalf("plan steps = %#v, want a tool step", run.Plan.Steps)
	}
	if run.Observation.Summary == "" {
		t.Fatal("observation summary is empty")
	}
	if !run.Grounding.Passed {
		t.Fatalf("grounding failed: %v", run.Grounding.Reasons)
	}
	if len(run.Trace.Events) != 5 {
		t.Fatalf("trace event count = %d, want 5", len(run.Trace.Events))
	}

	t.Logf(
		"target=%s tools=%v steps=%d observation=%q grounded=%v trace_events=%d",
		run.Decision.Target,
		run.Decision.Tools,
		len(run.Plan.Steps),
		run.Observation.Summary,
		run.Grounding.Passed,
		len(run.Trace.Events),
	)
}

func callDemoTool(name string) (string, error) {
	if name != "web_search" {
		return "", errors.New("unknown tool")
	}
	return "release notes: agent router now supports web search routing with normalized observations", nil
}

func first(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[0]
}

func hasStep(plan *planner.Plan, stepType planner.StepType) bool {
	if plan == nil {
		return false
	}
	for _, step := range plan.Steps {
		if step.Type == stepType {
			return true
		}
	}
	return false
}
