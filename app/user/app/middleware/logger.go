package middleware

import (
	"api/external/trans"
	"context"
	"net/http"
	"time"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
)

func Logger() middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			start := time.Now()
			resp, err := next(ctx, req)
			level := log.LevelInfo
			code := http.StatusOK
			path := ""
			var status *errors.Status = nil
			if err != nil {
				kerr := errors.FromError(err)
				level = log.LevelError
				status = &kerr.Status
				code = int(status.Code)
			} else if reply, ok := resp.(trans.Response); ok {
				code = reply.Code
			}

			// get transport info
			if tr, ok := transport.FromServerContext(ctx); ok {
				path = tr.Operation()
			}
			log.Context(ctx).Log(level,
				"source", "accesslog",
				"path", path,
				"code", code,
				"time", time.Since(start).Seconds(),
				"error", status.GetMessage(),
			)
			return resp, err
		}
	}
}
