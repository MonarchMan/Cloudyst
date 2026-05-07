package enhance

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/cloudwego/eino/schema"
)

const (
	MetaTitle         = "rag.title"
	MetaSummary       = "rag.summary"
	MetaTerms         = "rag.terms"
	MetaKeywords      = "rag.keywords"
	MetaContentHash   = "rag.content_hash"
	MetaCharCount     = "rag.char_count"
	MetaWordCount     = "rag.word_count"
	MetaLineCount     = "rag.line_count"
	MetaOriginalChars = "rag.original_char_count"
)

func cloneDocument(doc *schema.Document) *schema.Document {
	if doc == nil {
		return nil
	}
	next := &schema.Document{
		ID:       doc.ID,
		Content:  doc.Content,
		MetaData: cloneMeta(doc.MetaData),
	}
	return next
}

func cloneMeta(meta map[string]any) map[string]any {
	next := make(map[string]any, len(meta))
	for k, v := range meta {
		next[k] = v
	}
	return next
}

func setMeta(meta map[string]any, key string, value any, override bool) {
	if override {
		meta[key] = value
		return
	}
	if _, ok := meta[key]; !ok {
		meta[key] = value
	}
}

func contentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func countLines(content string) int {
	if content == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}
