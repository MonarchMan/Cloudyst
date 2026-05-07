package verify

import (
	"context"
	"regexp"
	"strings"

	"ai/internal/pkg/eino/agent/observe"
)

type GroundInput struct {
	Answer           string                `json:"answer"`
	Context          []*observe.ToolResult `json:"context,omitempty"`
	RequireCitations bool                  `json:"require_citations,omitempty"`
}

type GroundResult struct {
	Passed             bool     `json:"passed"`
	Reasons            []string `json:"reasons,omitempty"`
	HasAnswer          bool     `json:"has_answer"`
	HasCitations       bool     `json:"has_citations"`
	ReferencedCitation []int    `json:"referenced_citation,omitempty"`
}

type Grounder interface {
	Ground(ctx context.Context, input *GroundInput) (*GroundResult, error)
}

type GrounderFunc func(ctx context.Context, input *GroundInput) (*GroundResult, error)

func (f GrounderFunc) Ground(ctx context.Context, input *GroundInput) (*GroundResult, error) {
	return f(ctx, input)
}

type CitationGrounder struct{}

func NewCitationGrounder() *CitationGrounder {
	return &CitationGrounder{}
}

func (g *CitationGrounder) Ground(ctx context.Context, input *GroundInput) (*GroundResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	result := &GroundResult{}
	if input == nil {
		result.Reasons = append(result.Reasons, "nil_input")
		return result, nil
	}

	answer := strings.TrimSpace(input.Answer)
	result.HasAnswer = answer != ""
	result.ReferencedCitation = referencedCitations(answer)
	result.HasCitations = len(result.ReferencedCitation) > 0

	if !result.HasAnswer {
		result.Reasons = append(result.Reasons, "empty_answer")
	}
	if input.RequireCitations && len(input.Context) > 0 && !result.HasCitations {
		result.Reasons = append(result.Reasons, "missing_citations")
	}
	for _, citation := range result.ReferencedCitation {
		if citation <= 0 || citation > len(input.Context) {
			result.Reasons = append(result.Reasons, "citation_out_of_range")
			break
		}
	}

	result.Passed = len(result.Reasons) == 0
	return result, nil
}

func referencedCitations(answer string) []int {
	matches := regexp.MustCompile(`\[(\d+)\]`).FindAllStringSubmatch(answer, -1)
	seen := map[string]struct{}{}
	out := make([]int, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		if _, ok := seen[match[1]]; ok {
			continue
		}
		seen[match[1]] = struct{}{}
		var n int
		for _, r := range match[1] {
			n = n*10 + int(r-'0')
		}
		out = append(out, n)
	}
	return out
}
