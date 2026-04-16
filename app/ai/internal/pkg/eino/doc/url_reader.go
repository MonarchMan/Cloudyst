package doc

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/document/loader/url"
	"github.com/cloudwego/eino-ext/components/document/parser/docx"
	"github.com/cloudwego/eino-ext/components/document/parser/html"
	"github.com/cloudwego/eino-ext/components/document/parser/pdf"
	"github.com/cloudwego/eino-ext/components/document/parser/xlsx"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/document/parser"
)

type (
	UrlReader interface {
		ReadURL(ctx context.Context, url string) (string, error)
	}
	// urlReader 用 eino url.Loader 实现 UrlLoader
	// 对应 Java 的 readUrl：下载 + TikaDocumentReader 解析
	urlReader struct {
		loaders map[string]*url.Loader // mimeType → 对应 Loader（含对应 Parser）
	}
)

func NewURLReader() (UrlReader, error) {
	r := &urlReader{
		loaders: make(map[string]*url.Loader),
	}
	err := r.init()

	return r, err
}

func (r *urlReader) init() error {
	ctx := context.Background()
	httpClient := &http.Client{Timeout: 30 * time.Second}
	// PDF
	pdfParser, err := pdf.NewPDFParser(ctx, &pdf.Config{})
	if err != nil {
		return fmt.Errorf("init pdf parser failed: %w", err)
	}

	// Docx
	docxParser, err := docx.NewDocxParser(ctx, &docx.Config{})
	if err != nil {
		return fmt.Errorf("init docx parser failed: %w", err)
	}

	// Xlsx
	xlsxParser, err := xlsx.NewXlsxParser(ctx, &xlsx.Config{})
	if err != nil {
		return fmt.Errorf("init xlsx parser failed: %w", err)
	}

	// HTML
	htmlParser, err := html.NewParser(ctx, &html.Config{})
	if err != nil {
		return fmt.Errorf("init html parser failed: %w", err)
	}

	// 每种 mimeType 对应一个 Loader 实例（Parser 不同）
	// url.Loader 本身只负责下载，Parser 负责内容解析
	loaderConfigs := map[string]parser.Parser{
		"text/html":       htmlParser, // nil = 使用默认 HTML Parser
		"application/pdf": pdfParser,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": docxParser,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":       xlsxParser,
		"text/plain":    &parser.TextParser{},
		"text/markdown": &parser.TextParser{},
	}

	for mimeType, p := range loaderConfigs {
		cfg := &url.LoaderConfig{
			Client: httpClient,
			Parser: p,
		}
		loader, err := url.NewLoader(ctx, cfg)
		if err != nil {
			return fmt.Errorf("init url loader for %s: %w", mimeType, err)
		}
		r.loaders[mimeType] = loader
	}
	return nil
}

// ReadURL 对应 Java 的 readUrl 方法
func (r *urlReader) ReadURL(ctx context.Context, rawURL string) (string, error) {
	// 1. 推断 MIME 类型
	mimeType := mime.TypeByExtension(filepath.Ext(rawURL))

	// 2. 选择对应的 Loader
	loader, ok := r.loaders[mimeType]
	if !ok {
		return "", fmt.Errorf("[readUrl][url(%s) 不支持的文件类型: %s]", rawURL, mimeType)
	}

	// 3. 下载并解析
	docs, err := loader.Load(ctx, document.Source{URI: rawURL})
	if err != nil {
		return "", fmt.Errorf("[readUrl][url(%s) 读取失败]: %w", rawURL, err)
	}

	// 4. 取第一个文档
	if len(docs) == 0 || strings.TrimSpace(docs[0].Content) == "" {
		return "", fmt.Errorf("[readUrl][url(%s) 文件内容为空]", rawURL)
	}

	// 多页文档（如 PDF）合并所有页内容
	var sb strings.Builder
	for _, doc := range docs {
		sb.WriteString(doc.Content)
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String()), nil
}
