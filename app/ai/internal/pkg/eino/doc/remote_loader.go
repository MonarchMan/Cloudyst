package doc

import (
	pbfile "api/api/file/files/v1"
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/document/parser/docx"
	"github.com/cloudwego/eino-ext/components/document/parser/html"
	"github.com/cloudwego/eino-ext/components/document/parser/pdf"
	"github.com/cloudwego/eino-ext/components/document/parser/xlsx"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/document/parser"
	"github.com/cloudwego/eino/schema"
)

type (
	RemoteLoader struct {
		fc   pbfile.FileClient
		conf *RemoteLoaderConfig
	}

	RemoteLoaderConfig struct {
		// optional, default: RemoteFetcher
		Fetcher Fetcher
		// optional, default: parser/html.
		Parsers map[string]parser.Parser
	}
)

// NewRemoteLoader creates a new RemoteLoader.
// reference: https://github.com/cloudwego/eino-ext/blob/main/components/document/loader/url/url.go
func NewRemoteLoader(fc pbfile.FileClient, conf *RemoteLoaderConfig) (*RemoteLoader, error) {
	if conf == nil {
		conf = &RemoteLoaderConfig{}
	}
	if conf.Parsers == nil {
		var err error
		conf.Parsers, err = initParsers()
		if err != nil {
			return nil, err
		}
	}

	return &RemoteLoader{
		fc:   fc,
		conf: conf,
	}, nil
}

func (l *RemoteLoader) Load(ctx context.Context, src document.Source, opts ...document.LoaderOption) (docs []*schema.Document, err error) {
	ctx = callbacks.EnsureRunInfo(ctx, l.GetType(), components.ComponentOfLoader)
	ctx = callbacks.OnStart(ctx, &document.LoaderCallbackInput{
		Source: src,
	})
	defer func() {
		if err != nil {
			_ = callbacks.OnError(ctx, err)
		}
	}()

	info, err := l.conf.Fetcher.Fetch(ctx, src)
	defer info.Reader.Close()

	if l.conf.Parsers == nil {
		return nil, fmt.Errorf("failed to load content from uri [%s]: %w", src.URI, err)
	}

	o := document.GetLoaderCommonOptions(&document.LoaderOptions{}, opts...)
	p := l.conf.Parsers[info.Type]

	docs, err = p.Parse(ctx, info.Reader, append([]parser.Option{parser.WithURI(src.URI),
		parser.WithExtraMeta(info.Metadata)}, o.ParserOptions...)...)
	if err != nil {
		return nil, fmt.Errorf("parse content of uri [%s] err: %w", src.URI, err)
	}

	_ = callbacks.OnEnd(ctx, &document.LoaderCallbackOutput{
		Source: src,
		Docs:   docs,
	})

	return docs, nil
}

func (l *RemoteLoader) GetType() string {
	return "RemoteLoader"
}

func (l *RemoteLoader) IsCallbacksEnabled() bool {
	return true
}

func initParsers() (map[string]parser.Parser, error) {
	ctx := context.Background()
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

	// 每种 mimeType 对应一个 Loader 实例（Parser 不同）
	// url.Loader 本身只负责下载，Parser 负责内容解析
	return map[string]parser.Parser{
		"text/html":       htmlParser, // nil = 使用默认 HTML Parser
		"application/pdf": pdfParser,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": docxParser,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":       xlsxParser,
		"text/plain":    &parser.TextParser{},
		"text/markdown": &parser.TextParser{},
	}, nil
}
