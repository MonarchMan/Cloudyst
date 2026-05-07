package graphrag

import (
	"context"
	"regexp"
	"strings"
)

type AnswerVerification struct {
	Passed             bool     `json:"passed"`
	Reasons            []string `json:"reasons"`
	HasAnswer          bool     `json:"has_answer"`
	HasCitations       bool     `json:"has_citations"`
	RequiresCitations  bool     `json:"requires_citations"`
	ReferencedCitation []int    `json:"referenced_citation"`
}

type AnswerVerifier interface {
	Verify(ctx context.Context, input *AnswerInput, answer string) (*AnswerVerification, error)
}

type AnswerVerifyFunc func(ctx context.Context, input *AnswerInput, answer string) (*AnswerVerification, error)

func (f AnswerVerifyFunc) Verify(ctx context.Context, input *AnswerInput, answer string) (*AnswerVerification, error) {
	return f(ctx, input, answer)
}

type DefaultAnswerVerifier struct {
	RequireCitations bool
}

func (v DefaultAnswerVerifier) Verify(ctx context.Context, input *AnswerInput, answer string) (*AnswerVerification, error) {
	verification := &AnswerVerification{
		HasAnswer:         strings.TrimSpace(answer) != "",
		RequiresCitations: v.RequireCitations && input != nil && len(input.Citations) > 0,
	}
	verification.ReferencedCitation = referencedCitations(answer)
	verification.HasCitations = len(verification.ReferencedCitation) > 0
	if !verification.HasAnswer {
		verification.Reasons = append(verification.Reasons, "empty_answer")
	}
	if verification.RequiresCitations && !verification.HasCitations {
		verification.Reasons = append(verification.Reasons, "missing_citations")
	}
	verification.Passed = len(verification.Reasons) == 0
	return verification, nil
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
