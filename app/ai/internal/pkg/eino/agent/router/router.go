package router

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/schema"
)

type Target string

const (
	TargetLLM       Target = "llm"
	TargetRAG       Target = "rag"
	TargetWebSearch Target = "web_search"
	TargetFile      Target = "file"
	TargetMCP       Target = "mcp"
)

type Capability struct {
	Name        string            `json:"name"`
	Target      Target            `json:"target"`
	Description string            `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type RouteInput struct {
	Query              string            `json:"query"`
	Role               string            `json:"role,omitempty"`
	Messages           []*schema.Message `json:"messages,omitempty"`
	Capabilities       []Capability      `json:"capabilities,omitempty"`
	KnowledgeAvailable bool              `json:"knowledge_available,omitempty"`
	FileAvailable      bool              `json:"file_available,omitempty"`
	MCPAvailable       bool              `json:"mcp_available,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

type RouteDecision struct {
	Target     Target            `json:"target"`
	Tools      []string          `json:"tools,omitempty"`
	Reason     string            `json:"reason,omitempty"`
	Confidence float64           `json:"confidence,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type Router interface {
	Route(ctx context.Context, input *RouteInput) (*RouteDecision, error)
}

type RouterFunc func(ctx context.Context, input *RouteInput) (*RouteDecision, error)

func (f RouterFunc) Route(ctx context.Context, input *RouteInput) (*RouteDecision, error) {
	return f(ctx, input)
}

type RuleRouter struct{}

func NewRuleRouter() *RuleRouter {
	return &RuleRouter{}
}

func (r *RuleRouter) Route(ctx context.Context, input *RouteInput) (*RouteDecision, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input == nil {
		return &RouteDecision{Target: TargetLLM, Reason: "empty_input", Confidence: 0.2}, nil
	}

	query := strings.ToLower(strings.TrimSpace(input.Query))
	switch {
	case input.KnowledgeAvailable && hasAny(query,
		"knowledge", "doc", "document", "manual", "policy", "kb",
		"\u77e5\u8bc6", "\u6587\u6863", "\u624b\u518c", "\u8d44\u6599",
	):
		return r.decision(input, TargetRAG, "knowledge_hint", 0.8), nil
	case hasAny(query,
		"latest", "today", "news", "price", "web", "search",
		"\u6700\u65b0", "\u4eca\u5929", "\u65b0\u95fb", "\u641c\u7d22", "\u8054\u7f51",
	):
		return r.decision(input, TargetWebSearch, "freshness_hint", 0.75), nil
	case input.FileAvailable && hasAny(query,
		"file", "folder", "upload", "download",
		"\u6587\u4ef6", "\u76ee\u5f55", "\u4e0a\u4f20", "\u4e0b\u8f7d",
	):
		return r.decision(input, TargetFile, "file_hint", 0.75), nil
	case input.MCPAvailable && hasAny(query,
		"mcp", "plugin", "\u63d2\u4ef6", "\u5de5\u5177\u670d\u52a1\u5668",
	):
		return r.decision(input, TargetMCP, "mcp_hint", 0.7), nil
	default:
		return r.decision(input, TargetLLM, "no_tool_hint", 0.6), nil
	}
}

func (r *RuleRouter) decision(input *RouteInput, target Target, reason string, confidence float64) *RouteDecision {
	return &RouteDecision{
		Target:     target,
		Tools:      toolsForTarget(input.Capabilities, target),
		Reason:     reason,
		Confidence: confidence,
	}
}

func toolsForTarget(capabilities []Capability, target Target) []string {
	tools := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		if capability.Target == target && capability.Name != "" {
			tools = append(tools, capability.Name)
		}
	}
	return tools
}

func hasAny(s string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(s, strings.ToLower(term)) {
			return true
		}
	}
	return false
}
