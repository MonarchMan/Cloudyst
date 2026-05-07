package subagent

import (
	"ai/internal/pkg/eino/agent/citation"
	"ai/internal/pkg/eino/agent/observe"
	"ai/internal/pkg/eino/agent/trace"
)

type MergedResult struct {
	Observations []*observe.Observation
	Citations    []*citation.Citation
	Traces       []trace.Trace
	Outputs      []any
	Errors       []string
}

func MergeResults(results ...*Result) *MergedResult {
	merged := &MergedResult{}
	for _, result := range results {
		if result == nil {
			continue
		}
		if result.Observation != nil {
			merged.Observations = append(merged.Observations, result.Observation)
		}
		if len(result.Citations) > 0 {
			merged.Citations = append(merged.Citations, reindexCitations(len(merged.Citations), result.Citations)...)
		}
		if !result.Trace.StartedAt.IsZero() || len(result.Trace.Events) > 0 {
			merged.Traces = append(merged.Traces, result.Trace)
		}
		if result.Output != nil {
			merged.Outputs = append(merged.Outputs, result.Output)
		}
		if result.Error != "" {
			merged.Errors = append(merged.Errors, result.Error)
		}
	}
	return merged
}

func reindexCitations(offset int, citations []*citation.Citation) []*citation.Citation {
	out := make([]*citation.Citation, 0, len(citations))
	for _, c := range citations {
		if c == nil {
			continue
		}
		cp := *c
		cp.Index = offset + len(out) + 1
		out = append(out, &cp)
	}
	return out
}
