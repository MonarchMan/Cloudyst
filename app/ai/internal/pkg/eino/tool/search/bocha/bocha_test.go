package bocha

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func mockServer(t *testing.T, respBody string, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求头
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("missing Authorization header")
		}
		w.WriteHeader(statusCode)
		w.Write([]byte(respBody))
	}))
}

var mockResp = `{
  "code": 200,
  "logId": "test-log-id",
  "msg": "success",
  "data": {
    "_type": "SearchResponse",
    "queryContext": {"originalQuery": "Go语言"},
    "webPages": {
      "webSearchUrl": "https://bochaai.com/Search?q=Go",
      "totalEstimatedMatches": 1000000,
      "value": [
        {
          "id": "1",
          "name": "Go语言官网",
          "url": "https://go.dev",
          "snippet": "Go is an open source programming language...",
          "summary": "Go（又称Golang）是Google开发的静态类型编译语言...",
          "siteName": "go.dev",
          "datePublished": "2024-01-01T00:00:00+08:00"
        }
      ]
    }
  }
}`

func TestNewTool_MissingAPIKey(t *testing.T) {
	_, err := NewTool(context.Background(), &Config{})
	if err == nil {
		t.Fatal("expected error for missing APIKey")
	}
}

func TestInvokableRun_Success(t *testing.T) {
	srv := mockServer(t, mockResp, 200)
	defer srv.Close()

	tool, err := NewTool(context.Background(), &Config{
		APIKey:   "sk-test",
		Endpoint: srv.URL,
		Summary:  true,
	})
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}

	args, _ := json.Marshal(SearchRequest{Query: "Go语言"})
	result, err := tool.InvokableRun(context.Background(), string(args))
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(result, "Go语言官网") {
		t.Errorf("unexpected result: %s", result)
	}
	if !strings.Contains(result, "https://go.dev") {
		t.Errorf("result missing URL: %s", result)
	}
}

func TestInvokableRun_EmptyQuery(t *testing.T) {
	tool, _ := NewTool(context.Background(), &Config{APIKey: "sk-test"})
	args, _ := json.Marshal(SearchRequest{Query: "  "})
	_, err := tool.InvokableRun(context.Background(), string(args))
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestInvokableRun_APIError(t *testing.T) {
	srv := mockServer(t, `{"code":401,"msg":"invalid api key","logId":"xxx"}`, 200)
	defer srv.Close()

	tool, _ := NewTool(context.Background(), &Config{APIKey: "invalid", Endpoint: srv.URL})
	args, _ := json.Marshal(SearchRequest{Query: "test"})
	_, err := tool.InvokableRun(context.Background(), string(args))
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected API error, got: %v", err)
	}
}

func TestInfo(t *testing.T) {
	tool, _ := NewTool(context.Background(), &Config{APIKey: "sk-test"})
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "bocha_web_search" {
		t.Errorf("unexpected tool name: %s", info.Name)
	}
}

func TestInvokeBocha(t *testing.T) {
	searchClient, err := NewTool(context.Background(), &Config{APIKey: "sk-814230a8315b46c5b020a05bbdb759d1"})
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(SearchRequest{Query: "成都的景点有哪些"})
	result, err := searchClient.InvokableRun(context.Background(), string(args))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("result: %s", result)
}
