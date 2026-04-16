package serper

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
		if r.Header.Get("X-API-KEY") == "" {
			t.Error("missing X-API-KEY header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("Content-Type must be application/json")
		}
		var reqBody serperRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("invalid request body: %v", err)
		}
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(respBody))
	}))
}

const mockSuccessResp = `{
  "searchParameters": {"q": "golang concurrency"},
  "answerBox": {
    "title": "Go Concurrency",
    "answer": "Go uses goroutines and channels.",
    "snippet": "Go's concurrency model is based on CSP."
  },
  "knowledgeGraph": {
    "title": "Go (programming language)",
    "type": "Programming language",
    "description": "Go is a statically typed, compiled language.",
    "website": "https://go.dev"
  },
  "organic": [
    {
      "title": "Effective Go - Concurrency",
      "link": "https://go.dev/doc/effective_go#concurrency",
      "snippet": "Go provides goroutines and channels for concurrency.",
      "date": "2024-01-01",
      "position": 1
    },
    {
      "title": "Go by Example: Goroutines",
      "link": "https://gobyexample.com/goroutines",
      "snippet": "A goroutine is a lightweight thread of execution.",
      "position": 2
    }
  ]
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

	ss := &serperSearch{conf: &Config{
		APIKey:     "test-key",
		Endpoint:   srv.URL,
		Num:        10,
		HTTPClient: &http.Client{},
	}}

	resp, err := ss.search(context.Background(), &SearchRequest{Query: "golang concurrency"})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(resp.Organic) == 0 {
		t.Fatal("expected non-empty organic results")
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	ss := &serperSearch{conf: &Config{APIKey: "test-key", HTTPClient: &http.Client{}}}
	_, err := ss.search(context.Background(), &SearchRequest{Query: "  "})
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestSearch_HTTPError(t *testing.T) {
	srv := mockServer(t, ``, 429)
	defer srv.Close()

	ss := &serperSearch{conf: &Config{
		APIKey:     "test-key",
		Endpoint:   srv.URL,
		Num:        10,
		HTTPClient: &http.Client{},
	}}
	_, err := ss.search(context.Background(), &SearchRequest{Query: "test"})
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("expected 429 error, got: %v", err)
	}
}

func TestMarshalOutput_Full(t *testing.T) {
	ss := &serperSearch{conf: &Config{}}

	resp := &serperResponse{
		SearchParameters: struct {
			Q string `json:"q"`
		}{Q: "golang concurrency"},
		AnswerBox: &struct {
			Title   string `json:"title"`
			Answer  string `json:"answer"`
			Snippet string `json:"snippet"`
		}{Title: "Go Concurrency", Answer: "goroutines", Snippet: "CSP model"},
		KnowledgeGraph: &struct {
			Title       string `json:"title"`
			Type        string `json:"type"`
			Description string `json:"description"`
			Website     string `json:"website"`
		}{Title: "Go", Type: "Language", Description: "Compiled language", Website: "https://go.dev"},
		Organic: []struct {
			Title    string `json:"title"`
			Link     string `json:"link"`
			Snippet  string `json:"snippet"`
			Date     string `json:"date"`
			Position int    `json:"position"`
		}{
			{Title: "Effective Go", Link: "https://go.dev/doc/effective_go", Snippet: "...", Position: 1},
		},
	}

	out, err := ss.marshalOutput(context.Background(), resp)
	if err != nil {
		t.Fatalf("marshalOutput failed: %v", err)
	}

	var result SearchResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if result.Query != "golang concurrency" {
		t.Errorf("unexpected query: %s", result.Query)
	}
	if result.AnswerBox == nil {
		t.Error("expected AnswerBox")
	}
	if result.KnowledgeGraph == nil {
		t.Error("expected KnowledgeGraph")
	}
	if len(result.Organic) != 1 {
		t.Errorf("expected 1 organic result, got %d", len(result.Organic))
	}
}

func TestMarshalOutput_WrongType(t *testing.T) {
	ss := &serperSearch{conf: &Config{}}
	_, err := ss.marshalOutput(context.Background(), "not a serperResponse")
	if err == nil {
		t.Fatal("expected type assertion error")
	}
}

func TestSearch_RequestOverridesConfig(t *testing.T) {
	var capturedReq serperRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedReq)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(mockSuccessResp))
	}))
	defer srv.Close()

	ss := &serperSearch{conf: &Config{
		APIKey:     "test-key",
		Endpoint:   srv.URL,
		Num:        10,
		GL:         "us",
		HL:         "en",
		HTTPClient: &http.Client{},
	}}

	_, _ = ss.search(context.Background(), &SearchRequest{
		Query: "test",
		Num:   5,
		GL:    "cn",
		HL:    "zh-cn",
	})

	if capturedReq.Num != 5 {
		t.Errorf("expected num=5, got %d", capturedReq.Num)
	}
	if capturedReq.GL != "cn" {
		t.Errorf("expected gl=cn, got %s", capturedReq.GL)
	}
	if capturedReq.HL != "zh-cn" {
		t.Errorf("expected hl=zh-cn, got %s", capturedReq.HL)
	}
}
