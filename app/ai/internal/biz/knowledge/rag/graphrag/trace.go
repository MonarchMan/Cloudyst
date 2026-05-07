package graphrag

import (
	"time"
)

const (
	TraceStatusOK    = "ok"
	TraceStatusError = "error"
)

type Trace struct {
	StartedAt     time.Time    `json:"started_at"`
	FinishedAt    time.Time    `json:"finished_at"`
	DurationMs    int64        `json:"duration_ms"`
	Events        []TraceEvent `json:"events"`
	QueryCount    int          `json:"query_count"`
	DocumentCount int          `json:"document_count"`
	ContextChars  int          `json:"context_chars"`
}

type TraceEvent struct {
	Node       string         `json:"node"`
	Status     string         `json:"status"`
	StartedAt  time.Time      `json:"started_at"`
	DurationMs int64          `json:"duration_ms"`
	Error      string         `json:"error,omitempty"`
	Fields     map[string]any `json:"fields,omitempty"`
}

type TraceObserver interface {
	OnEvent(event TraceEvent)
}

type TraceObserverFunc func(event TraceEvent)

func (f TraceObserverFunc) OnEvent(event TraceEvent) {
	f(event)
}

func newTrace() *Trace {
	now := time.Now()
	return &Trace{StartedAt: now}
}

func (t *Trace) finish(queryCount int, documentCount int, contextChars int) {
	if t == nil {
		return
	}
	t.FinishedAt = time.Now()
	t.DurationMs = t.FinishedAt.Sub(t.StartedAt).Milliseconds()
	t.QueryCount = queryCount
	t.DocumentCount = documentCount
	t.ContextChars = contextChars
}
