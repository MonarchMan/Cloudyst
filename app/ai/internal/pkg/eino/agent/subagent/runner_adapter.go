package subagent

import (
	"context"
	"fmt"

	"ai/internal/pkg/eino/agent/citation"
	"ai/internal/pkg/eino/agent/observe"
	"ai/internal/pkg/eino/agent/runner"
)

type RunnerAdapter struct {
	Name            string
	Runner          *runner.Runner
	CitationBuilder citation.Builder
}

func NewRunnerAdapter(name string, r *runner.Runner) *RunnerAdapter {
	return &RunnerAdapter{
		Name:            name,
		Runner:          r,
		CitationBuilder: citation.NewDefaultBuilder(),
	}
}

func (a *RunnerAdapter) Run(ctx context.Context, task *Task) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a == nil {
		return nil, fmt.Errorf("subagent runner adapter is nil")
	}
	if a.Runner == nil {
		return nil, fmt.Errorf("subagent runner is nil")
	}
	if task == nil {
		return nil, fmt.Errorf("subagent task is nil")
	}

	out, err := a.Runner.Run(ctx, &runner.Input{
		Query:    task.Goal,
		Messages: task.Messages,
		Route:    task.Route,
	})
	if err != nil {
		return nil, err
	}

	var citations []*citation.Citation
	if out.ToolResult != nil {
		builder := a.CitationBuilder
		if builder == nil {
			builder = citation.NewDefaultBuilder()
		}
		citations, err = builder.Build(ctx, citation.SourcesFromToolResults([]*observe.ToolResult{out.ToolResult}))
		if err != nil {
			return nil, err
		}
	}
	return &Result{
		TaskID:      task.ID,
		AgentName:   a.Name,
		Observation: out.Observation,
		Citations:   citations,
		Trace:       out.Trace,
		Output:      out,
	}, nil
}
