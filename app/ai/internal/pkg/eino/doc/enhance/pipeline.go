package enhance

import (
	"context"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

type Pipeline struct {
	transformers []document.Transformer
}

func NewPipeline(transformers ...document.Transformer) document.Transformer {
	return &Pipeline{transformers: transformers}
}

func NewDefaultPipeline() document.Transformer {
	return NewPipeline(
		NewNormalizer(&NormalizerConfig{
			TrimSpace:          true,
			CollapseBlankLines: true,
			NormalizeTabs:      true,
			TrackOriginalChars: true,
		}),
		NewQualityGate(nil),
		NewDeduper(nil),
		NewEnricher(nil),
	)
}

func (p *Pipeline) Transform(ctx context.Context, src []*schema.Document, opts ...document.TransformerOption) ([]*schema.Document, error) {
	docs := src
	var err error
	for _, transformer := range p.transformers {
		if transformer == nil {
			continue
		}
		docs, err = transformer.Transform(ctx, docs, opts...)
		if err != nil {
			return nil, err
		}
	}
	return docs, nil
}
