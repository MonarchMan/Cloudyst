package agent_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ai/internal/pkg/eino/agent/citation"
	agentcontext "ai/internal/pkg/eino/agent/context"
	"ai/internal/pkg/eino/agent/observe"
	"ai/internal/pkg/eino/agent/planner"
	"ai/internal/pkg/eino/agent/policy"
	"ai/internal/pkg/eino/agent/router"
	"ai/internal/pkg/eino/agent/runner"
	"ai/internal/pkg/eino/agent/verify"

	"github.com/cloudwego/eino/schema"
)

func TestAgentComponentChainRunnerCitationContextGrounder(t *testing.T) {
	ctx := context.Background()
	query := "search latest product release notes"

	agent := runner.New(
		router.NewRuleRouter(),
		planner.NewSimplePlanner(),
		runner.ToolInvokerFunc(func(ctx context.Context, toolName string, input *runner.ToolInput) (any, error) {
			return callDemoTool(toolName)
		}),
	)
	agent.Observer = observe.NewSummaryObserver(120)

	run, err := agent.Run(ctx, &runner.Input{
		Query: query,
		Route: &router.RouteInput{
			Capabilities: []router.Capability{
				{Name: "web_search", Target: router.TargetWebSearch},
				{Name: "knowledge_retrieval", Target: router.TargetRAG},
			},
			KnowledgeAvailable: true,
		},
	})
	if err != nil {
		t.Fatalf("runner failed: %v", err)
	}
	if run.ToolResult == nil || run.Observation == nil {
		t.Fatalf("runner did not produce tool context: tool=%#v observation=%#v", run.ToolResult, run.Observation)
	}

	citations, err := citation.NewDefaultBuilder().Build(ctx, citation.SourcesFromToolResults([]*observe.ToolResult{run.ToolResult}))
	if err != nil {
		t.Fatalf("build citations failed: %v", err)
	}
	if len(citations) != 1 {
		t.Fatalf("citation count = %d, want 1", len(citations))
	}

	messages, err := agentcontext.NewDefaultAssembler().Assemble(ctx, &agentcontext.Input{
		SystemPrompt: "Use citations when context is provided.",
		Observations: []*observe.Observation{
			run.Observation,
		},
		Citations: citations,
		Messages: []*schema.Message{
			schema.UserMessage(query),
		},
	})
	if err != nil {
		t.Fatalf("assemble context failed: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("message count = %d, want 3", len(messages))
	}
	if !strings.Contains(messages[1].Content, "Citations:") {
		t.Fatalf("assembled context missing citations: %q", messages[1].Content)
	}

	answer, err := answerFromContext(ctx, messages)
	if err != nil {
		t.Fatalf("answer failed: %v", err)
	}
	grounding, err := verify.NewCitationGrounder().Ground(ctx, &verify.GroundInput{
		Answer:           answer,
		Context:          []*observe.ToolResult{run.ToolResult},
		RequireCitations: true,
	})
	if err != nil {
		t.Fatalf("ground failed: %v", err)
	}
	if !grounding.Passed {
		t.Fatalf("grounding failed: %v", grounding.Reasons)
	}

	t.Logf("chain target=%s citation=%s answer=%q grounded=%v trace_events=%d",
		run.Decision.Target,
		citations[0].ID,
		answer,
		grounding.Passed,
		len(run.Trace.Events),
	)
}

func TestAgentComponentChainFallbackExplainsToolFailure(t *testing.T) {
	ctx := context.Background()
	query := "search latest product release notes"

	agent := runner.New(
		router.NewRuleRouter(),
		planner.NewSimplePlanner(),
		runner.ToolInvokerFunc(func(ctx context.Context, toolName string, input *runner.ToolInput) (any, error) {
			return "", errors.New("search backend down")
		}),
	)
	agent.Observer = observe.NewSummaryObserver(120)
	agent.FallbackPolicy = policy.ErrorMessageFallbackPolicy{Prefix: "fallback"}

	run, err := agent.Run(ctx, &runner.Input{
		Query: query,
		Route: &router.RouteInput{
			Capabilities: []router.Capability{{Name: "web_search", Target: router.TargetWebSearch}},
		},
	})
	if err != nil {
		t.Fatalf("runner failed: %v", err)
	}
	if run.ToolResult == nil || run.ToolResult.Content != "fallback: search backend down" {
		t.Fatalf("tool result = %#v, want fallback content", run.ToolResult)
	}

	citations, err := citation.NewDefaultBuilder().Build(ctx, citation.SourcesFromToolResults([]*observe.ToolResult{run.ToolResult}))
	if err != nil {
		t.Fatalf("build citations failed: %v", err)
	}
	messages, err := agentcontext.NewDefaultAssembler().Assemble(ctx, &agentcontext.Input{
		Observations: []*observe.Observation{run.Observation},
		Citations:    citations,
		Messages:     []*schema.Message{schema.UserMessage(query)},
	})
	if err != nil {
		t.Fatalf("assemble context failed: %v", err)
	}
	if !strings.Contains(messages[0].Content, "fallback: search backend down") {
		t.Fatalf("assembled context missing fallback: %q", messages[0].Content)
	}

	answer := "The search tool was unavailable, so the answer is based on the fallback observation [1]."
	grounding, err := verify.NewCitationGrounder().Ground(ctx, &verify.GroundInput{
		Answer:           answer,
		Context:          []*observe.ToolResult{run.ToolResult},
		RequireCitations: true,
	})
	if err != nil {
		t.Fatalf("ground failed: %v", err)
	}
	if !grounding.Passed {
		t.Fatalf("grounding failed: %v", grounding.Reasons)
	}

	t.Logf("fallback chain citation=%s answer=%q grounded=%v", citations[0].ID, answer, grounding.Passed)
}

func TestAgentComponentChainGrounderFailsWithoutCitation(t *testing.T) {
	ctx := context.Background()
	query := "search latest product release notes"

	agent := runner.New(
		router.NewRuleRouter(),
		planner.NewSimplePlanner(),
		runner.ToolInvokerFunc(func(ctx context.Context, toolName string, input *runner.ToolInput) (any, error) {
			return callDemoTool(toolName)
		}),
	)
	run, err := agent.Run(ctx, &runner.Input{
		Query: query,
		Route: &router.RouteInput{
			Capabilities: []router.Capability{{Name: "web_search", Target: router.TargetWebSearch}},
		},
	})
	if err != nil {
		t.Fatalf("runner failed: %v", err)
	}

	answer := "The release notes mention web search routing and normalized observations."
	grounding, err := verify.NewCitationGrounder().Ground(ctx, &verify.GroundInput{
		Answer:           answer,
		Context:          []*observe.ToolResult{run.ToolResult},
		RequireCitations: true,
	})
	if err != nil {
		t.Fatalf("ground failed: %v", err)
	}
	if grounding.Passed {
		t.Fatal("grounding passed, want missing_citations failure")
	}
	if !containsReason(grounding.Reasons, "missing_citations") {
		t.Fatalf("reasons = %v, want missing_citations", grounding.Reasons)
	}

	t.Logf("missing citation answer=%q grounded=%v reasons=%v", answer, grounding.Passed, grounding.Reasons)
}

func answerFromContext(ctx context.Context, messages []*schema.Message) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if strings.Contains(msg.Content, "Citations:") && strings.Contains(msg.Content, "[1]") {
			return "The release notes mention web search routing and normalized observations [1].", nil
		}
	}
	return "The available context does not include a citation.", nil
}

func containsReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
