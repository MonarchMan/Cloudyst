package ingestion

import (
	biztypes "ai/internal/biz/types"
	"context"
)

type (
	DocumentInfoKey struct{}
	DocumentInfo    struct {
		KnowledgeID   int
		Name          string
		Version       string // 文件版本(entity id)
		Url           string // 文件的uri，非下载链接
		Type          string // 文件类型
		ID            int
		SplitStrategy biztypes.Strategy
		MaxTokens     int
	}
)

func WithDocumentInfo(ctx context.Context, docInfo *DocumentInfo) context.Context {
	return context.WithValue(ctx, DocumentInfoKey{}, docInfo)
}

func GetDocumentInfo(ctx context.Context) *DocumentInfo {
	info, _ := ctx.Value(DocumentInfoKey{}).(*DocumentInfo)
	return info
}
