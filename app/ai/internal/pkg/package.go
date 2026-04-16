package pkg

import (
	"ai/internal/conf"
	"ai/internal/pkg/eino/model"
	"ai/internal/pkg/eino/tool/factory"
	"common/hashid"
	"common/logging"
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/cloudwego/eino-ext/components/document/loader/url"
	"github.com/cloudwego/eino-ext/components/document/parser/docx"
	"github.com/cloudwego/eino-ext/components/document/parser/html"
	"github.com/cloudwego/eino-ext/components/document/parser/pdf"
	"github.com/cloudwego/eino-ext/components/document/parser/xlsx"
	"github.com/cloudwego/eino-ext/libs/acl/openai"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/document/parser"
	"github.com/cloudwego/eino/components/embedding"
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
	model.NewAiModelManager,
	factory.NewToolRegistry,
	HasherWrapper,
	LoggerWrapper,
	TracerProvider,
	Propagator,
	OllamaEmbedder,
	ExtParser,
	URLLoader,
)

func HasherWrapper(bs *conf.Bootstrap) (hashid.Encoder, error) {
	return hashid.New(bs.Server.Sys.HashSalt)
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

func OllamaEmbedder(bs *conf.Bootstrap) (embedding.Embedder, error) {
	embedderCfg := bs.Server.Sys.Embedder
	if embedderCfg.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	if embedderCfg.BaseUrl == "" {
		embedderCfg.BaseUrl = "http://localhost:11434/v1"
	}
	var dimensions int
	if embedderCfg.Dimensions > 0 {
		dimensions = int(embedderCfg.Dimensions)
	}
	cfg := &openai.EmbeddingConfig{
		Model:      embedderCfg.Model,
		BaseURL:    embedderCfg.BaseUrl,
		APIKey:     embedderCfg.ApiKey,
		Dimensions: &dimensions,
		HTTPClient: http.DefaultClient,
	}
	embedder, err := openai.NewEmbeddingClient(context.Background(), cfg)
	if err != nil {
		return nil, err
	}
	return embedder, nil
}

func ExtParser() (parser.Parser, error) {
	ctx := context.Background()
	textParser := &parser.TextParser{}
	// PDF
	pdfParser, err := pdf.NewPDFParser(ctx, &pdf.Config{})
	if err != nil {
		return nil, fmt.Errorf("init pdf parser failed: %w", err)
	}

	// Docx
	docxParser, err := docx.NewDocxParser(ctx, &docx.Config{})
	if err != nil {
		return nil, fmt.Errorf("init docx parser failed: %w", err)
	}

	// Xlsx
	xlsxParser, err := xlsx.NewXlsxParser(ctx, &xlsx.Config{})
	if err != nil {
		return nil, fmt.Errorf("init xlsx parser failed: %w", err)
	}

	// HTML
	htmlParser, err := html.NewParser(ctx, &html.Config{})
	if err != nil {
		return nil, fmt.Errorf("init html parser failed: %w", err)
	}

	// 创建扩展解析器
	extParser, err := parser.NewExtParser(ctx, &parser.ExtParserConfig{
		// 注册特定扩展名的解析器
		Parsers: map[string]parser.Parser{
			".html": htmlParser,
			".pdf":  pdfParser,
			".docx": docxParser,
			".xlsx": xlsxParser,
		},
		// 设置默认解析器，用于处理未知格式
		FallbackParser: textParser,
	})

	return extParser, err
}

func URLLoader(parser parser.Parser) (document.Loader, error) {
	return url.NewLoader(context.Background(), &url.LoaderConfig{
		Parser: parser,
	})
}
