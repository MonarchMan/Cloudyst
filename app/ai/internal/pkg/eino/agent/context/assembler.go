package context

import (
	stdcontext "context"
	"fmt"
	"strings"

	"ai/internal/pkg/eino/agent/citation"
	"ai/internal/pkg/eino/agent/memory"
	"ai/internal/pkg/eino/agent/observe"

	"github.com/cloudwego/eino/schema"
)

const (
	DefaultMaxMemoryItems      = 6
	DefaultMaxObservationItems = 6
	DefaultMaxItemChars        = 1200
)

type Input struct {
	SystemPrompt string
	RolePrompt   string
	Messages     []*schema.Message
	Memories     []*memory.Item
	Observations []*observe.Observation
	Citations    []*citation.Citation

	MaxMemoryItems      int
	MaxObservationItems int
	MaxItemChars        int
}

type Assembler interface {
	Assemble(ctx stdcontext.Context, input *Input) ([]*schema.Message, error)
}

type AssemblerFunc func(ctx stdcontext.Context, input *Input) ([]*schema.Message, error)

func (f AssemblerFunc) Assemble(ctx stdcontext.Context, input *Input) ([]*schema.Message, error) {
	return f(ctx, input)
}

type DefaultAssembler struct{}

func NewDefaultAssembler() *DefaultAssembler {
	return &DefaultAssembler{}
}

func (a *DefaultAssembler) Assemble(ctx stdcontext.Context, input *Input) ([]*schema.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, fmt.Errorf("context assembler input is nil")
	}

	out := make([]*schema.Message, 0, len(input.Messages)+2)
	if prompt := joinNonEmpty(input.SystemPrompt, input.RolePrompt); prompt != "" {
		out = append(out, schema.SystemMessage(prompt))
	}
	if block := a.contextBlock(input); block != "" {
		out = append(out, schema.SystemMessage(block))
	}
	out = append(out, cloneMessages(input.Messages)...)
	return out, nil
}

func (a *DefaultAssembler) contextBlock(input *Input) string {
	var sections []string
	if memories := formatMemories(input.Memories, limit(input.MaxMemoryItems, DefaultMaxMemoryItems), limit(input.MaxItemChars, DefaultMaxItemChars)); memories != "" {
		sections = append(sections, "Memories:\n"+memories)
	}
	if observations := formatObservations(input.Observations, limit(input.MaxObservationItems, DefaultMaxObservationItems), limit(input.MaxItemChars, DefaultMaxItemChars)); observations != "" {
		sections = append(sections, "Observations:\n"+observations)
	}
	if citations := citation.FormatBlock(input.Citations); citations != "" {
		sections = append(sections, "Citations:\n"+citations)
	}
	if len(sections) == 0 {
		return ""
	}
	return "Agent context:\n" + strings.Join(sections, "\n\n")
}

func formatMemories(items []*memory.Item, maxItems int, maxChars int) string {
	var lines []string
	for _, item := range items {
		if item == nil || strings.TrimSpace(item.Content) == "" {
			continue
		}
		if len(lines) >= maxItems {
			break
		}
		source := item.Source
		if source == "" {
			source = string(item.Type)
		}
		lines = append(lines, fmt.Sprintf("[%d] source=%s type=%s content=%s", len(lines)+1, source, item.Type, trim(item.Content, maxChars)))
	}
	return strings.Join(lines, "\n")
}

func formatObservations(items []*observe.Observation, maxItems int, maxChars int) string {
	var lines []string
	for _, item := range items {
		if item == nil || strings.TrimSpace(item.Summary) == "" {
			continue
		}
		if len(lines) >= maxItems {
			break
		}
		source := item.Source
		if source == "" {
			source = "tool"
		}
		line := fmt.Sprintf("[%d] source=%s type=%s summary=%s", len(lines)+1, source, item.Type, trim(item.Summary, maxChars))
		if item.Error != "" {
			line += " error=" + trim(item.Error, maxChars)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func cloneMessages(messages []*schema.Message) []*schema.Message {
	out := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		cp := *msg
		out = append(out, &cp)
	}
	return out
}

func joinNonEmpty(parts ...string) string {
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, "\n")
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
