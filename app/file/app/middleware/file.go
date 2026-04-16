package middleware

import (
	"context"
	"file/internal/biz/filemanager/fs/dbfs"
	"file/internal/pkg/utils"

	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/gofrs/uuid"
)

func ParseHintToContext() middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			reqHeader, _, err := utils.HeaderFromContext(ctx)
			if err != nil {
				return nil, err
			}
			if reqHeader.Get(dbfs.ContextHintHeader) != "" {
				ctx = context.WithValue(ctx, dbfs.ContextHintCtxKey{}, uuid.FromStringOrNil(reqHeader.Get(dbfs.ContextHintHeader)))
			}
			return next(ctx, req)
		}
	}
}
