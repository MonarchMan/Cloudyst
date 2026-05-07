package subagent_test

import (
	"context"
	"testing"

	"ai/internal/pkg/eino/agent/citation"
	"ai/internal/pkg/eino/agent/observe"
	"ai/internal/pkg/eino/agent/planner"
	"ai/internal/pkg/eino/agent/router"
	"ai/internal/pkg/eino/agent/runner"
	"ai/internal/pkg/eino/agent/subagent"
	"ai/internal/pkg/eino/agent/trace"
)

func TestRegistryRunsNamedSubagent(t *testing.T) {
	registry := subagent.NewRegistry()
	if err := registry.Register("web", subagent.AgentFunc(func(ctx context.Context, task *subagent.Task) (*subagent.Result, error) {
		return &subagent.Result{
			TaskID:    task.ID,
			AgentName: "web",
			Observation: &observe.Observation{
				Source:  "web",
				Type:    "text",
				Summary: "web observation",
			},
			Citations: []*citation.Citation{{Index: 1, ID: "web-1", Source: "web", Snippet: "web observation"}},
			Trace:     trace.Trace{Events: []trace.Event{{Node: "web"}}},
		}, nil
	})); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	result, err := registry.Run(context.Background(), "web", &subagent.Task{ID: "task-1", Goal: "search"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if result.TaskID != "task-1" || result.AgentName != "web" {
		t.Fatalf("result = %#v, want task and agent names", result)
	}
	if result.Observation == nil || result.Observation.Summary != "web observation" {
		t.Fatalf("observation = %#v", result.Observation)
	}
}

func TestRunnerAdapterProducesSubagentResult(t *testing.T) {
	agentRunner := runner.New(
		router.RouterFunc(func(ctx context.Context, input *router.RouteInput) (*router.RouteDecision, error) {
			return &router.RouteDecision{
				Target: router.TargetWebSearch,
				Tools:  []string{"web_search"},
			}, nil
		}),
		planner.NewSimplePlanner(),
		runner.ToolInvokerFunc(func(ctx context.Context, toolName string, input *runner.ToolInput) (any, error) {
			return "release notes mention agent routing", nil
		}),
	)
	adapter := subagent.NewRunnerAdapter("web_agent", agentRunner)

	result, err := adapter.Run(context.Background(), &subagent.Task{
		ID:   "task-2",
		Goal: "search latest release notes",
	})
	if err != nil {
		t.Fatalf("adapter run failed: %v", err)
	}
	if result.AgentName != "web_agent" || result.TaskID != "task-2" {
		t.Fatalf("result = %#v, want adapter metadata", result)
	}
	if result.Observation == nil || result.Observation.Summary == "" {
		t.Fatalf("observation = %#v", result.Observation)
	}
	if len(result.Citations) != 1 {
		t.Fatalf("citation count = %d, want 1", len(result.Citations))
	}
	if len(result.Trace.Events) == 0 {
		t.Fatal("trace events are empty")
	}
}

func TestMergeResultsReindexesCitations(t *testing.T) {
	merged := subagent.MergeResults(
		&subagent.Result{
			Observation: &observe.Observation{Source: "web", Summary: "web observation"},
			Citations:   []*citation.Citation{{Index: 1, ID: "web-1", Source: "web", Snippet: "web observation"}},
			Output:      "web output",
		},
		&subagent.Result{
			Observation: &observe.Observation{Source: "rag", Summary: "rag observation"},
			Citations:   []*citation.Citation{{Index: 1, ID: "rag-1", Source: "rag", Snippet: "rag observation"}},
			Error:       "rag warning",
		},
	)

	if len(merged.Observations) != 2 {
		t.Fatalf("observations = %d, want 2", len(merged.Observations))
	}
	if len(merged.Citations) != 2 {
		t.Fatalf("citations = %d, want 2", len(merged.Citations))
	}
	if merged.Citations[0].Index != 1 || merged.Citations[1].Index != 2 {
		t.Fatalf("citation indexes = %d,%d want 1,2", merged.Citations[0].Index, merged.Citations[1].Index)
	}
	if len(merged.Outputs) != 1 || len(merged.Errors) != 1 {
		t.Fatalf("outputs/errors = %d/%d, want 1/1", len(merged.Outputs), len(merged.Errors))
	}
}
