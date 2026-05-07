package ingestion

import (
	"ai/internal/pkg/eino/doc/enhance"

	"github.com/cloudwego/eino/components/document"
)

const EnhancerNode = "enhancer"

type EnhancementConfig struct {
	Normalizer          *enhance.NormalizerConfig
	QualityGate         *enhance.QualityGateConfig
	Deduper             *enhance.DeduperConfig
	Enricher            *enhance.EnricherConfig
	ChunkPostProcessor  *enhance.ChunkPostProcessorConfig
	EnableQualityGate   bool
	EnableChunkMetadata bool
}

func NewEnhancer(conf *EnhancementConfig) document.Transformer {
	if conf == nil {
		return enhance.NewDefaultPipeline()
	}
	nodes := []document.Transformer{
		enhance.NewNormalizer(conf.Normalizer),
	}
	if conf.EnableQualityGate {
		nodes = append(nodes, enhance.NewQualityGate(conf.QualityGate))
	}
	nodes = append(nodes, enhance.NewDeduper(conf.Deduper), enhance.NewEnricher(conf.Enricher))
	if conf.EnableChunkMetadata {
		nodes = append(nodes, enhance.NewChunkPostProcessor(conf.ChunkPostProcessor))
	}
	return enhance.NewPipeline(nodes...)
}
