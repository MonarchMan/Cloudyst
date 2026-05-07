package enhance

import (
	"context"
	"strings"
	"unicode"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

type NormalizerConfig struct {
	TrimSpace          bool
	CollapseBlankLines bool
	MaxBlankLines      int
	NormalizeTabs      bool
	TrackOriginalChars bool
}

type Normalizer struct {
	conf NormalizerConfig
}

func NewNormalizer(conf *NormalizerConfig) document.Transformer {
	if conf == nil {
		conf = &NormalizerConfig{}
	}
	cfg := *conf
	if cfg.MaxBlankLines <= 0 {
		cfg.MaxBlankLines = 1
	}
	return &Normalizer{conf: cfg}
}

func (n *Normalizer) Transform(ctx context.Context, src []*schema.Document, opts ...document.TransformerOption) ([]*schema.Document, error) {
	out := make([]*schema.Document, 0, len(src))
	for _, doc := range src {
		next := cloneDocument(doc)
		if next == nil {
			continue
		}
		originalLen := len([]rune(next.Content))
		next.Content = n.normalize(next.Content)
		if n.conf.TrackOriginalChars {
			setMeta(next.MetaData, MetaOriginalChars, originalLen, false)
		}
		out = append(out, next)
	}
	return out, nil
}

func (n *Normalizer) normalize(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	if n.conf.NormalizeTabs {
		content = strings.ReplaceAll(content, "\t", " ")
	}

	lines := strings.Split(content, "\n")
	normalized := make([]string, 0, len(lines))
	blankCount := 0
	for _, line := range lines {
		if n.conf.TrimSpace {
			line = strings.TrimFunc(line, unicode.IsSpace)
		}
		if line == "" {
			blankCount++
			if n.conf.CollapseBlankLines && blankCount > n.conf.MaxBlankLines {
				continue
			}
		} else {
			blankCount = 0
		}
		normalized = append(normalized, line)
	}

	content = strings.Join(normalized, "\n")
	if n.conf.TrimSpace {
		content = strings.TrimSpace(content)
	}
	return content
}
