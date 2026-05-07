package enhance

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

const (
	MetaChunkIndex       = "rag.chunk.index"
	MetaChunkTotal       = "rag.chunk.total"
	MetaChunkPrevID      = "rag.chunk.prev_id"
	MetaChunkNextID      = "rag.chunk.next_id"
	MetaChunkSection     = "rag.chunk.section"
	MetaChunkSourceHash  = "rag.chunk.source_hash"
	MetaChunkStartOffset = "rag.chunk.start_offset"
	MetaChunkEndOffset   = "rag.chunk.end_offset"
)

type ChunkPostProcessorConfig struct {
	SectionMetaKeys []string
	Overwrite       bool
}

type ChunkPostProcessor struct {
	conf ChunkPostProcessorConfig
}

func NewChunkPostProcessor(conf *ChunkPostProcessorConfig) document.Transformer {
	if conf == nil {
		conf = &ChunkPostProcessorConfig{}
	}
	cfg := *conf
	if len(cfg.SectionMetaKeys) == 0 {
		cfg.SectionMetaKeys = []string{MetaTitle, "headerNameOfLevel2", "section", "title"}
	}
	return &ChunkPostProcessor{conf: cfg}
}

func (p *ChunkPostProcessor) Transform(ctx context.Context, src []*schema.Document, opts ...document.TransformerOption) ([]*schema.Document, error) {
	out := make([]*schema.Document, 0, len(src))
	total := len(src)
	for i, doc := range src {
		next := cloneDocument(doc)
		if next == nil {
			continue
		}
		if next.MetaData == nil {
			next.MetaData = map[string]any{}
		}
		if next.ID == "" {
			next.ID = fmt.Sprintf("chunk-%d", i)
		}
		setMeta(next.MetaData, MetaChunkIndex, i, p.conf.Overwrite)
		setMeta(next.MetaData, MetaChunkTotal, total, p.conf.Overwrite)
		setMeta(next.MetaData, MetaChunkSourceHash, contentHash(next.Content), p.conf.Overwrite)
		if section := p.section(next.MetaData); section != "" {
			setMeta(next.MetaData, MetaChunkSection, section, p.conf.Overwrite)
		}
		if i > 0 && src[i-1] != nil {
			setMeta(next.MetaData, MetaChunkPrevID, src[i-1].ID, p.conf.Overwrite)
		}
		if i+1 < len(src) && src[i+1] != nil {
			setMeta(next.MetaData, MetaChunkNextID, src[i+1].ID, p.conf.Overwrite)
		}
		out = append(out, next)
	}
	return out, nil
}

func (p *ChunkPostProcessor) section(meta map[string]any) string {
	for _, key := range p.conf.SectionMetaKeys {
		if value, ok := meta[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}
