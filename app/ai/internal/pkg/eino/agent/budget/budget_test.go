package budget_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"ai/internal/pkg/eino/agent/budget"
)

func TestTrackerConsumeAndRemaining(t *testing.T) {
	tracker := budget.NewTracker(budget.Limits{
		MaxSteps:        3,
		MaxToolCalls:    2,
		MaxContextItems: 4,
	})

	if err := tracker.Consume(context.Background(), budget.ResourceStep, 2); err != nil {
		t.Fatalf("consume step failed: %v", err)
	}
	if err := tracker.Consume(context.Background(), budget.ResourceToolCall, 1); err != nil {
		t.Fatalf("consume tool call failed: %v", err)
	}

	usage := tracker.Usage()
	if usage.Steps != 2 || usage.ToolCalls != 1 {
		t.Fatalf("usage = %#v, want steps=2 tool_calls=1", usage)
	}
	remaining := tracker.Remaining()
	if remaining.Steps != 1 || remaining.ToolCalls != 1 || remaining.ContextItems != 4 {
		t.Fatalf("remaining = %#v, want steps=1 tool_calls=1 context_items=4", remaining)
	}
}

func TestTrackerReturnsExceededError(t *testing.T) {
	tracker := budget.NewTracker(budget.Limits{MaxToolCalls: 1})

	if err := tracker.Consume(context.Background(), budget.ResourceToolCall, 1); err != nil {
		t.Fatalf("consume tool call failed: %v", err)
	}
	err := tracker.Consume(context.Background(), budget.ResourceToolCall, 1)
	if err == nil {
		t.Fatal("expected exceeded error")
	}

	var exceeded *budget.ExceededError
	if !errors.As(err, &exceeded) {
		t.Fatalf("error = %T, want *ExceededError", err)
	}
	if exceeded.Resource != budget.ResourceToolCall || exceeded.Limit != 1 || exceeded.Used != 1 || exceeded.Requested != 1 {
		t.Fatalf("exceeded = %#v", exceeded)
	}
	if tracker.Usage().ToolCalls != 1 {
		t.Fatalf("usage changed after failed consume: %#v", tracker.Usage())
	}
}

func TestTrackerCheckDoesNotConsume(t *testing.T) {
	tracker := budget.NewTracker(budget.Limits{MaxContextItems: 2})

	if err := tracker.Check(context.Background(), budget.ResourceContextItem, 2); err != nil {
		t.Fatalf("check context items failed: %v", err)
	}
	if tracker.Usage().ContextItems != 0 {
		t.Fatalf("check consumed budget: %#v", tracker.Usage())
	}
}

func TestTrackerWithTimeout(t *testing.T) {
	tracker := budget.NewTracker(budget.Limits{Timeout: time.Minute})
	ctx, cancel := tracker.WithTimeout(context.Background())
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("timeout context has no deadline")
	}
	if time.Until(deadline) <= 0 {
		t.Fatalf("deadline is not in the future: %v", deadline)
	}
}
