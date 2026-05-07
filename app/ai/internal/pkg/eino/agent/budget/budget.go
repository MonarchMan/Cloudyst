package budget

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Resource string

const (
	ResourceStep        Resource = "step"
	ResourceToolCall    Resource = "tool_call"
	ResourceContextItem Resource = "context_item"
	ResourceTimeout     Resource = "timeout"
)

type Limits struct {
	MaxSteps        int           `json:"max_steps,omitempty"`
	MaxToolCalls    int           `json:"max_tool_calls,omitempty"`
	MaxContextItems int           `json:"max_context_items,omitempty"`
	Timeout         time.Duration `json:"timeout,omitempty"`
}

type Usage struct {
	Steps        int `json:"steps,omitempty"`
	ToolCalls    int `json:"tool_calls,omitempty"`
	ContextItems int `json:"context_items,omitempty"`
}

type ExceededError struct {
	Resource  Resource `json:"resource"`
	Limit     int      `json:"limit,omitempty"`
	Used      int      `json:"used,omitempty"`
	Requested int      `json:"requested,omitempty"`
}

func (e *ExceededError) Error() string {
	if e == nil {
		return ""
	}
	if e.Resource == ResourceTimeout {
		return "budget exceeded: timeout"
	}
	return fmt.Sprintf("budget exceeded: resource=%s limit=%d used=%d requested=%d", e.Resource, e.Limit, e.Used, e.Requested)
}

type Tracker struct {
	mu        sync.Mutex
	limits    Limits
	usage     Usage
	startedAt time.Time
}

func NewTracker(limits Limits) *Tracker {
	return &Tracker{
		limits:    limits,
		startedAt: time.Now(),
	}
}

func (t *Tracker) Limits() Limits {
	if t == nil {
		return Limits{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.limits
}

func (t *Tracker) Usage() Usage {
	if t == nil {
		return Usage{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.usage
}

func (t *Tracker) Remaining() Usage {
	if t == nil {
		return Usage{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return Usage{
		Steps:        remaining(t.limits.MaxSteps, t.usage.Steps),
		ToolCalls:    remaining(t.limits.MaxToolCalls, t.usage.ToolCalls),
		ContextItems: remaining(t.limits.MaxContextItems, t.usage.ContextItems),
	}
}

func (t *Tracker) Consume(ctx context.Context, resource Resource, amount int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if amount <= 0 {
		return nil
	}
	if t == nil {
		return fmt.Errorf("budget tracker is nil")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.timeoutExceededLocked() {
		return &ExceededError{Resource: ResourceTimeout}
	}
	limit, used := t.limitAndUsedLocked(resource)
	if limit > 0 && used+amount > limit {
		return &ExceededError{
			Resource:  resource,
			Limit:     limit,
			Used:      used,
			Requested: amount,
		}
	}
	t.addUsageLocked(resource, amount)
	return nil
}

func (t *Tracker) Check(ctx context.Context, resource Resource, amount int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if amount <= 0 {
		return nil
	}
	if t == nil {
		return fmt.Errorf("budget tracker is nil")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.timeoutExceededLocked() {
		return &ExceededError{Resource: ResourceTimeout}
	}
	limit, used := t.limitAndUsedLocked(resource)
	if limit > 0 && used+amount > limit {
		return &ExceededError{
			Resource:  resource,
			Limit:     limit,
			Used:      used,
			Requested: amount,
		}
	}
	return nil
}

func (t *Tracker) WithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if t == nil {
		return context.WithCancel(ctx)
	}
	t.mu.Lock()
	timeout := t.limits.Timeout
	t.mu.Unlock()
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func (t *Tracker) limitAndUsedLocked(resource Resource) (int, int) {
	switch resource {
	case ResourceStep:
		return t.limits.MaxSteps, t.usage.Steps
	case ResourceToolCall:
		return t.limits.MaxToolCalls, t.usage.ToolCalls
	case ResourceContextItem:
		return t.limits.MaxContextItems, t.usage.ContextItems
	default:
		return 0, 0
	}
}

func (t *Tracker) addUsageLocked(resource Resource, amount int) {
	switch resource {
	case ResourceStep:
		t.usage.Steps += amount
	case ResourceToolCall:
		t.usage.ToolCalls += amount
	case ResourceContextItem:
		t.usage.ContextItems += amount
	}
}

func (t *Tracker) timeoutExceededLocked() bool {
	return t.limits.Timeout > 0 && time.Since(t.startedAt) > t.limits.Timeout
}

func remaining(limit int, used int) int {
	if limit <= 0 {
		return 0
	}
	if used >= limit {
		return 0
	}
	return limit - used
}
