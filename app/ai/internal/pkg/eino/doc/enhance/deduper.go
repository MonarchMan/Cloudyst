package enhance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

type DeduperConfig struct {
	PreferID bool
}

type Deduper struct {
	conf DeduperConfig
}

func NewDeduper(conf *DeduperConfig) document.Transformer {
	if conf == nil {
		conf = &DeduperConfig{PreferID: true}
	}
	return &Deduper{conf: *conf}
}

func (d *Deduper) Transform(ctx context.Context, src []*schema.Document, opts ...document.TransformerOption) ([]*schema.Document, error) {
	seen := make(map[string]struct{}, len(src))
	out := make([]*schema.Document, 0, len(src))
	for _, doc := range src {
		key := d.key(doc)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cloneDocument(doc))
	}
	return out, nil
}

func (d *Deduper) key(doc *schema.Document) string {
	if doc == nil {
		return ""
	}
	if d.conf.PreferID && doc.ID != "" {
		return "id:" + doc.ID
	}
	content := strings.Join(strings.Fields(doc.Content), " ")
	if content == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(content))
	return "content:" + hex.EncodeToString(sum[:])
}
