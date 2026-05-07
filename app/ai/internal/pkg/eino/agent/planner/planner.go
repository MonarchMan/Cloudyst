package planner

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/schema"
)

type StepType string

const (
	StepThink StepType = "think"
	StepTool  StepType = "tool"
	StepFinal StepType = "final"
)

type Step struct {
	ID          string            `json:"id"`
	Type        StepType          `json:"type"`
	Description string            `json:"description"`
	Tool        string            `json:"tool,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type Plan struct {
	Goal  string `json:"goal"`
	Steps []Step `json:"steps"`
}

type Planner interface {
	Plan(ctx context.Context, goal string, context []*schema.Message) (*Plan, error)
}

type PlannerFunc func(ctx context.Context, goal string, context []*schema.Message) (*Plan, error)

func (f PlannerFunc) Plan(ctx context.Context, goal string, context []*schema.Message) (*Plan, error) {
	return f(ctx, goal, context)
}

type SimplePlanner struct{}

func NewSimplePlanner() *SimplePlanner {
	return &SimplePlanner{}
}

func (p *SimplePlanner) Plan(ctx context.Context, goal string, _ []*schema.Message) (*Plan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	goal = strings.TrimSpace(goal)
	plan := &Plan{Goal: goal}
	if goal == "" {
		return plan, nil
	}

	plan.Steps = append(plan.Steps,
		Step{ID: "think", Type: StepThink, Description: "Understand the user goal and available context."},
	)
	if needsTool(goal) {
		plan.Steps = append(plan.Steps,
			Step{ID: "tool", Type: StepTool, Description: "Call the routed tool and collect observations."},
		)
	}
	plan.Steps = append(plan.Steps,
		Step{ID: "final", Type: StepFinal, Description: "Answer with the collected context and note uncertainty."},
	)
	return plan, nil
}

func needsTool(goal string) bool {
	goal = strings.ToLower(goal)
	for _, term := range []string{
		"search", "latest", "document", "file", "knowledge", "tool",
		"\u641c\u7d22", "\u6700\u65b0", "\u6587\u6863", "\u6587\u4ef6", "\u77e5\u8bc6",
	} {
		if strings.Contains(goal, term) {
			return true
		}
	}
	return false
}
