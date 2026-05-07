package enhance

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

const (
	MetaQualityValid             = "rag.quality.valid"
	MetaQualityReason            = "rag.quality.reason"
	MetaQualityDuplicateLineRate = "rag.quality.duplicate_line_rate"
	MetaQualityReplacementRate   = "rag.quality.replacement_rate"
)

type QualityGateConfig struct {
	MinChars              int
	MaxReplacementRate    float64
	MaxDuplicateLineRate  float64
	RejectInvalidDocument bool
}

type QualityGate struct {
	conf QualityGateConfig
}

func NewQualityGate(conf *QualityGateConfig) document.Transformer {
	if conf == nil {
		conf = &QualityGateConfig{}
	}
	cfg := *conf
	if cfg.MinChars <= 0 {
		cfg.MinChars = 1
	}
	if cfg.MaxReplacementRate <= 0 {
		cfg.MaxReplacementRate = 0.05
	}
	if cfg.MaxDuplicateLineRate <= 0 {
		cfg.MaxDuplicateLineRate = 0.8
	}
	return &QualityGate{conf: cfg}
}

func (g *QualityGate) Transform(ctx context.Context, src []*schema.Document, opts ...document.TransformerOption) ([]*schema.Document, error) {
	out := make([]*schema.Document, 0, len(src))
	for _, doc := range src {
		next := cloneDocument(doc)
		if next == nil {
			continue
		}
		if next.MetaData == nil {
			next.MetaData = map[string]any{}
		}
		reason := g.invalidReason(next.Content)
		valid := reason == ""
		next.MetaData[MetaQualityValid] = valid
		if !valid {
			next.MetaData[MetaQualityReason] = reason
			if g.conf.RejectInvalidDocument {
				return nil, fmt.Errorf("document %s failed quality gate: %s", next.ID, reason)
			}
		}
		out = append(out, next)
	}
	return out, nil
}

func (g *QualityGate) invalidReason(content string) string {
	content = strings.TrimSpace(content)
	if utf8.RuneCountInString(content) < g.conf.MinChars {
		return "content_too_short"
	}
	if replacementRate(content) > g.conf.MaxReplacementRate {
		return "too_many_replacement_chars"
	}
	if duplicateLineRate(content) > g.conf.MaxDuplicateLineRate {
		return "too_many_duplicate_lines"
	}
	return ""
}

func replacementRate(content string) float64 {
	total := utf8.RuneCountInString(content)
	if total == 0 {
		return 0
	}
	replacements := strings.Count(content, "\uFFFD")
	return float64(replacements) / float64(total)
}

func duplicateLineRate(content string) float64 {
	lines := strings.Split(content, "\n")
	seen := make(map[string]int, len(lines))
	total := 0
	duplicates := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		total++
		if seen[line] > 0 {
			duplicates++
		}
		seen[line]++
	}
	if total == 0 {
		return 0
	}
	return float64(duplicates) / float64(total)
}
