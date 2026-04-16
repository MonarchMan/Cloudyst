package pkg

import (
	"common/auth"
	"common/hashid"
	"common/logging"
	"context"
	"os"
	"time"
	"user/internal/conf"
	"user/internal/pkg/captcha"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/wire"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.38.0"
	"go.opentelemetry.io/otel/trace"
)

var ProviderSet = wire.NewSet(
	HashEncoderWrapper,
	LoggerWrapper,
	HmacAuthWrapper,
	WebAuthnWrapper,
	TracerProvider,
	Propagator,
	captcha.NewRedisStore,
	captcha.NewCaptchaHelper,
)

func LoggerWrapper(config *conf.Bootstrap) log.Logger {
	logger := logging.NewStdLogger(os.Stdout)
	logger = log.With(logger,
		"ts", time.Now().Format("2006-01-02 15:04:05"),
		"caller", log.DefaultCaller,
		"trace_id", tracing.TraceID(),
		"span_id", tracing.SpanID(),
	)
	logger = log.NewFilter(logger, log.FilterLevel(log.ParseLevel(config.Server.Sys.LogLevel)))
	return logger
}

func HmacAuthWrapper(bs *conf.Bootstrap) auth.Auth {
	return &auth.HMACAuth{SecretKey: []byte(bs.Server.Sys.Secret)}
}

func HashEncoderWrapper(bs *conf.Bootstrap) (hashid.Encoder, error) {
	return hashid.New(bs.Server.Sys.HashSalt)
}

func TracerProvider(bs *conf.Bootstrap) (trace.TracerProvider, error) {
	ctx := context.Background()
	// 1. 创建导出器
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(bs.Extensions.Jaeger.Addr),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	// 2. 资源信息
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(bs.Name),
			attribute.String("env", "dev"),
			attribute.String("version", bs.Version),
		),
	)
	if err != nil {
		return nil, err
	}

	// 3. 创建 TracerProvider
	tp := tracesdk.NewTracerProvider(
		// 将基于父span的采样率设置为100%
		tracesdk.WithSampler(tracesdk.ParentBased(tracesdk.TraceIDRatioBased(1.0))),
		tracesdk.WithBatcher(exporter),
		tracesdk.WithResource(res),
	)

	return tp, nil
}

func Propagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func WebAuthnWrapper(bs *conf.Bootstrap) (*webauthn.WebAuthn, error) {
	settings := bs.Extensions.Webauthn
	wConfig := &webauthn.Config{
		RPID:          settings.RpId,
		RPDisplayName: settings.RpDisplayName,
		RPOrigins:     settings.RpOrigins,
	}

	return webauthn.New(wConfig)
}
