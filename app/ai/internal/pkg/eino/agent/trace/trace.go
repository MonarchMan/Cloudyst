package trace

import (
	"sync"
	"time"
)

const (
	StatusOK    = "ok"
	StatusError = "error"
)

type Event struct {
	Node          string         `json:"node"`
	InputSummary  string         `json:"input_summary,omitempty"`
	OutputSummary string         `json:"output_summary,omitempty"`
	Tool          string         `json:"tool,omitempty"`
	LatencyMs     int64          `json:"latency_ms,omitempty"`
	Error         string         `json:"error,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type Trace struct {
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Events     []Event   `json:"events,omitempty"`
}

type Recorder interface {
	Record(event Event)
	Finish() Trace
}

type InMemoryRecorder struct {
	mu    sync.Mutex
	trace Trace
}

func NewInMemoryRecorder() *InMemoryRecorder {
	return &InMemoryRecorder{
		trace: Trace{StartedAt: time.Now()},
	}
}

func (r *InMemoryRecorder) Record(event Event) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.trace.Events = append(r.trace.Events, event)
}

func (r *InMemoryRecorder) Finish() Trace {
	if r == nil {
		return Trace{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.trace.FinishedAt = time.Now()
	return r.trace
}

func Measure(node string, run func() (outputSummary string, err error)) Event {
	start := time.Now()
	output, err := run()
	event := Event{
		Node:          node,
		OutputSummary: output,
		LatencyMs:     time.Since(start).Milliseconds(),
	}
	if err != nil {
		event.Error = err.Error()
	}
	return event
}
