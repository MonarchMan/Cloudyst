package graphrag

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
)

type ContextCompressor interface {
	Compress(ctx context.Context, query string, docs []*schema.Document) ([]*schema.Document, error)
}

type ContextCompressFunc func(ctx context.Context, query string, docs []*schema.Document) ([]*schema.Document, error)

func (f ContextCompressFunc) Compress(ctx context.Context, query string, docs []*schema.Document) ([]*schema.Document, error) {
	return f(ctx, query, docs)
}

type NoopContextCompressor struct{}

func (NoopContextCompressor) Compress(ctx context.Context, query string, docs []*schema.Document) ([]*schema.Document, error) {
	return docs, nil
}

type KeywordContextCompressorConfig struct {
	MaxCharsPerDocument int
	MinKeywordHits      int
}

type KeywordContextCompressor struct {
	conf KeywordContextCompressorConfig
}

func NewKeywordContextCompressor(conf *KeywordContextCompressorConfig) *KeywordContextCompressor {
	if conf == nil {
		conf = &KeywordContextCompressorConfig{}
	}
	cfg := *conf
	if cfg.MaxCharsPerDocument <= 0 {
		cfg.MaxCharsPerDocument = 800
	}
	if cfg.MinKeywordHits <= 0 {
		cfg.MinKeywordHits = 1
	}
	return &KeywordContextCompressor{conf: cfg}
}

func (c *KeywordContextCompressor) Compress(ctx context.Context, query string, docs []*schema.Document) ([]*schema.Document, error) {
	keywords := queryKeywords(query)
	if len(keywords) == 0 {
		return docs, nil
	}
	out := make([]*schema.Document, 0, len(docs))
	for _, doc := range docs {
		next := cloneDocument(doc)
		if next == nil {
			continue
		}
		next.Content = c.compressContent(next.Content, keywords)
		out = append(out, next)
	}
	return out, nil
}

func (c *KeywordContextCompressor) compressContent(content string, keywords []string) string {
	if utf8.RuneCountInString(content) <= c.conf.MaxCharsPerDocument {
		return content
	}
	sentences := splitContextSentences(content)
	selected := make([]string, 0, len(sentences))
	for _, sentence := range sentences {
		hits := keywordHits(sentence, keywords)
		if hits >= c.conf.MinKeywordHits {
			selected = append(selected, strings.TrimSpace(sentence))
		}
	}
	if len(selected) == 0 {
		return truncate(content, c.conf.MaxCharsPerDocument)
	}
	compressed := strings.Join(selected, " ")
	if utf8.RuneCountInString(compressed) > c.conf.MaxCharsPerDocument {
		return truncate(compressed, c.conf.MaxCharsPerDocument)
	}
	return compressed
}

func queryKeywords(query string) []string {
	parts := regexp.MustCompile(`[A-Za-z0-9_+\-./#\p{Han}]+`).FindAllString(strings.ToLower(query), -1)
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		if utf8.RuneCountInString(part) < 2 {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	return out
}

func splitContextSentences(content string) []string {
	return regexp.MustCompile(`[^.!?\x{3002}\x{FF01}\x{FF1F}\x{FF1B};]+[.!?\x{3002}\x{FF01}\x{FF1F}\x{FF1B};]?`).FindAllString(content, -1)
}

func keywordHits(content string, keywords []string) int {
	content = strings.ToLower(content)
	hits := 0
	for _, keyword := range keywords {
		if strings.Contains(content, keyword) {
			hits++
		}
	}
	return hits
}
