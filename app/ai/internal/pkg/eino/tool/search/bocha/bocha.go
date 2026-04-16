package bocha

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
	defaultEndpoint   = "https://api.bochaai.com/v1/web-search"
	defaultMaxResults = 5
	defaultTimeout    = 10 * time.Second
	defaultToolName   = "bocha_web_search"
	defaultToolDesc   = "使用博查搜索引擎搜索互联网实时信息，返回网页标题、链接和摘要。适合查询最新资讯、事实核查、获取当前数据。"
)

// Freshness 搜索时效枚举
type Freshness string

const (
	FreshnessOneDay   Freshness = "oneDay"
	FreshnessOneWeek  Freshness = "oneWeek"
	FreshnessOneMonth Freshness = "oneMonth"
	FreshnessOneYear  Freshness = "oneYear"
	FreshnessNoLimit  Freshness = "noLimit"
)

// Config 博查搜索工具配置
type Config struct {
	// APIKey 博查开放平台 API Key，必填
	APIKey string `json:"api_key"`
	// Endpoint 自定义接口地址，可选，默认 https://api.bochaai.com/v1/web-search
	Endpoint string `json:"endpoint"`
	// Count 默认返回结果数，范围 1-50，默认 5；可被 SearchRequest.Count 覆盖
	Count int `json:"count"`
	// Freshness 默认搜索时效，默认 FreshnessNoLimit；可被 SearchRequest.Freshness 覆盖
	Freshness Freshness `json:"freshness"`
	// Summary 是否让博查返回 AI 长摘要，默认 true
	Summary bool `json:"summary"`

	// ToolName 工具名称，默认 "bocha_web_search"
	ToolName string `json:"tool_name"`
	// ToolDesc 工具描述，默认见 defaultToolDesc
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
	if c.Count <= 0 || c.Count > 50 {
		c.Count = defaultMaxResults
	}
	if c.Freshness == "" {
		c.Freshness = FreshnessNoLimit
	}
	if c.Endpoint == "" {
		c.Endpoint = defaultEndpoint
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: defaultTimeout}
	}
}

// ---- 对外暴露的请求 / 响应类型 ----

// SearchRequest 是暴露给 LLM Function Call 的入参 schema。
// 字段 tag `jsonschema_description` 与 google search 实现保持一致风格。
type SearchRequest struct {
	// Query 搜索关键词或自然语言问题
	Query string `json:"query" jsonschema_description:"queried string to the bocha Search engine"`
	// Count 本次返回结果数，1-50，0 表示使用 Config 默认值
	Count int `json:"count,omitempty" jsonschema_description:"number of Search results to return, valid values are between 1 and 50, inclusive"`
	// Freshness 时效过滤，留空使用 Config 默认值
	Freshness Freshness `json:"freshness,omitempty" jsonschema_description:"time range filter for results: oneDay, oneWeek, oneMonth, oneYear, noLimit"`
}

// SearchResult 是最终序列化返回给 LLM 的结构
type SearchResult struct {
	Query                 string        `json:"query"`
	TotalEstimatedMatches int           `json:"total_estimated_matches,omitempty"`
	Items                 []*SearchItem `json:"items"`
}

// SearchItem 单条搜索结果
type SearchItem struct {
	Title         string `json:"title,omitempty"`
	URL           string `json:"url"`
	SiteName      string `json:"site_name,omitempty"`
	DatePublished string `json:"date_published,omitempty"`
	Snippet       string `json:"snippet,omitempty"`
	// Summary 博查 AI 摘要，仅 Config.Summary=true 时有值
	Summary string `json:"summary,omitempty"`
}

// ---- 内部 HTTP 请求 / 响应结构 ----

type bochaRequest struct {
	Query     string    `json:"query"`
	Freshness Freshness `json:"freshness"`
	Summary   bool      `json:"summary"`
	Count     int       `json:"count"`
}

type BochaResponse struct {
	Code  int        `json:"code"`
	LogID string     `json:"logId"`
	Msg   string     `json:"msg"`
	Data  *bochaData `json:"data"`
}

type bochaData struct {
	QueryContext struct {
		OriginalQuery string `json:"originalQuery"`
	} `json:"queryContext"`
	WebPages *bochaWebPages `json:"webPages"`
}

type bochaWebPages struct {
	TotalEstimatedMatches int         `json:"totalEstimatedMatches"`
	Value                 []bochaPage `json:"value"`
}

type bochaPage struct {
	ID               string `json:"id"`               // 网页排序 ID
	Name             string `json:"name"`             // 网页标题
	URL              string `json:"url"`              // 网页链接
	Snippet          string `json:"snippet"`          // 短摘要（150-300字）
	Summary          string `json:"summary"`          // AI 长摘要（summary=true 时有）
	SiteName         string `json:"siteName"`         // 站点名称
	SiteIcon         string `json:"siteIcon"`         // 站点图标 url
	DatePublished    string `json:"datePublished"`    // 发布时间 RFC3339
	CachedPageUrl    string `json:"cachedPageUrl"`    // 缓存页面 url
	Language         string `json:"language"`         // 语言
	IsFamilyFriendly bool   `json:"isFamilyFriendly"` // 是否为家庭友好的页面
	IsNavigational   bool   `json:"isNavigational"`   // 是否为导航性页面
}

// ---- Tool 构造 ----

// BochaSearch 持有配置和 HTTP 客户端，是实际的业务逻辑载体
type BochaSearch struct {
	conf *Config
}

// NewTool 创建博查搜索工具，返回标准的 tool.InvokableTool。
//
// 示例：
//
//	t, err := bocha.NewTool(ctx, &bocha.Config{
//	    APIKey:    os.Getenv("BOCHA_API_KEY"),
//	    Count:     8,
//	    Summary:   true,
//	    Freshness: bocha.FreshnessOneMonth,
//	})
func NewTool(ctx context.Context, conf *Config) (tool.InvokableTool, error) {
	if conf == nil {
		conf = &Config{}
	}
	if conf.APIKey == "" {
		return nil, fmt.Errorf("bocha: APIKey is required")
	}

	// 拷贝一份，避免修改调用方的结构体
	//c := *conf
	conf.withDefaults()

	bs := &BochaSearch{conf: conf}

	// 与 google search 保持一致：
	// - search 函数签名交给 InferTool 推断 schema
	// - MarshalOutput 负责将内部响应转为 LLM 友好的 JSON 字符串
	tl, err := utils.InferTool(
		conf.ToolName,
		conf.ToolDesc,
		bs.Search,
		utils.WithMarshalOutput(bs.MarshalOutput),
	)
	if err != nil {
		return nil, fmt.Errorf("bocha: InferTool failed: %w", err)
	}

	return tl, nil
}

func NewBoCha(conf *Config) (*BochaSearch, error) {
	if conf == nil {
		conf = &Config{}
	}
	if conf.APIKey == "" {
		return nil, fmt.Errorf("bocha: APIKey is required")
	}
	conf.withDefaults()
	return &BochaSearch{conf: conf}, nil
}

// Search 是核心业务函数，由 InferTool 通过反射推断入参 schema（SearchRequest）。
// 返回值 *BochaResponse 会传给 MarshalOutput 做格式化。
func (bs *BochaSearch) Search(ctx context.Context, req *SearchRequest) (*BochaResponse, error) {
	if strings.TrimSpace(req.Query) == "" {
		return nil, fmt.Errorf("bocha: query must not be empty")
	}

	// 本次请求参数：优先使用请求级别的值，回退到 Config 默认值
	count := req.Count
	if count <= 0 {
		count = bs.conf.Count
	}
	freshness := req.Freshness
	if freshness == "" {
		freshness = bs.conf.Freshness
	}

	body := bochaRequest{
		Query:     req.Query,
		Freshness: freshness,
		Summary:   bs.conf.Summary,
		Count:     count,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("bocha: marshal request failed: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		bs.conf.Endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("bocha: create http request failed: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+bs.conf.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := bs.conf.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("bocha: http request failed: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("bocha: read response body failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bocha: unexpected http status %d: %s", resp.StatusCode, rawBody)
	}

	var bochaResp BochaResponse
	if err := json.Unmarshal(rawBody, &bochaResp); err != nil {
		return nil, fmt.Errorf("bocha: unmarshal response failed: %w", err)
	}
	if bochaResp.Code != 200 {
		return nil, fmt.Errorf("bocha: API error code=%d msg=%s logId=%s",
			bochaResp.Code, bochaResp.Msg, bochaResp.LogID)
	}

	return &bochaResp, nil
}

// MarshalOutput 将 search 返回的 *BochaResponse 转换为 LLM 友好的 JSON 字符串。
// 与 google search 的 MarshalOutput 职责完全一致：类型断言 + 精简字段 + 序列化。
func (bs *BochaSearch) MarshalOutput(_ context.Context, output any) (string, error) {
	resp, ok := output.(*BochaResponse)
	if !ok {
		return "", fmt.Errorf("bocha: unexpected output type, want *bochaResponse but got %T", output)
	}

	result := SearchResult{}

	if resp.Data != nil {
		result.Query = resp.Data.QueryContext.OriginalQuery

		if wp := resp.Data.WebPages; wp != nil {
			result.TotalEstimatedMatches = wp.TotalEstimatedMatches
			result.Items = make([]*SearchItem, 0, len(wp.Value))

			for _, p := range wp.Value {
				item := &SearchItem{
					Title:    p.Name,
					URL:      p.URL,
					SiteName: p.SiteName,
					Snippet:  p.Snippet,
					Summary:  p.Summary,
				}
				// 只保留日期部分 "2024-07-22T00:00:00+08:00" -> "2024-07-22"
				if len(p.DatePublished) >= 10 {
					item.DatePublished = p.DatePublished[:10]
				}
				result.Items = append(result.Items, item)
			}
		}
	}

	b, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("bocha: marshal SearchResult failed: %w", err)
	}
	return string(b), nil
}
