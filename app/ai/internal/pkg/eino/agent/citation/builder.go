package citation

import (
	"context"
	"crypto/sha1"
	"fmt"
	"strings"

	"ai/internal/pkg/eino/agent/memory"
	"ai/internal/pkg/eino/agent/observe"
)

const (
	DefaultMaxItems        = 20
	DefaultMaxSnippetChars = 240
)

type SourceType string

const (
	SourceTypeToolResult  SourceType = "tool_result"
	SourceTypeObservation SourceType = "observation"
	SourceTypeMemory      SourceType = "memory"
)

type Source struct {
	ID       string         `json:"id,omitempty"`
	Type     SourceType     `json:"type,omitempty"`
	Source   string         `json:"source,omitempty"`
	Content  string         `json:"content,omitempty"`
	Score    float64        `json:"score,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Error    string         `json:"error,omitempty"`
}

type Citation struct {
	Index    int            `json:"index"`
	ID       string         `json:"id"`
	Type     SourceType     `json:"type,omitempty"`
	Source   string         `json:"source,omitempty"`
	Snippet  string         `json:"snippet"`
	Score    float64        `json:"score,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Error    string         `json:"error,omitempty"`
}

type Builder interface {
	Build(ctx context.Context, sources []*Source) ([]*Citation, error)
}

type BuilderFunc func(ctx context.Context, sources []*Source) ([]*Citation, error)

func (f BuilderFunc) Build(ctx context.Context, sources []*Source) ([]*Citation, error) {
	return f(ctx, sources)
}

type DefaultBuilder struct {
	MaxItems        int
	MaxSnippetChars int
}

func NewDefaultBuilder() *DefaultBuilder {
	return &DefaultBuilder{
		MaxItems:        DefaultMaxItems,
		MaxSnippetChars: DefaultMaxSnippetChars,
	}
}

func (b *DefaultBuilder) Build(ctx context.Context, sources []*Source) ([]*Citation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b == nil {
		b = NewDefaultBuilder()
	}

	maxItems := limit(b.MaxItems, DefaultMaxItems)
	maxChars := limit(b.MaxSnippetChars, DefaultMaxSnippetChars)
	citations := make([]*Citation, 0, len(sources))
	for _, source := range sources {
		if source == nil {
			continue
		}
		content := strings.TrimSpace(source.Content)
		if content == "" {
			content = strings.TrimSpace(source.Error)
		}
		if content == "" {
			continue
		}
		if len(citations) >= maxItems {
			break
		}
		citation := &Citation{
			Index:    len(citations) + 1,
			ID:       source.ID,
			Type:     source.Type,
			Source:   source.Source,
			Snippet:  trim(content, maxChars),
			Score:    source.Score,
			Metadata: source.Metadata,
			Error:    source.Error,
		}
		if citation.ID == "" {
			citation.ID = stableID(source, content)
		}
		citations = append(citations, citation)
	}
	return citations, nil
}

func FormatBlock(citations []*Citation) string {
	var lines []string
	for _, citation := range citations {
		if citation == nil || citation.Snippet == "" {
			continue
		}
		source := citation.Source
		if source == "" {
			source = string(citation.Type)
		}
		line := fmt.Sprintf("[%d] source=%s id=%s snippet=%s", citation.Index, source, citation.ID, citation.Snippet)
		if citation.Error != "" {
			line += " error=" + trim(citation.Error, DefaultMaxSnippetChars)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func SourcesFromToolResults(results []*observe.ToolResult) []*Source {
	sources := make([]*Source, 0, len(results))
	for _, result := range results {
		if result == nil {
			continue
		}
		sources = append(sources, &Source{
			Type:     SourceTypeToolResult,
			Source:   result.Source,
			Content:  result.Content,
			Score:    result.Score,
			Metadata: result.Metadata,
			Error:    result.Error,
		})
	}
	return sources
}

func SourcesFromObservations(observations []*observe.Observation) []*Source {
	sources := make([]*Source, 0, len(observations))
	for _, observation := range observations {
		if observation == nil {
			continue
		}
		sources = append(sources, &Source{
			Type:     SourceTypeObservation,
			Source:   observation.Source,
			Content:  observation.Summary,
			Metadata: observation.Metadata,
			Error:    observation.Error,
		})
	}
	return sources
}

func SourcesFromMemories(memories []*memory.Item) []*Source {
	sources := make([]*Source, 0, len(memories))
	for _, item := range memories {
		if item == nil {
			continue
		}
		source := item.Source
		if source == "" {
			source = string(item.Type)
		}
		sources = append(sources, &Source{
			Type:     SourceTypeMemory,
			Source:   source,
			Content:  item.Content,
			Score:    item.Score,
			Metadata: item.Metadata,
		})
	}
	return sources
}

func stableID(source *Source, content string) string {
	key := strings.Join([]string{string(source.Type), source.Source, content}, "\x00")
	sum := sha1.Sum([]byte(key))
	return fmt.Sprintf("%x", sum[:8])
}

func limit(value int, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func trim(s string, maxChars int) string {
	s = strings.Join(strings.Fields(s), " ")
	if maxChars <= 0 || len(s) <= maxChars {
		return s
	}
	if maxChars <= 3 {
		return s[:maxChars]
	}
	return s[:maxChars-3] + "..."
}
