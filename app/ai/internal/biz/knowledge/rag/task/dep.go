package task

import (
	"ai/internal/biz/knowledge/rag/ingestion"
	"ai/internal/data"
	"ai/internal/data/rpc"
	"ai/internal/data/vector"
	"context"
)

type IngestDepCtx struct{}
type IngestDep struct {
	Engine         *ingestion.IngestEngine
	DocumentClient data.KnowledgeDocumentClient
	VectorStore    vector.VectorStore
	FileClient     rpc.FileClient
}

func IngestEngineFromContext(ctx context.Context) *IngestDep {
	return ctx.Value(IngestDepCtx{}).(*IngestDep)
}
