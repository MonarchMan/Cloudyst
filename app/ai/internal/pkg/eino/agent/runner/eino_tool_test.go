package runner_test

import (
	"context"
	"encoding/json"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ai/internal/pkg/eino/agent/planner"
	"ai/internal/pkg/eino/agent/router"
	"ai/internal/pkg/eino/agent/runner"
)

func TestEinoToolInvokerAdaptsInvokableTool(t *testing.T) {
	fake := &fakeInvokableTool{name: "web_search", output: "ok"}
	invoker, err := runner.NewEinoToolInvoker(context.Background(), fake)
	if err != nil {
		t.Fatalf("new invoker failed: %v", err)
	}
	agent := runner.New(
		router.RouterFunc(func(ctx context.Context, input *router.RouteInput) (*router.RouteDecision, error) {
			return &router.RouteDecision{
				Target: router.TargetWebSearch,
				Tools:  []string{"web_search"},
			}, nil
		}),
		planner.NewSimplePlanner(),
		invoker,
	)

	out, err := agent.Run(context.Background(), &runner.Input{Query: "search latest status"})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if out.ToolResult == nil || out.ToolResult.Content != "ok" {
		t.Fatalf("tool result = %#v, want content ok", out.ToolResult)
	}

	var args map[string]string
	if err := json.Unmarshal([]byte(fake.receivedArguments), &args); err != nil {
		t.Fatalf("tool arguments are not json: %v", err)
	}
	if args["query"] != "search latest status" {
		t.Fatalf("query arg = %q, want search latest status", args["query"])
	}
}

type fakeInvokableTool struct {
	name              string
	output            string
	receivedArguments string
}

func (t *fakeInvokableTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: t.name}, nil
}

func (t *fakeInvokableTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...einotool.Option) (string, error) {
	t.receivedArguments = argumentsInJSON
	return t.output, nil
}
