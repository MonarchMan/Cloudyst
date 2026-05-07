package context_test

import (
	"context"
	"strings"
	"testing"

	"ai/internal/pkg/eino/agent/citation"
	agentcontext "ai/internal/pkg/eino/agent/context"
	"ai/internal/pkg/eino/agent/memory"
	"ai/internal/pkg/eino/agent/observe"

	"github.com/cloudwego/eino/schema"
)

func TestAssemblerBuildsPromptContextAndMessages(t *testing.T) {
	assembler := agentcontext.NewDefaultAssembler()

	msgs, err := assembler.Assemble(context.Background(), &agentcontext.Input{
		SystemPrompt: "You are helpful.",
		RolePrompt:   "Answer as support.",
		Memories: []*memory.Item{
			{Type: memory.TypeLongTerm, Source: "profile", Content: "User prefers concise answers."},
		},
		Observations: []*observe.Observation{
			{Source: "web_search", Type: "text", Summary: "Release notes mention normalized observations."},
		},
		Citations: []*citation.Citation{
			{Index: 1, ID: "cite-1", Source: "web_search", Snippet: "Release notes mention normalized observations."},
		},
		Messages: []*schema.Message{
			schema.UserMessage("What changed?"),
		},
	})
	if err != nil {
		t.Fatalf("assemble failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("message count = %d, want 3", len(msgs))
	}
	if msgs[0].Role != schema.System || !strings.Contains(msgs[0].Content, "You are helpful.") || !strings.Contains(msgs[0].Content, "Answer as support.") {
		t.Fatalf("system prompt message = %#v", msgs[0])
	}
	if msgs[1].Role != schema.System || !strings.Contains(msgs[1].Content, "Agent context:") {
		t.Fatalf("context message = %#v", msgs[1])
	}
	if !strings.Contains(msgs[1].Content, "User prefers concise answers.") {
		t.Fatalf("context missing memory: %q", msgs[1].Content)
	}
	if !strings.Contains(msgs[1].Content, "Release notes mention normalized observations.") {
		t.Fatalf("context missing observation: %q", msgs[1].Content)
	}
	if !strings.Contains(msgs[1].Content, "Citations:") || !strings.Contains(msgs[1].Content, "[1] source=web_search id=cite-1") {
		t.Fatalf("context missing citation: %q", msgs[1].Content)
	}
	if msgs[2].Role != schema.User || msgs[2].Content != "What changed?" {
		t.Fatalf("user message = %#v", msgs[2])
	}
}

func TestAssemblerAppliesItemLimits(t *testing.T) {
	assembler := agentcontext.NewDefaultAssembler()

	msgs, err := assembler.Assemble(context.Background(), &agentcontext.Input{
		Memories: []*memory.Item{
			{Type: memory.TypeShortTerm, Source: "m1", Content: "first memory is long"},
			{Type: memory.TypeShortTerm, Source: "m2", Content: "second memory"},
		},
		Observations: []*observe.Observation{
			{Source: "tool1", Type: "text", Summary: "first observation is long"},
			{Source: "tool2", Type: "text", Summary: "second observation"},
		},
		MaxMemoryItems:      1,
		MaxObservationItems: 1,
		MaxItemChars:        12,
	})
	if err != nil {
		t.Fatalf("assemble failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("message count = %d, want 1", len(msgs))
	}
	content := msgs[0].Content
	if strings.Contains(content, "second memory") {
		t.Fatalf("memory item limit not applied: %q", content)
	}
	if strings.Contains(content, "second observation") {
		t.Fatalf("observation item limit not applied: %q", content)
	}
	if !strings.Contains(content, "first mem...") {
		t.Fatalf("memory char limit not applied: %q", content)
	}
	if !strings.Contains(content, "first obs...") {
		t.Fatalf("observation char limit not applied: %q", content)
	}
}
