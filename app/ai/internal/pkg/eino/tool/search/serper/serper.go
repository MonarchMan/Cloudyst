package serper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

const (
	defaultEndpoint = "https://google.serper.dev/search"
	defaultNum      = 10
	defaultTimeout  = 10 * time.Second
	defaultToolName = "serper_web_search"
	defaultToolDesc = "Search Google via Serper.dev and return structured results including organic results, answer box, and knowledge graph."
)

// Config Serper 搜索工具配置
type Config struct {
	// APIKey Serper API Key，必填，请求头 X-API-KEY
	APIKey string `json:"api_key"`
	// Endpoint 自定义接口地址，默认 https://google.serper.dev/search
	Endpoint string `json:"endpoint"`
	// Num 默认返回条数，默认 10；可被 SearchRequest.Num 覆盖
	Num int `json:"num"`
	// GL 国家代码（Google country parameter），如 "us"、"cn"；可被 SearchRequest.GL 覆盖
	GL string `json:"gl"`
	// HL 语言代码（Google UI language），如 "en"、"zh-cn"；可被 SearchRequest.HL 覆盖
	HL string `json:"hl"`

	// ToolName 工具名称，默认 "serper_web_search"
	ToolName string `json:"tool_name"`
	// ToolDesc 工具描述
	ToolDesc string `json:"tool_desc"`

	// HTTPClient 自定义 HTTP 客户端，可选
	HTTPClient *http.Client `json:"-"`
}

func (c *Config) withDefaults() {
	if c.ToolName == "" {
		c.ToolName = defaultToolName
	}
	if c.ToolDesc == "" {
		c.ToolDesc = defaultToolDesc
	}
	if c.Num <= 0 {
		c.Num = defaultNum
	}
	if c.Endpoint == "" {
		c.Endpoint = defaultEndpoint
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: defaultTimeout}
	}
}

// ---- 对外暴露的请求 / 响应类型 ----

// SearchRequest 是暴露给 LLM Function Call 的入参 schema
type SearchRequest struct {
	// Query 搜索词或自然语言问题
	Query string `json:"query" jsonschema_description:"search query string sent to Google via Serper"`
	// Num 返回结果数，默认使用 Config.Num
	Num int `json:"num,omitempty" jsonschema_description:"number of results to return, default 10"`
	// GL 国家代码，如 us、cn、jp，影响结果的地域偏好
	GL string `json:"gl,omitempty" jsonschema_description:"country code for search results, e.g. us, cn, jp"`
	// HL 界面语言代码，如 en、zh-cn
	HL string `json:"hl,omitempty" jsonschema_description:"language code for search interface, e.g. en, zh-cn"`
}

// SearchResult 返回给 LLM 的简化结构
type SearchResult struct {
	Query          string          `json:"query"`
	AnswerBox      *AnswerBox      `json:"answer_box,omitempty"`
	KnowledgeGraph *KnowledgeGraph `json:"knowledge_graph,omitempty"`
	Organic        []*OrganicItem  `json:"organic"`
}

// AnswerBox Google 直接答案框
type AnswerBox struct {
	Title   string `json:"title,omitempty"`
	Answer  string `json:"answer,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

// KnowledgeGraph 知识图谱卡片
type KnowledgeGraph struct {
	Title       string `json:"title,omitempty"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	Website     string `json:"website,omitempty"`
}

// OrganicItem 单条自然搜索结果
type OrganicItem struct {
	Title    string `json:"title,omitempty"`
	Link     string `json:"link"`
	Snippet  string `json:"snippet,omitempty"`
	Date     string `json:"date,omitempty"`
	Position int    `json:"position,omitempty"`
}

// ---- 内部 HTTP 请求 / 响应结构 ----

type serperRequest struct {
	Q   string `json:"q"`
	Num int    `json:"num,omitempty"`
	GL  string `json:"gl,omitempty"`
	HL  string `json:"hl,omitempty"`
}

type serperResponse struct {
	SearchParameters struct {
		Q string `json:"q"`
	} `json:"searchParameters"`
	AnswerBox *struct {
		Title   string `json:"title"`
		Answer  string `json:"answer"`
		Snippet string `json:"snippet"`
	} `json:"answerBox"`
	KnowledgeGraph *struct {
		Title       string `json:"title"`
		Type        string `json:"type"`
		Description string `json:"description"`
		Website     string `json:"website"`
	} `json:"knowledgeGraph"`
	Organic []struct {
		Title    string `json:"title"`
		Link     string `json:"link"`
		Snippet  string `json:"snippet"`
		Date     string `json:"date"`
		Position int    `json:"position"`
	} `json:"organic"`
}

// ---- Tool 构造 ----

type serperSearch struct {
	conf *Config
}

// NewTool 创建 Serper 搜索工具，返回标准的 tool.InvokableTool。
//
// 示例：
//
//	t, err := serper.NewTool(ctx, &serper.Config{
//	    APIKey: os.Getenv("SERPER_API_KEY"),
//	    Num:    10,
//	    GL:     "cn",
//	    HL:     "zh-cn",
//	})
func NewTool(_ context.Context, conf *Config) (tool.InvokableTool, error) {
	if conf == nil {
		return nil, fmt.Errorf("serper: config is required")
	}
	if conf.APIKey == "" {
		return nil, fmt.Errorf("serper: APIKey is required")
	}

	c := *conf
	c.withDefaults()

	ss := &serperSearch{conf: &c}

	tl, err := utils.InferTool(
		c.ToolName,
		c.ToolDesc,
		ss.search,
		utils.WithMarshalOutput(ss.marshalOutput),
	)
	if err != nil {
		return nil, fmt.Errorf("serper: InferTool failed: %w", err)
	}
	return tl, nil
}

// search 执行 HTTP 请求，返回原始响应供 marshalOutput 处理
func (ss *serperSearch) search(ctx context.Context, req *SearchRequest) (*serperResponse, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("serper: query must not be empty")
	}

	num := req.Num
	if num <= 0 {
		num = ss.conf.Num
	}
	gl := req.GL
	if gl == "" {
		gl = ss.conf.GL
	}
	hl := req.HL
	if hl == "" {
		hl = ss.conf.HL
	}

	body := serperRequest{
		Q:   req.Query,
		Num: num,
		GL:  gl,
		HL:  hl,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("serper: marshal request failed: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		ss.conf.Endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("serper: create http request failed: %w", err)
	}
	httpReq.Header.Set("X-API-KEY", ss.conf.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := ss.conf.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("serper: http request failed: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("serper: read response body failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("serper: unexpected http status %d: %s", resp.StatusCode, rawBody)
	}

	var serperResp serperResponse
	if err := json.Unmarshal(rawBody, &serperResp); err != nil {
		return nil, fmt.Errorf("serper: unmarshal response failed: %w", err)
	}
	return &serperResp, nil
}

// marshalOutput 将 *serperResponse 精简为 LLM 友好的 JSON 字符串
func (ss *serperSearch) marshalOutput(_ context.Context, output any) (string, error) {
	resp, ok := output.(*serperResponse)
	if !ok {
		return "", fmt.Errorf("serper: unexpected output type, want *serperResponse but got %T", output)
	}

	result := SearchResult{
		Query:   resp.SearchParameters.Q,
		Organic: make([]*OrganicItem, 0, len(resp.Organic)),
	}

	if ab := resp.AnswerBox; ab != nil {
		result.AnswerBox = &AnswerBox{
			Title:   ab.Title,
			Answer:  ab.Answer,
			Snippet: ab.Snippet,
		}
	}

	if kg := resp.KnowledgeGraph; kg != nil {
		result.KnowledgeGraph = &KnowledgeGraph{
			Title:       kg.Title,
			Type:        kg.Type,
			Description: kg.Description,
			Website:     kg.Website,
		}
	}

	for _, item := range resp.Organic {
		result.Organic = append(result.Organic, &OrganicItem{
			Title:    item.Title,
			Link:     item.Link,
			Snippet:  item.Snippet,
			Date:     item.Date,
			Position: item.Position,
		})
	}

	b, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("serper: marshal SearchResult failed: %w", err)
	}
	return string(b), nil
}
