package brave

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func mockServer(t *testing.T, respBody string, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subscription-Token") == "" {
			t.Error("missing X-Subscription-Token header")
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(respBody))
	}))
}

const mockSuccessResp = `{
  "query": {
    "original": "open source golang projects",
    "more_results_available": true
  },
  "web": {
    "results": [
      {
        "title": "cloudwego/eino",
        "url": "https://github.com/cloudwego/eino",
        "description": "The ultimate LLM/AI application development framework in Go.",
        "age": "2 days ago",
        "page_age": "2025-04-01T10:00:00"
      },
      {
        "title": "golang/go",
        "url": "https://github.com/golang/go",
        "description": "The Go programming language.",
        "age": "1 hour ago",
        "page_age": "2025-04-03T08:00:00"
      }
    ]
  }
}`

func TestNewTool_NilConfig(t *testing.T) {
	_, err := NewTool(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestNewTool_MissingAPIKey(t *testing.T) {
	_, err := NewTool(context.Background(), &Config{})
	if err == nil {
		t.Fatal("expected error for missing APIKey")
	}
}

func TestNewTool_Success(t *testing.T) {
	srv := mockServer(t, mockSuccessResp, 200)
	defer srv.Close()

	tl, err := NewTool(context.Background(), &Config{
		APIKey:   "test-key",
		Endpoint: srv.URL,
	})
	if err != nil {
		t.Fatalf("NewTool failed: %v", err)
	}
	if tl == nil {
		t.Fatal("expected non-nil tool")
	}
}

func TestSearch_Success(t *testing.T) {
	srv := mockServer(t, mockSuccessResp, 200)
	defer srv.Close()

	bs := &braveSearch{conf: &Config{
		APIKey:     "test-key",
		Endpoint:   srv.URL,
		Count:      10,
		SafeSearch: SafeSearchModerate,
		HTTPClient: &http.Client{},
	}}

	resp, err := bs.search(context.Background(), &SearchRequest{Query: "open source golang projects"})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if resp.Web == nil || len(resp.Web.Results) == 0 {
		t.Fatal("expected non-empty web results")
	}
	if !resp.Query.MoreResultsAvailable {
		t.Error("expected more_results_available=true")
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	bs := &braveSearch{conf: &Config{APIKey: "test-key", HTTPClient: &http.Client{}}}
	_, err := bs.search(context.Background(), &SearchRequest{Query: "   "})
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestSearch_HTTPError(t *testing.T) {
	srv := mockServer(t, `{"message":"too many requests"}`, 429)
	defer srv.Close()

	bs := &braveSearch{conf: &Config{
		APIKey:     "test-key",
		Endpoint:   srv.URL,
		Count:      10,
		SafeSearch: SafeSearchModerate,
		HTTPClient: &http.Client{},
	}}
	_, err := bs.search(context.Background(), &SearchRequest{Query: "test"})
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("expected 429 error, got: %v", err)
	}
}

func TestSearch_QueryStringParams(t *testing.T) {
	// 验证 GET query string 参数是否正确编码
	var capturedURL *url.URL
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL
		w.WriteHeader(200)
		_, _ = w.Write([]byte(mockSuccessResp))
	}))
	defer srv.Close()

	bs := &braveSearch{conf: &Config{
		APIKey:     "test-key",
		Endpoint:   srv.URL,
		Count:      10,
		SafeSearch: SafeSearchStrict,
		HTTPClient: &http.Client{},
	}}

	_, _ = bs.search(context.Background(), &SearchRequest{
		Query:      "golang",
		Count:      5,
		Offset:     10,
		Country:    "us",
		SearchLang: "en",
	})

	q := capturedURL.Query()
	if q.Get("q") != "golang" {
		t.Errorf("expected q=golang, got %s", q.Get("q"))
	}
	if q.Get("count") != "5" {
		t.Errorf("expected count=5, got %s", q.Get("count"))
	}
	if q.Get("offset") != "10" {
		t.Errorf("expected offset=10, got %s", q.Get("offset"))
	}
	if q.Get("country") != "us" {
		t.Errorf("expected country=us, got %s", q.Get("country"))
	}
	if q.Get("search_lang") != "en" {
		t.Errorf("expected search_lang=en, got %s", q.Get("search_lang"))
	}
	if q.Get("safesearch") != "strict" {
		t.Errorf("expected safesearch=strict, got %s", q.Get("safesearch"))
	}
}

func TestMarshalOutput_Success(t *testing.T) {
	bs := &braveSearch{conf: &Config{}}
	resp := &braveResponse{
		Query: &struct {
			Original             string `json:"original"`
			MoreResultsAvailable bool   `json:"more_results_available"`
		}{Original: "golang", MoreResultsAvailable: true},
		Web: &struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
				Age         string `json:"age"`
				PageAge     string `json:"page_age"`
			} `json:"results"`
		}{
			Results: []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
				Age         string `json:"age"`
				PageAge     string `json:"page_age"`
			}{
				{Title: "Go", URL: "https://go.dev", Description: "The Go language", Age: "1 day ago"},
			},
		},
	}

	out, err := bs.marshalOutput(context.Background(), resp)
	if err != nil {
		t.Fatalf("marshalOutput failed: %v", err)
	}

	var result SearchResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if result.Query != "golang" {
		t.Errorf("unexpected query: %s", result.Query)
	}
	if !result.MoreResultsAvailable {
		t.Error("expected more_results_available=true")
	}
	if len(result.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(result.Results))
	}
}

func TestMarshalOutput_WrongType(t *testing.T) {
	bs := &braveSearch{conf: &Config{}}
	_, err := bs.marshalOutput(context.Background(), 42)
	if err == nil {
		t.Fatal("expected type assertion error")
	}
}
