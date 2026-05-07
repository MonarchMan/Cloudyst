package graphrag

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

type Evaluation struct {
	QueryCount             int     `json:"query_count"`
	RetrievedDocumentCount int     `json:"retrieved_document_count"`
	FinalDocumentCount     int     `json:"final_document_count"`
	CitationCount          int     `json:"citation_count"`
	ContextChars           int     `json:"context_chars"`
	AnswerChars            int     `json:"answer_chars"`
	HasAnswer              bool    `json:"has_answer"`
	HasCitationsInAnswer   bool    `json:"has_citations_in_answer"`
	CitationCoverage       float64 `json:"citation_coverage"`
	ContextDensity         float64 `json:"context_density"`
}

type Evaluator interface {
	Evaluate(result *Result) *Evaluation
}

type EvaluatorFunc func(result *Result) *Evaluation

func (f EvaluatorFunc) Evaluate(result *Result) *Evaluation {
	return f(result)
}

type DefaultEvaluator struct{}

func (DefaultEvaluator) Evaluate(result *Result) *Evaluation {
	if result == nil {
		return &Evaluation{}
	}
	retrieved := 0
	for _, retrieval := range result.Retrievals {
		retrieved += len(retrieval.Documents)
	}
	eval := &Evaluation{
		QueryCount:             len(result.Queries),
		RetrievedDocumentCount: retrieved,
		FinalDocumentCount:     len(result.Documents),
		CitationCount:          len(result.Citations),
		ContextChars:           utf8.RuneCountInString(result.Context),
		AnswerChars:            utf8.RuneCountInString(result.Answer),
		HasAnswer:              strings.TrimSpace(result.Answer) != "",
		HasCitationsInAnswer:   regexp.MustCompile(`\[\d+\]`).MatchString(result.Answer),
	}
	if len(result.Documents) > 0 {
		eval.CitationCoverage = float64(len(result.Citations)) / float64(len(result.Documents))
	}
	if eval.ContextChars > 0 && len(result.Citations) > 0 {
		eval.ContextDensity = float64(len(result.Citations)) / float64(eval.ContextChars)
	}
	return eval
}
