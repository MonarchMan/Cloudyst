package ingestion

import (
	"ai/internal/biz/types"
	"ai/internal/data"
	"ai/internal/data/rpc"
	"ai/internal/pkg/eino/doc"
	"ai/internal/pkg/utils"
	"common/request"
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

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
		kdc  data.KnowledgeDocumentClient
		conf *RemoteLoaderConfig
		fc   rpc.FileClient
	}

	RemoteLoaderConfig struct {
		// optional, default: defaultParser.
		Parser parser.Parser
		// optional, default: request.NewClient(constants.MasterMode)
		Client request.Client
	}
)

func NewRemoteLoader(kdc data.KnowledgeDocumentClient, fc rpc.FileClient, conf *RemoteLoaderConfig) (*RemoteLoader, error) {
	if conf == nil {
		conf = &RemoteLoaderConfig{}
	}
	if conf.Parser == nil {
		var err error
		conf.Parser, err = defaultParser()
		if err != nil {
			return nil, err
		}
	}

	return &RemoteLoader{
		kdc:  kdc,
		conf: conf,
		fc:   fc,
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

	// 1. 下载文件
	fileInfo, changed, err := l.fetch(ctx, src)
	if err != nil {
		return nil, fmt.Errorf("failed to load content from uri [%s]: %w", src.URI, err)
	}
	defer fileInfo.Reader.Close()

	// 2. 更新文档信息
	documentInfo := GetDocumentInfo(ctx)
	version := ""
	if changed {
		version = documentInfo.Version
	}
	_, err = l.kdc.UpdateContentLenVersionAndProcess(ctx, documentInfo.ID, int(fileInfo.Size), version, types.DocumentProcessing)
	if err != nil {
		return nil, err
	}

	o := document.GetLoaderCommonOptions(&document.LoaderOptions{}, opts...)

	docs, err = l.conf.Parser.Parse(ctx, fileInfo.Reader, append([]parser.Option{parser.WithURI(src.URI),
		parser.WithExtraMeta(fileInfo.Metadata)}, o.ParserOptions...)...)
	if err != nil {
		return nil, fmt.Errorf("parse content of uri [%s] err: %w", src.URI, err)
	}
	// 3. 检测文档切分策略
	documentInfo.SplitStrategy = detectSplitStrategy(docs, src.URI)

	_ = callbacks.OnEnd(ctx, &document.LoaderCallbackOutput{
		Source: src,
		Docs:   docs,
	})

	return docs, nil
}

// fetch file content from uri
// if uri is multiple urls, it should be separated by ";"
func (l *RemoteLoader) fetch(ctx context.Context, src document.Source) (info *doc.FileInfo, changed bool, err error) {
	// 1. 获取文件信息
	documentInfo := GetDocumentInfo(ctx)
	fileInfo, err := l.fc.GetFileInfo(ctx, documentInfo.Url)
	if err != nil {
		return nil, false, err
	}
	info = &doc.FileInfo{
		Type:     filepath.Ext(fileInfo.Name),
		Metadata: utils.StringMapToAnyMap(fileInfo.Metadata),
		Size:     fileInfo.Size,
	}

	// 2. 检查文件版本是否匹配
	changed = false
	if documentInfo.Version == "" {
		documentInfo.Version = fileInfo.PrimaryEntity
		changed = true
	} else if documentInfo.Version != fileInfo.PrimaryEntity {
		return nil, true, fmt.Errorf("file version not match: %s != %s", documentInfo.Version, fileInfo.PrimaryEntity)
	}

	// 3. 下载文件内容
	response := l.conf.Client.Request(http.MethodGet, src.URI, nil, request.WithContext(ctx)).
		CheckHTTPResponse(http.StatusOK)
	if response.Err != nil {
		return nil, changed, response.Err
	}
	info.Reader = response.Response.Body
	return info, changed, nil
}

func (l *RemoteLoader) GetType() string {
	return "RemoteLoader"
}

func (l *RemoteLoader) IsCallbacksEnabled() bool {
	return true
}

// detectSplitStrategy detect split strategy for document(temporarily ignore semantic strategy)
// if document is markdown, it will detect if it has multiple level 2 headers
// if document is not markdown, it will use sentence split strategy
func detectSplitStrategy(docs []*schema.Document, url string) types.Strategy {
	if len(docs) == 0 || docs[0].Content == "" {
		return types.StrategyParagraph
	}
	ext := filepath.Ext(url)
	h2Count := 0
	totalLines := 0
	if ext == ".md" {
		for _, d := range docs {
			lines := strings.Lines(d.Content)
			for line := range lines {
				if strings.HasPrefix(line, "##") {
					h2Count++
				}
				totalLines++
			}
		}
		if h2Count > 1 && float64(h2Count/totalLines) > 0.1 {
			return types.StrategyMarkdownHeader
		}

		return types.StrategySentence
	}
	return types.StrategySentence
}

func defaultParser() (parser.Parser, error) {
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
