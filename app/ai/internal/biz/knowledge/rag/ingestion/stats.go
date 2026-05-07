package ingestion

import (
	"ai/internal/data"
	"ai/internal/pkg/eino/doc/enhance"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/cloudwego/eino/schema"
)

const (
	documentMetaName          = "rag.document.name"
	documentMetaURL           = "rag.document.url"
	documentMetaVersion       = "rag.document.version"
	documentMetaType          = "rag.document.type"
	documentMetaSplitStrategy = "rag.document.split_strategy"
	documentMetaMaxTokens     = "rag.document.max_tokens"
)

func prepareChunkMetadata(docs []*schema.Document) {
	total := len(docs)
	cursor := 0
	for i, doc := range docs {
		if doc == nil {
			continue
		}
		if doc.MetaData == nil {
			doc.MetaData = map[string]any{}
		}
		setMetaIfAbsent(doc.MetaData, enhance.MetaChunkIndex, i)
		setMetaIfAbsent(doc.MetaData, enhance.MetaChunkTotal, total)
		setMetaIfAbsent(doc.MetaData, enhance.MetaChunkSourceHash, contentHash(doc.Content))
		setMetaIfAbsent(doc.MetaData, enhance.MetaChunkStartOffset, cursor)
		cursor += len(doc.Content)
		setMetaIfAbsent(doc.MetaData, enhance.MetaChunkEndOffset, cursor)
		if i > 0 && docs[i-1] != nil && docs[i-1].ID != "" {
			setMetaIfAbsent(doc.MetaData, enhance.MetaChunkPrevID, docs[i-1].ID)
		}
		if i+1 < total && docs[i+1] != nil && docs[i+1].ID != "" {
			setMetaIfAbsent(doc.MetaData, enhance.MetaChunkNextID, docs[i+1].ID)
		}
	}
}

func buildDocumentIndexStats(info *DocumentInfo, docs []*schema.Document, contentLen int, tokens int) *data.DocumentIndexStats {
	stats := &data.DocumentIndexStats{
		ContentLen:  contentLen,
		Tokens:      tokens,
		Chunks:      len(docs),
		ContentHash: documentContentHash(docs),
		Metadata:    documentMetadata(info, docs),
	}
	if info != nil {
		stats.ParseType = info.Type
	}
	return stats
}

func documentMetadata(info *DocumentInfo, docs []*schema.Document) map[string]any {
	meta := map[string]any{}
	if info != nil {
		if info.Name != "" {
			meta[documentMetaName] = info.Name
		}
		if info.Url != "" {
			meta[documentMetaURL] = info.Url
		}
		if info.Version != "" {
			meta[documentMetaVersion] = info.Version
		}
		if info.Type != "" {
			meta[documentMetaType] = info.Type
		}
		if info.SplitStrategy != "" {
			meta[documentMetaSplitStrategy] = string(info.SplitStrategy)
		}
		if info.MaxTokens > 0 {
			meta[documentMetaMaxTokens] = info.MaxTokens
		}
	}

	for _, doc := range docs {
		if doc == nil || doc.MetaData == nil {
			continue
		}
		copyFirstMeta(meta, doc.MetaData, enhance.MetaTitle)
		copyFirstMeta(meta, doc.MetaData, enhance.MetaSummary)
		copyFirstMeta(meta, doc.MetaData, enhance.MetaTerms)
		copyFirstMeta(meta, doc.MetaData, enhance.MetaKeywords)
		copyFirstMeta(meta, doc.MetaData, enhance.MetaContentHash)
	}
	meta[enhance.MetaChunkTotal] = len(docs)
	return meta
}

func documentContentHash(docs []*schema.Document) string {
	hash := sha256.New()
	for i, doc := range docs {
		if doc == nil {
			continue
		}
		if i > 0 {
			_, _ = hash.Write([]byte("\n"))
		}
		_, _ = hash.Write([]byte(strconv.Itoa(i)))
		_, _ = hash.Write([]byte(":"))
		_, _ = hash.Write([]byte(doc.Content))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func contentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func setMetaIfAbsent(meta map[string]any, key string, value any) {
	if _, ok := meta[key]; !ok {
		meta[key] = value
	}
}

func copyFirstMeta(dst map[string]any, src map[string]any, key string) {
	if _, ok := dst[key]; ok {
		return
	}
	if value, ok := src[key]; ok {
		dst[key] = value
	}
}
