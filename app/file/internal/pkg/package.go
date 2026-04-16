package pkg

import (
	"common/auth"
	"common/constants"
	"common/hashid"
	"common/logging"
	"context"
	"file/internal/biz/filemanager/lock"
	"file/internal/biz/setting"
	"file/internal/conf"
	"fmt"
	"os"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
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
	HasherWrapper,
	lock.NewMemLS,
	GeneralAuthWrapper,
	LoggerWrapper,
	TracerProvider,
	Propagator,
)

func HasherWrapper(bs *conf.Bootstrap) (hashid.Encoder, error) {
	return hashid.New(bs.Server.Sys.HashSalt)
}

func GeneralAuthWrapper(config *conf.Bootstrap, settings setting.Provider) (auth.Auth, error) {
	var secretKey string
	if config.Server.Sys.Mode == constants.MasterMode {
		secretKey = settings.SecretKey(context.Background())
	} else {
		secretKey = config.Slave.Secret
		if secretKey == "" {
			return nil, fmt.Errorf("SlaveSecret is not set, please specify it in config file")
		}
	}

	return auth.HMACAuth{
		SecretKey: []byte(secretKey),
	}, nil
}

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
