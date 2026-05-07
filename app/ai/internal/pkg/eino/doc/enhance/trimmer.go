package enhance

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

const MetaTrimmed = "rag.trimmed"

type TrimmerConfig struct {
	MaxChars  int
	HeadChars int
	TailChars int
}

type Trimmer struct {
	conf TrimmerConfig
}

func NewTrimmer(conf *TrimmerConfig) (document.Transformer, error) {
	if conf == nil || conf.MaxChars <= 0 {
		return nil, fmt.Errorf("trimmer max chars must be positive")
	}
	cfg := *conf
	if cfg.HeadChars < 0 || cfg.TailChars < 0 {
		return nil, fmt.Errorf("trimmer head and tail chars cannot be negative")
	}
	if cfg.HeadChars+cfg.TailChars > cfg.MaxChars {
		return nil, fmt.Errorf("trimmer head and tail chars exceed max chars")
	}
	return &Trimmer{conf: cfg}, nil
}

func (t *Trimmer) Transform(ctx context.Context, src []*schema.Document, opts ...document.TransformerOption) ([]*schema.Document, error) {
	out := make([]*schema.Document, 0, len(src))
	for _, doc := range src {
		next := cloneDocument(doc)
		if next == nil {
			continue
		}
		if utf8.RuneCountInString(next.Content) > t.conf.MaxChars {
			next.Content = t.trim(next.Content)
			if next.MetaData == nil {
				next.MetaData = map[string]any{}
			}
			next.MetaData[MetaTrimmed] = true
		}
		out = append(out, next)
	}
	return out, nil
}

func (t *Trimmer) trim(content string) string {
	runes := []rune(content)
	if len(runes) <= t.conf.MaxChars {
		return content
	}
	if t.conf.HeadChars == 0 && t.conf.TailChars == 0 {
		return strings.TrimSpace(string(runes[:t.conf.MaxChars]))
	}

	head := t.conf.HeadChars
	tail := t.conf.TailChars
	if head == 0 {
		head = t.conf.MaxChars - tail
	}
	if tail == 0 {
		tail = t.conf.MaxChars - head
	}
	ellipsis := "\n...\n"
	ellipsisLen := utf8.RuneCountInString(ellipsis)
	if head+tail+ellipsisLen > t.conf.MaxChars {
		tail = t.conf.MaxChars - head - ellipsisLen
	}
	if tail <= 0 {
		return strings.TrimSpace(string(runes[:t.conf.MaxChars]))
	}
	return strings.TrimSpace(string(runes[:head])) + ellipsis + strings.TrimSpace(string(runes[len(runes)-tail:]))
}
