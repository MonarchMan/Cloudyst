package subagent

import (
	"context"

	"ai/internal/pkg/eino/agent/citation"
	"ai/internal/pkg/eino/agent/observe"
	"ai/internal/pkg/eino/agent/router"
	"ai/internal/pkg/eino/agent/trace"

	"github.com/cloudwego/eino/schema"
)

type Task struct {
	ID       string
	Goal     string
	Messages []*schema.Message
	Route    *router.RouteInput
	Metadata map[string]any
}

type Result struct {
	TaskID      string
	AgentName   string
	Observation *observe.Observation
	Citations   []*citation.Citation
	Trace       trace.Trace
	Output      any
	Error       string
	Metadata    map[string]any
}

type Agent interface {
	Run(ctx context.Context, task *Task) (*Result, error)
}

type AgentFunc func(ctx context.Context, task *Task) (*Result, error)

func (f AgentFunc) Run(ctx context.Context, task *Task) (*Result, error) {
	return f(ctx, task)
}
