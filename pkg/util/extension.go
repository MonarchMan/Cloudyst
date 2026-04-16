package util

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

func TraceID(ctx context.Context) string {
	if span := trace.SpanFromContext(ctx); span != nil {
		return span.SpanContext().TraceID().String()
	}
	return ""
}
