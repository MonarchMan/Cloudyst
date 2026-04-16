package factory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type ToolHTTPConfig struct {
	ID         int64             `json:"id"`
	ToolID     int64             `json:"tool_id"`
	Endpoint   string            `json:"endpoint"`
	Method     string            `json:"method"`
	Headers    map[string]string `json:"headers"`
	TimeoutMs  int               `json:"timeout_ms"`
	RetryTimes int               `json:"retry_times"`
}

type Config struct {
	Endpoint   string
	Method     string
	Headers    map[string]string
	TimeoutMs  int
	RetryTimes int
}

type httpTool struct {
	info   *schema.ToolInfo
	config *Config
	client *http.Client
}

func NewHTTPTool(info *schema.ToolInfo, config *Config) (tool.InvokableTool, error) {
	if info == nil || info.Name == "" {
		return nil, fmt.Errorf("tool info and name are required")
	}
	if config.Endpoint == "" || config.Method == "" {
		return nil, fmt.Errorf("endpoint and method are required")
	}

	timeout := time.Duration(config.TimeoutMs) * time.Millisecond
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	return &httpTool{
		info:   info,
		config: config,
		client: &http.Client{Timeout: timeout},
	}, nil
}

// Info 返回 Tool 元数据，ChatModel.BindTools 从这里取
func (t *httpTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return t.info, nil
}

// InvokableRun 是 LLM 决定调用时 ToolsNode 实际执行的入口
// args 是 LLM 按照 ToolInfo.Parameters 填入的参数，JSON 字符串形式
func (t *httpTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	// 1. 解析 LLM 传入的参数
	var params map[string]any
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("parse args failed: %w", err)
	}

	// 2. 构造 HTTP 请求
	req, err := t.buildRequest(ctx, params)
	if err != nil {
		return "", fmt.Errorf("build request failed: %w", err)
	}

	// 3. 执行，支持重试
	resp, err := t.doWithRetry(req)
	if err != nil {
		return "", fmt.Errorf("http request failed: %w", err)
	}

	return resp, nil
}

func (t *httpTool) buildRequest(ctx context.Context, params map[string]any) (*http.Request, error) {
	var (
		req *http.Request
		err error
	)

	switch t.config.Method {
	case http.MethodGet:
		// GET 参数拼接到 URL query string
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, t.config.Endpoint, nil)
		if err != nil {
			return nil, err
		}
		q := req.URL.Query()
		for k, v := range params {
			q.Set(k, fmt.Sprintf("%v", v))
		}
		req.URL.RawQuery = q.Encode()

	case http.MethodPost, http.MethodPut, http.MethodDelete:
		// 其他方法参数放 body
		body, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		req, err = http.NewRequestWithContext(ctx, t.config.Method, t.config.Endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

	default:
		return nil, fmt.Errorf("unsupported method: %s", t.config.Method)
	}

	// 注入静态 Header（如鉴权信息）
	for k, v := range t.config.Headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

func (t *httpTool) doWithRetry(req *http.Request) (string, error) {
	var (
		lastErr  error
		attempts = t.config.RetryTimes + 1
	)

	for i := 0; i < attempts; i++ {
		resp, err := t.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("http status %d: %s", resp.StatusCode, string(body))
			continue
		}

		return string(body), nil
	}

	return "", fmt.Errorf("all %d attempts failed, last error: %w", attempts, lastErr)
}
