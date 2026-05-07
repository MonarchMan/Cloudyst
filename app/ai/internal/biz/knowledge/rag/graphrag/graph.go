package graphrag

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"ai/internal/pkg/eino/doc/enhance"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const (
	NodePrepare  = "prepare"
	NodeRewrite  = "plan_queries"
	NodeRetrieve = "retrieve"
	NodeFuse     = "fuse"
	NodeNeighbor = "expand_neighbors"
	NodeRerank   = "rerank"
	NodeCompress = "compress_context"
	NodeAssemble = "assemble"
	NodeFallback = "fallback"
	NodeAnswer   = "answer"
	NodeVerify   = "verify_answer"
)

type Request struct {
	Query              string
	AdditionalQueries  []string
	TopK               int
	ScoreThreshold     float64
	GenerateAnswer     bool
	RetrieverOptions   []retriever.Option
	TransformerOptions []document.TransformerOption
	Metadata           map[string]any
}

type Result struct {
	Query          string
	RewrittenQuery string
	Queries        []string
	Documents      []*schema.Document
	Retrievals     []RetrievalResult
	Context        string
	Citations      []Citation
	Answer         string
	Verification   *AnswerVerification
	Trace          *Trace
	Evaluation     *Evaluation
	Metadata       map[string]any
}

type Citation struct {
	Index    int
	ID       string
	Title    string
	Summary  string
	Terms    []string
	Score    float64
	MetaData map[string]any
}

type Config struct {
	Retriever            retriever.Retriever
	Reranker             document.Transformer
	QueryRewriter        QueryRewriter
	QueryExpander        QueryExpander
	NeighborExpander     NeighborExpander
	ContextCompressor    ContextCompressor
	AnswerGenerator      AnswerGenerator
	AnswerVerifier       AnswerVerifier
	Fallback             FallbackHandler
	Evaluator            Evaluator
	TraceObserver        TraceObserver
	TopK                 int
	ScoreThreshold       float64
	MaxQueries           int
	FusionRRFK           float64
	GenerateAnswer       bool
	MaxContextChars      int
	CitationContentChars int
	ContextSeparator     string
}

type Graph struct {
	conf   Config
	runner compose.Runnable[*Request, *Result]
}

type graphState struct {
	request       *Request
	query         string
	rewritten     string
	queries       []string
	documents     []*schema.Document
	retrievals    []RetrievalResult
	context       string
	citations     []Citation
	answer        string
	verification  *AnswerVerification
	trace         *Trace
	evaluation    *Evaluation
	retrieverOpts []retriever.Option
	transformOpts []document.TransformerOption
}

func New(conf *Config) (*Graph, error) {
	if conf == nil {
		return nil, fmt.Errorf("raggraph config is nil")
	}
	if conf.Retriever == nil {
		return nil, fmt.Errorf("raggraph retriever is nil")
	}

	cfg := *conf
	if cfg.QueryRewriter == nil {
		cfg.QueryRewriter = NoopQueryRewriter{}
	}
	if cfg.QueryExpander == nil {
		cfg.QueryExpander = NoopQueryExpander{}
	}
	if cfg.NeighborExpander == nil {
		cfg.NeighborExpander = NoopNeighborExpander{}
	}
	if cfg.ContextCompressor == nil {
		cfg.ContextCompressor = NoopContextCompressor{}
	}
	if cfg.Fallback == nil {
		cfg.Fallback = NoopFallbackHandler{}
	}
	if cfg.AnswerVerifier == nil {
		cfg.AnswerVerifier = DefaultAnswerVerifier{RequireCitations: true}
	}
	if cfg.Evaluator == nil {
		cfg.Evaluator = DefaultEvaluator{}
	}
	if cfg.TopK <= 0 {
		cfg.TopK = 5
	}
	if cfg.MaxQueries <= 0 {
		cfg.MaxQueries = 3
	}
	if cfg.MaxContextChars <= 0 {
		cfg.MaxContextChars = 6000
	}
	if cfg.CitationContentChars <= 0 {
		cfg.CitationContentChars = 240
	}
	if cfg.ContextSeparator == "" {
		cfg.ContextSeparator = "\n\n"
	}

	graph := &Graph{conf: cfg}
	runner, err := graph.compile(context.Background())
	if err != nil {
		return nil, err
	}
	graph.runner = runner
	return graph, nil
}

func (g *Graph) Invoke(ctx context.Context, input *Request, opts ...compose.Option) (*Result, error) {
	return g.runner.Invoke(ctx, input, opts...)
}

func (g *Graph) compile(ctx context.Context) (compose.Runnable[*Request, *Result], error) {
	gr := compose.NewGraph[*Request, *Result]()

	if err := gr.AddLambdaNode(NodePrepare, compose.InvokableLambda(g.prepare)); err != nil {
		return nil, err
	}
	if err := gr.AddLambdaNode(NodeRewrite, compose.InvokableLambda(g.rewrite)); err != nil {
		return nil, err
	}
	if err := gr.AddLambdaNode(NodeRetrieve, compose.InvokableLambda(g.retrieve)); err != nil {
		return nil, err
	}
	if err := gr.AddLambdaNode(NodeFuse, compose.InvokableLambda(g.fuse)); err != nil {
		return nil, err
	}
	if err := gr.AddLambdaNode(NodeNeighbor, compose.InvokableLambda(g.expandNeighbors)); err != nil {
		return nil, err
	}
	if err := gr.AddLambdaNode(NodeRerank, compose.InvokableLambda(g.rerank)); err != nil {
		return nil, err
	}
	if err := gr.AddLambdaNode(NodeCompress, compose.InvokableLambda(g.compress)); err != nil {
		return nil, err
	}
	if err := gr.AddLambdaNode(NodeAssemble, compose.InvokableLambda(g.assemble)); err != nil {
		return nil, err
	}
	if err := gr.AddLambdaNode(NodeFallback, compose.InvokableLambda(g.fallback)); err != nil {
		return nil, err
	}
	if err := gr.AddLambdaNode(NodeAnswer, compose.InvokableLambda(g.answer)); err != nil {
		return nil, err
	}
	if err := gr.AddLambdaNode(NodeVerify, compose.InvokableLambda(g.verify)); err != nil {
		return nil, err
	}

	edges := [][2]string{
		{compose.START, NodePrepare},
		{NodePrepare, NodeRewrite},
		{NodeRewrite, NodeRetrieve},
		{NodeRetrieve, NodeFuse},
		{NodeFuse, NodeNeighbor},
		{NodeNeighbor, NodeRerank},
		{NodeRerank, NodeCompress},
		{NodeCompress, NodeAssemble},
		{NodeAssemble, NodeFallback},
		{NodeFallback, NodeAnswer},
		{NodeAnswer, NodeVerify},
		{NodeVerify, compose.END},
	}
	for _, edge := range edges {
		if err := gr.AddEdge(edge[0], edge[1]); err != nil {
			return nil, err
		}
	}
	return gr.Compile(ctx)
}

func (g *Graph) prepare(ctx context.Context, req *Request) (*graphState, error) {
	if req == nil {
		return nil, fmt.Errorf("raggraph request is nil")
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, fmt.Errorf("raggraph query is empty")
	}

	topK := req.TopK
	if topK <= 0 {
		topK = g.conf.TopK
	}
	scoreThreshold := req.ScoreThreshold
	if scoreThreshold <= 0 {
		scoreThreshold = g.conf.ScoreThreshold
	}

	retrieverOpts := []retriever.Option{retriever.WithTopK(topK)}
	if scoreThreshold > 0 {
		retrieverOpts = append(retrieverOpts, retriever.WithScoreThreshold(scoreThreshold))
	}
	retrieverOpts = append(retrieverOpts, req.RetrieverOptions...)

	transformOpts := make([]document.TransformerOption, 0, len(req.TransformerOptions))
	transformOpts = append(transformOpts, req.TransformerOptions...)

	return &graphState{
		request:       req,
		query:         query,
		rewritten:     query,
		queries:       []string{query},
		trace:         newTrace(),
		retrieverOpts: retrieverOpts,
		transformOpts: transformOpts,
	}, nil
}

func (g *Graph) rewrite(ctx context.Context, st *graphState) (*graphState, error) {
	done := g.startTrace(st, NodeRewrite, nil)
	rewritten, err := g.conf.QueryRewriter.Rewrite(ctx, st.query)
	if err != nil {
		done(err, nil)
		return nil, err
	}
	rewritten = strings.TrimSpace(rewritten)
	if rewritten == "" {
		rewritten = st.query
	}
	st.rewritten = rewritten

	expanded, err := g.conf.QueryExpander.Expand(ctx, rewritten)
	if err != nil {
		done(err, nil)
		return nil, err
	}
	queries := make([]string, 0, 2+len(st.request.AdditionalQueries)+len(expanded))
	queries = append(queries, rewritten)
	queries = append(queries, st.request.AdditionalQueries...)
	queries = append(queries, expanded...)
	st.queries = normalizeQueries(queries, g.conf.MaxQueries, 512)
	if len(st.queries) == 0 {
		st.queries = []string{st.rewritten}
	}
	done(nil, map[string]any{
		"rewritten_query": st.rewritten,
		"query_count":     len(st.queries),
	})
	return st, nil
}

func (g *Graph) retrieve(ctx context.Context, st *graphState) (*graphState, error) {
	done := g.startTrace(st, NodeRetrieve, map[string]any{"query_count": len(st.queries)})
	retrievals := make([]RetrievalResult, 0, len(st.queries))
	totalDocs := 0
	for _, query := range st.queries {
		docs, err := g.conf.Retriever.Retrieve(ctx, query, st.retrieverOpts...)
		if err != nil {
			done(err, map[string]any{"failed_query": query})
			return nil, err
		}
		totalDocs += len(docs)
		retrievals = append(retrievals, RetrievalResult{
			Query:     query,
			Documents: docs,
		})
	}
	st.retrievals = retrievals
	done(nil, map[string]any{"retrieved_documents": totalDocs})
	return st, nil
}

func (g *Graph) fuse(ctx context.Context, st *graphState) (*graphState, error) {
	done := g.startTrace(st, NodeFuse, nil)
	st.documents = FuseRetrievalResults(st.retrievals, &FusionConfig{
		RRFK:   g.conf.FusionRRFK,
		TopN:   effectiveTopK(st.request.TopK, g.conf.TopK),
		Dedupe: true,
	})
	done(nil, map[string]any{"documents": len(st.documents)})
	return st, nil
}

func (g *Graph) rerank(ctx context.Context, st *graphState) (*graphState, error) {
	if g.conf.Reranker == nil || len(st.documents) == 0 {
		return st, nil
	}
	done := g.startTrace(st, NodeRerank, map[string]any{"before": len(st.documents)})
	docs, err := g.conf.Reranker.Transform(ctx, st.documents, st.transformOpts...)
	if err != nil {
		done(err, nil)
		return nil, err
	}
	st.documents = docs
	done(nil, map[string]any{"after": len(st.documents)})
	return st, nil
}

func (g *Graph) expandNeighbors(ctx context.Context, st *graphState) (*graphState, error) {
	if len(st.documents) == 0 {
		return st, nil
	}
	done := g.startTrace(st, NodeNeighbor, map[string]any{"before": len(st.documents)})
	docs, err := g.conf.NeighborExpander.Expand(ctx, st.documents)
	if err != nil {
		done(err, nil)
		return nil, err
	}
	st.documents = docs
	done(nil, map[string]any{"after": len(st.documents)})
	return st, nil
}

func (g *Graph) compress(ctx context.Context, st *graphState) (*graphState, error) {
	if len(st.documents) == 0 {
		return st, nil
	}
	done := g.startTrace(st, NodeCompress, map[string]any{"before": len(st.documents)})
	docs, err := g.conf.ContextCompressor.Compress(ctx, st.rewritten, st.documents)
	if err != nil {
		done(err, nil)
		return nil, err
	}
	st.documents = docs
	done(nil, map[string]any{"after": len(st.documents)})
	return st, nil
}

func (g *Graph) assemble(ctx context.Context, st *graphState) (*graphState, error) {
	done := g.startTrace(st, NodeAssemble, nil)
	contextText, citations := BuildContext(st.documents, &ContextConfig{
		MaxContextChars:      g.conf.MaxContextChars,
		CitationContentChars: g.conf.CitationContentChars,
		Separator:            g.conf.ContextSeparator,
	})
	st.context = contextText
	st.citations = citations
	done(nil, map[string]any{
		"context_chars": utf8.RuneCountInString(st.context),
		"citations":     len(st.citations),
	})
	return st, nil
}

func (g *Graph) fallback(ctx context.Context, st *graphState) (*graphState, error) {
	reason := fallbackReason(st)
	if reason == "" {
		return st, nil
	}
	done := g.startTrace(st, NodeFallback, map[string]any{"reason": reason})
	output, err := g.conf.Fallback.Handle(ctx, &FallbackInput{
		Query:          st.query,
		RewrittenQuery: st.rewritten,
		Queries:        st.queries,
		Reason:         reason,
		Documents:      st.documents,
		Context:        st.context,
		Citations:      st.citations,
		Answer:         st.answer,
		Metadata:       st.request.Metadata,
	})
	if err != nil {
		done(err, nil)
		return nil, err
	}
	if output != nil && output.Applied {
		if len(output.Documents) > 0 {
			st.documents = output.Documents
			st.context, st.citations = BuildContext(st.documents, &ContextConfig{
				MaxContextChars:      g.conf.MaxContextChars,
				CitationContentChars: g.conf.CitationContentChars,
				Separator:            g.conf.ContextSeparator,
			})
		}
		if output.Context != "" {
			st.context = output.Context
		}
		if output.Answer != "" {
			st.answer = output.Answer
		}
		for k, v := range output.Metadata {
			if st.request.Metadata == nil {
				st.request.Metadata = map[string]any{}
			}
			st.request.Metadata[k] = v
		}
	}
	done(nil, map[string]any{"applied": output != nil && output.Applied})
	return st, nil
}

func (g *Graph) answer(ctx context.Context, st *graphState) (*graphState, error) {
	if g.conf.AnswerGenerator == nil || (!g.conf.GenerateAnswer && !st.request.GenerateAnswer) {
		return st, nil
	}
	if st.answer != "" {
		return st, nil
	}
	done := g.startTrace(st, NodeAnswer, nil)
	answer, err := g.conf.AnswerGenerator.Generate(ctx, &AnswerInput{
		Query:          st.query,
		RewrittenQuery: st.rewritten,
		Queries:        st.queries,
		Context:        st.context,
		Citations:      st.citations,
		Metadata:       st.request.Metadata,
	})
	if err != nil {
		done(err, nil)
		return nil, err
	}
	st.answer = answer
	done(nil, map[string]any{"answer_chars": utf8.RuneCountInString(st.answer)})
	return st, nil
}

func (g *Graph) verify(ctx context.Context, st *graphState) (*Result, error) {
	if st.answer == "" {
		return g.finish(st), nil
	}
	done := g.startTrace(st, NodeVerify, nil)
	verification, err := g.conf.AnswerVerifier.Verify(ctx, &AnswerInput{
		Query:          st.query,
		RewrittenQuery: st.rewritten,
		Queries:        st.queries,
		Context:        st.context,
		Citations:      st.citations,
		Metadata:       st.request.Metadata,
	}, st.answer)
	if err != nil {
		done(err, nil)
		return nil, err
	}
	st.verification = verification
	if verification != nil && !verification.Passed {
		output, fallbackErr := g.conf.Fallback.Handle(ctx, &FallbackInput{
			Query:          st.query,
			RewrittenQuery: st.rewritten,
			Queries:        st.queries,
			Reason:         FallbackReasonBadAnswer,
			Documents:      st.documents,
			Context:        st.context,
			Citations:      st.citations,
			Answer:         st.answer,
			Metadata:       st.request.Metadata,
		})
		if fallbackErr != nil {
			done(fallbackErr, nil)
			return nil, fallbackErr
		}
		if output != nil && output.Applied && output.Answer != "" {
			st.answer = output.Answer
		}
	}
	done(nil, map[string]any{"passed": verification == nil || verification.Passed})
	return g.finish(st), nil
}

func (g *Graph) finish(st *graphState) *Result {
	result := g.result(st)
	if g.conf.Evaluator != nil {
		st.evaluation = g.conf.Evaluator.Evaluate(result)
		result.Evaluation = st.evaluation
	}
	if st.trace != nil {
		st.trace.finish(len(st.queries), len(st.documents), utf8.RuneCountInString(st.context))
		result.Trace = st.trace
	}
	return result
}

func (g *Graph) result(st *graphState) *Result {
	return &Result{
		Query:          st.query,
		RewrittenQuery: st.rewritten,
		Queries:        st.queries,
		Documents:      st.documents,
		Retrievals:     st.retrievals,
		Context:        st.context,
		Citations:      st.citations,
		Answer:         st.answer,
		Verification:   st.verification,
		Trace:          st.trace,
		Evaluation:     st.evaluation,
		Metadata:       st.request.Metadata,
	}
}

func fallbackReason(st *graphState) string {
	if st == nil {
		return ""
	}
	if len(st.documents) == 0 {
		return FallbackReasonNoDocuments
	}
	if strings.TrimSpace(st.context) == "" {
		return FallbackReasonEmptyContext
	}
	return ""
}

func (g *Graph) startTrace(st *graphState, node string, fields map[string]any) func(error, map[string]any) {
	startedAt := time.Now()
	return func(err error, more map[string]any) {
		if st == nil || st.trace == nil {
			return
		}
		merged := make(map[string]any, len(fields)+len(more))
		for k, v := range fields {
			merged[k] = v
		}
		for k, v := range more {
			merged[k] = v
		}
		status := TraceStatusOK
		errText := ""
		if err != nil {
			status = TraceStatusError
			errText = err.Error()
		}
		event := TraceEvent{
			Node:       node,
			Status:     status,
			StartedAt:  startedAt,
			DurationMs: time.Since(startedAt).Milliseconds(),
			Error:      errText,
			Fields:     merged,
		}
		st.trace.Events = append(st.trace.Events, event)
		if g.conf.TraceObserver != nil {
			g.conf.TraceObserver.OnEvent(event)
		}
	}
}

type ContextConfig struct {
	MaxContextChars      int
	CitationContentChars int
	Separator            string
}

func BuildContext(docs []*schema.Document, conf *ContextConfig) (string, []Citation) {
	if conf == nil {
		conf = &ContextConfig{}
	}
	maxChars := conf.MaxContextChars
	if maxChars <= 0 {
		maxChars = 6000
	}
	citationChars := conf.CitationContentChars
	if citationChars <= 0 {
		citationChars = 240
	}
	separator := conf.Separator
	if separator == "" {
		separator = "\n\n"
	}

	parts := make([]string, 0, len(docs))
	citations := make([]Citation, 0, len(docs))
	used := 0
	for _, doc := range docs {
		if doc == nil || strings.TrimSpace(doc.Content) == "" {
			continue
		}
		content := strings.TrimSpace(doc.Content)
		part := fmt.Sprintf("[%d] %s", len(citations)+1, content)
		partLen := utf8.RuneCountInString(part)
		if used > 0 {
			partLen += utf8.RuneCountInString(separator)
		}
		if used+partLen > maxChars {
			remaining := maxChars - used
			if used > 0 {
				remaining -= utf8.RuneCountInString(separator)
			}
			if remaining <= 0 {
				break
			}
			prefix := fmt.Sprintf("[%d] ", len(citations)+1)
			contentLimit := remaining - utf8.RuneCountInString(prefix)
			if contentLimit <= 0 {
				break
			}
			part = prefix + truncate(content, contentLimit)
			partLen = utf8.RuneCountInString(part)
			if used > 0 {
				partLen += utf8.RuneCountInString(separator)
			}
		}

		parts = append(parts, part)
		used += partLen
		citations = append(citations, citationFromDocument(len(citations)+1, doc, citationChars))
		if used >= maxChars {
			break
		}
	}

	return strings.Join(parts, separator), citations
}

func citationFromDocument(index int, doc *schema.Document, contentChars int) Citation {
	meta := cloneMeta(doc.MetaData)
	title, _ := meta[enhance.MetaTitle].(string)
	summary, _ := meta[enhance.MetaSummary].(string)
	if summary == "" {
		summary = truncate(strings.TrimSpace(doc.Content), contentChars)
	}
	return Citation{
		Index:    index,
		ID:       doc.ID,
		Title:    title,
		Summary:  summary,
		Terms:    stringSlice(meta[enhance.MetaTerms]),
		Score:    doc.Score(),
		MetaData: meta,
	}
}

func cloneMeta(meta map[string]any) map[string]any {
	out := make(map[string]any, len(meta))
	for k, v := range meta {
		out[k] = v
	}
	return out
}

func stringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func truncate(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:max]))
}

func effectiveTopK(requestTopK int, defaultTopK int) int {
	if requestTopK > 0 {
		return requestTopK
	}
	return defaultTopK
}
