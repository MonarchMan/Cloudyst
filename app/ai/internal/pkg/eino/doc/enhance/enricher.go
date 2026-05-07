package enhance

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

type EnricherConfig struct {
	SummaryMaxChars int
	MaxTerms        int
	MinTermLength   int
	Override        bool
	StopWords       []string
}

type Enricher struct {
	conf      EnricherConfig
	stopWords map[string]struct{}
}

func NewEnricher(conf *EnricherConfig) document.Transformer {
	if conf == nil {
		conf = &EnricherConfig{}
	}
	cfg := *conf
	if cfg.SummaryMaxChars <= 0 {
		cfg.SummaryMaxChars = 280
	}
	if cfg.MaxTerms <= 0 {
		cfg.MaxTerms = 12
	}
	if cfg.MinTermLength <= 0 {
		cfg.MinTermLength = 3
	}

	stopWords := defaultStopWords()
	for _, word := range cfg.StopWords {
		stopWords[strings.ToLower(strings.TrimSpace(word))] = struct{}{}
	}

	return &Enricher{
		conf:      cfg,
		stopWords: stopWords,
	}
}

func (e *Enricher) Transform(ctx context.Context, src []*schema.Document, opts ...document.TransformerOption) ([]*schema.Document, error) {
	out := make([]*schema.Document, 0, len(src))
	for _, doc := range src {
		next := cloneDocument(doc)
		if next == nil {
			continue
		}
		if next.MetaData == nil {
			next.MetaData = map[string]any{}
		}

		terms := e.extractTerms(next.Content)
		setMeta(next.MetaData, MetaTitle, extractTitle(next.Content), e.conf.Override)
		setMeta(next.MetaData, MetaSummary, summarize(next.Content, e.conf.SummaryMaxChars), e.conf.Override)
		setMeta(next.MetaData, MetaTerms, terms, e.conf.Override)
		setMeta(next.MetaData, MetaKeywords, terms, e.conf.Override)
		setMeta(next.MetaData, MetaContentHash, contentHash(next.Content), e.conf.Override)
		setMeta(next.MetaData, MetaCharCount, utf8.RuneCountInString(next.Content), e.conf.Override)
		setMeta(next.MetaData, MetaWordCount, len(strings.Fields(next.Content)), e.conf.Override)
		setMeta(next.MetaData, MetaLineCount, countLines(next.Content), e.conf.Override)

		out = append(out, next)
	}
	return out, nil
}

func extractTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			line = strings.TrimLeft(line, "#")
			line = strings.TrimSpace(line)
		}
		return truncateRunes(line, 120)
	}
	return ""
}

func summarize(content string, maxChars int) string {
	content = strings.Join(strings.Fields(content), " ")
	if maxChars <= 0 || utf8.RuneCountInString(content) <= maxChars {
		return content
	}

	var b strings.Builder
	for _, sentence := range splitSentences(content) {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}
		nextLen := utf8.RuneCountInString(b.String()) + utf8.RuneCountInString(sentence)
		if b.Len() > 0 {
			nextLen += 1
		}
		if nextLen > maxChars {
			break
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(sentence)
	}

	if b.Len() == 0 {
		return truncateRunes(content, maxChars)
	}
	return b.String()
}

func splitSentences(content string) []string {
	re := regexp.MustCompile(`[^.!?\x{3002}\x{FF01}\x{FF1F}\x{FF1B};]+[.!?\x{3002}\x{FF01}\x{FF1F}\x{FF1B};]?`)
	return re.FindAllString(content, -1)
}

type termStat struct {
	term  string
	count int
}

func (e *Enricher) extractTerms(content string) []string {
	re := regexp.MustCompile(`[A-Za-z][A-Za-z0-9_+\-./#]{2,}|[\p{Han}]{2,}`)
	matches := re.FindAllString(content, -1)

	stats := make(map[string]*termStat)
	for _, raw := range matches {
		term := strings.Trim(raw, " \t\n\r.,;:!?()[]{}<>\"'")
		if utf8.RuneCountInString(term) < e.conf.MinTermLength {
			continue
		}
		key := strings.ToLower(term)
		if _, blocked := e.stopWords[key]; blocked {
			continue
		}
		if !looksLikeTerm(term) {
			continue
		}
		if stat, ok := stats[key]; ok {
			stat.count++
			continue
		}
		stats[key] = &termStat{term: term, count: 1}
	}

	ordered := make([]termStat, 0, len(stats))
	for _, stat := range stats {
		ordered = append(ordered, *stat)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].count == ordered[j].count {
			return ordered[i].term < ordered[j].term
		}
		return ordered[i].count > ordered[j].count
	})

	limit := e.conf.MaxTerms
	if len(ordered) < limit {
		limit = len(ordered)
	}
	terms := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		terms = append(terms, ordered[i].term)
	}
	return terms
}

func looksLikeTerm(term string) bool {
	if regexp.MustCompile(`[\p{Han}]`).MatchString(term) {
		return true
	}
	if regexp.MustCompile(`[A-Z0-9_+\-./#]`).MatchString(term) {
		return true
	}
	return utf8.RuneCountInString(term) >= 8
}

func truncateRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:max]))
}

func defaultStopWords() map[string]struct{} {
	words := []string{
		"about", "after", "before", "between", "could", "document", "example",
		"from", "into", "should", "system", "their", "there", "these", "this",
		"through", "using", "where", "which", "while", "with", "without",
	}
	out := make(map[string]struct{}, len(words))
	for _, word := range words {
		out[word] = struct{}{}
	}
	return out
}
