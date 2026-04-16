package filters

import (
	"net/http"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// statusRecorder 用于记录响应状态码
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code // 记录状态码
	r.ResponseWriter.WriteHeader(code)
}

func Logger() khttp.FilterFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recoder := &statusRecorder{ResponseWriter: w}
			start := time.Now()
			next.ServeHTTP(recoder, r)
			level := log.LevelInfo
			code := recoder.status
			path := r.URL.Path
			log.Context(r.Context()).Log(level,
				"source", "accesslog",
				"path", path,
				"code", code,
				"time", time.Since(start).Seconds(),
			)
		})
	}
}
