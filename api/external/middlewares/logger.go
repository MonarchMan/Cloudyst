package middlewares

import (
	"api/external/trans"
	"context"
	"net/http"
	"time"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	"google.golang.org/grpc"
)

func Logger() middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			start := time.Now()
			resp, err := next(ctx, req)
			level := log.LevelInfo
			code := http.StatusOK
			path := ""

			isStream := false
			// get transport info
			if tr, ok := transport.FromServerContext(ctx); ok {
				path = tr.Operation()
				if tr.Kind() == transport.KindGRPC {
					// 检查 context 中是否有 grpc.ServerStream
					// 在 gRPC 流式调用中，req 传给中间件通常是 nil，进一步确认是否真的有流信息
					if req == nil && grpc.ServerTransportStreamFromContext(ctx) != nil {
						isStream = true
					}
				}
			}
			if err != nil {
				log.Errorf("get request from context failed: %v", err)
			}
			var status *errors.Status = nil
			if err != nil {
				kerr := errors.FromError(err)
				level = log.LevelError
				status = &kerr.Status
				code = int(status.Code)
			} else if reply, ok := resp.(trans.Response); ok {
				code = reply.Code
			}
			if isStream {
				go func() {
					<-ctx.Done()
					log.Context(ctx).Log(level,
						"source", "accesslog",
						"path", path,
						"code", code,
						"time", time.Since(start).Seconds(),
						"isStream", isStream,
						"error", status.GetMessage(),
					)
				}()
			}
			log.Context(ctx).Log(level,
				"source", "accesslog",
				"path", path,
				"code", code,
				"time", time.Since(start).Seconds(),
				"isStream", isStream,
				"error", status.GetMessage(),
			)
			return resp, err
		}
	}
}
