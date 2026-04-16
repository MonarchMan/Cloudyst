package biz

import (
	"ai/internal/biz/chat"
	"ai/internal/biz/image"
	"ai/internal/biz/knowledge"
	"ai/internal/biz/knowledge/rag/ingestion"
	"ai/internal/biz/knowledge/rag/retrieval"
	"ai/internal/biz/model"

	"github.com/google/wire"
)

// ProviderSet is biz providers.
var ProviderSet = wire.NewSet(
	chat.NewChatBiz,
	knowledge.NewKnowledgeBiz,
	image.NewImageBiz,
	model.NewModelBiz,
	ingestion.NewIngestEngine,
	retrieval.NewRetrieveEngine,
	ingestion.NewMilvusIndexer,
)
