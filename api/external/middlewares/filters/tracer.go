package filters

import (
	"net/http"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"go.opentelemetry.io/otel/trace"
)

func Trace(tracer trace.Tracer) khttp.FilterFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, span := tracer.Start(r.Context(), r.URL.Path)
			defer span.End()
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}
