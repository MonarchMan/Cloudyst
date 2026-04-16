package brave

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

const (
	defaultEndpoint = "https://api.search.brave.com/res/v1/web/search"
	defaultCount    = 10
	defaultTimeout  = 10 * time.Second
	defaultToolName = "brave_web_search"
	defaultToolDesc = "Search the web using Brave's independent index. Returns structured results with title, URL, and description."
)

// SafeSearch 安全搜索级别
type SafeSearch string

const (
	SafeSearchOff      SafeSearch = "off"
	SafeSearchModerate SafeSearch = "moderate"
	SafeSearchStrict   SafeSearch = "strict"
)

// Config Brave 搜索工具配置
type Config struct {
	// APIKey Brave Search API Key，必填，请求头 X-Subscription-Token
	APIKey string `json:"api_key"`
	// Endpoint 自定义接口地址，默认 https://api.search.brave.com/res/v1/web/search
	Endpoint string `json:"endpoint"`
	// Count 默认返回条数，最大 20，默认 10；可被 SearchRequest.Count 覆盖
	Count int `json:"count"`
	// Country 国家代码，如 "us"、"cn"；可被 SearchRequest.Country 覆盖
	Country string `json:"country"`
	// SearchLang 内容语言偏好，如 "en"、"zh-hans"；可被 SearchRequest.SearchLang 覆盖
	SearchLang string `json:"search_lang"`
	// SafeSearch 安全过滤级别，默认 SafeSearchModerate
	SafeSearch SafeSearch `json:"safesearch"`

	// ToolName 工具名称，默认 "brave_web_search"
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
	if c.Count <= 0 || c.Count > 20 {
		c.Count = defaultCount
	}
	if c.SafeSearch == "" {
		c.SafeSearch = SafeSearchModerate
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
	Query string `json:"query" jsonschema_description:"search query string sent to Brave Search"`
	// Count 返回条数，最大 20，默认使用 Config.Count
	Count int `json:"count,omitempty" jsonschema_description:"number of results to return, max 20"`
	// Offset 分页偏移量，默认 0
	Offset int `json:"offset,omitempty" jsonschema_description:"pagination offset for results, default 0"`
	// Country 国家代码，如 us、cn、jp
	Country string `json:"country,omitempty" jsonschema_description:"country code to bias results, e.g. us, cn, jp"`
	// SearchLang 内容语言，如 en、zh-hans
	SearchLang string `json:"search_lang,omitempty" jsonschema_description:"preferred language for search results, e.g. en, zh-hans"`
}

// SearchResult 返回给 LLM 的简化结构
type SearchResult struct {
	Query                string       `json:"query"`
	MoreResultsAvailable bool         `json:"more_results_available,omitempty"`
	Results              []*WebResult `json:"results"`
}

// WebResult 单条网页搜索结果
type WebResult struct {
	Title       string `json:"title,omitempty"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	// Age 文章发布时间，如 "2 days ago" 或 ISO 时间
	Age     string `json:"age,omitempty"`
	PageAge string `json:"page_age,omitempty"`
}

// ---- 内部 HTTP 响应结构 ----

type braveResponse struct {
	Query *struct {
		Original             string `json:"original"`
		MoreResultsAvailable bool   `json:"more_results_available"`
	} `json:"query"`
	Web *struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
			Age         string `json:"age"`
			PageAge     string `json:"page_age"`
		} `json:"results"`
	} `json:"web"`
}

// ---- Tool 构造 ----

type braveSearch struct {
	conf *Config
}

// NewTool 创建 Brave Search 工具，返回标准的 tool.InvokableTool。
//
// 示例：
//
//	t, err := brave.NewTool(ctx, &brave.Config{
//	    APIKey:     os.Getenv("BRAVE_API_KEY"),
//	    Count:      10,
//	    Country:    "us",
//	    SearchLang: "en",
//	})
func NewTool(_ context.Context, conf *Config) (tool.InvokableTool, error) {
	if conf == nil {
		return nil, fmt.Errorf("brave: config is required")
	}
	if conf.APIKey == "" {
		return nil, fmt.Errorf("brave: APIKey is required")
	}

	c := *conf
	c.withDefaults()

	bs := &braveSearch{conf: &c}

	tl, err := utils.InferTool(
		c.ToolName,
		c.ToolDesc,
		bs.search,
		utils.WithMarshalOutput(bs.marshalOutput),
	)
	if err != nil {
		return nil, fmt.Errorf("brave: InferTool failed: %w", err)
	}
	return tl, nil
}

// search 执行 Brave Search HTTP GET 请求（Brave 用 query string，非 POST body）
func (bs *braveSearch) search(ctx context.Context, req *SearchRequest) (*braveResponse, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("brave: query must not be empty")
	}

	count := req.Count
	if count <= 0 {
		count = bs.conf.Count
	}
	country := req.Country
	if country == "" {
		country = bs.conf.Country
	}
	searchLang := req.SearchLang
	if searchLang == "" {
		searchLang = bs.conf.SearchLang
	}

	// Brave 使用 GET + query string
	u, err := url.Parse(bs.conf.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("brave: parse endpoint failed: %w", err)
	}
	q := u.Query()
	q.Set("q", req.Query)
	q.Set("count", fmt.Sprintf("%d", count))
	q.Set("safesearch", string(bs.conf.SafeSearch))
	if req.Offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", req.Offset))
	}
	if country != "" {
		q.Set("country", country)
	}
	if searchLang != "" {
		q.Set("search_lang", searchLang)
	}
	u.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("brave: create http request failed: %w", err)
	}
	httpReq.Header.Set("X-Subscription-Token", bs.conf.APIKey)
	httpReq.Header.Set("Accept", "application/json")
	// Brave 推荐启用 gzip，标准 http.Client 会自动解压
	httpReq.Header.Set("Accept-Encoding", "gzip")

	resp, err := bs.conf.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("brave: http request failed: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("brave: read response body failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("brave: unexpected http status %d: %s", resp.StatusCode, rawBody)
	}

	var braveResp braveResponse
	if err := json.Unmarshal(rawBody, &braveResp); err != nil {
		return nil, fmt.Errorf("brave: unmarshal response failed: %w", err)
	}
	return &braveResp, nil
}

// marshalOutput 将 *braveResponse 精简为 LLM 友好的 JSON 字符串
func (bs *braveSearch) marshalOutput(_ context.Context, output any) (string, error) {
	resp, ok := output.(*braveResponse)
	if !ok {
		return "", fmt.Errorf("brave: unexpected output type, want *braveResponse but got %T", output)
	}

	result := SearchResult{
		Results: make([]*WebResult, 0),
	}

	if resp.Query != nil {
		result.Query = resp.Query.Original
		result.MoreResultsAvailable = resp.Query.MoreResultsAvailable
	}

	if resp.Web != nil {
		for _, item := range resp.Web.Results {
			result.Results = append(result.Results, &WebResult{
				Title:       item.Title,
				URL:         item.URL,
				Description: item.Description,
				Age:         item.Age,
				PageAge:     item.PageAge,
			})
		}
	}

	b, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("brave: marshal SearchResult failed: %w", err)
	}
	return string(b), nil
}
