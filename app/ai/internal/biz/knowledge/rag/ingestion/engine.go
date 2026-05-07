package ingestion

import (
	"ai/ent"
	"ai/internal/biz/types"
	"ai/internal/conf"
	"ai/internal/data"
	"ai/internal/data/rpc"
	einointerrupt "ai/internal/pkg/eino/interrupt"
	"common/constants"
	"common/request"
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/document/parser"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/go-kratos/kratos/v2/log"
)

const (
	SemanticSplitterNode  = "semantic_splitter"
	RecursiveSplitterNode = "dynamic_splitter"
	MarkdownSplitterNode  = "markdown_splitter"
	IndexConcurrent       = 4
)

type IngestionGraphConfig struct {
	EnableEnhancer bool
	Enhancement    *EnhancementConfig
}

type IngestEngine struct {
	kdc     data.KnowledgeDocumentClient
	ksc     data.KnowledgeSegmentClient
	kc      data.KnowledgeClient
	fc      rpc.FileClient
	conf    *conf.Bootstrap
	parser  parser.Parser
	indexer indexer.Indexer
	store   *einointerrupt.CheckPointStore
	l       *log.Helper

	graph compose.Runnable[document.Source, []string]
}

type ProgressDocumentFunc func(doc *ent.AiKnowledgeDocument)

type IngestInterruptedError struct {
	CheckPointID string
	Info         *compose.InterruptInfo
}

func (e *IngestInterruptedError) Error() string {
	return fmt.Sprintf("ingest interrupted, checkpoint_id=%s", e.CheckPointID)
}

func (e *IngestInterruptedError) InterruptIDs() []string {
	if e == nil || e.Info == nil {
		return nil
	}
	ids := make([]string, 0, len(e.Info.InterruptContexts))
	for _, interruptCtx := range e.Info.InterruptContexts {
		ids = append(ids, interruptCtx.ID)
	}
	return ids
}

func IsIngestInterrupted(err error) (*IngestInterruptedError, bool) {
	var interrupted *IngestInterruptedError
	if errors.As(err, &interrupted) {
		return interrupted, true
	}
	return nil, false
}

func NewIngestEngine(kdc data.KnowledgeDocumentClient, ksc data.KnowledgeSegmentClient, kc data.KnowledgeClient,
	embedder embedding.Embedder, parser parser.Parser, indexer indexer.Indexer, fc rpc.FileClient, store *einointerrupt.CheckPointStore,
	conf *conf.Bootstrap, logger log.Logger) (*IngestEngine, error) {
	e := &IngestEngine{
		kdc:     kdc,
		ksc:     ksc,
		kc:      kc,
		fc:      fc,
		conf:    conf,
		parser:  parser,
		indexer: indexer,
		store:   store,
		l:       log.NewHelper(logger, log.WithMessageKey("biz-knowledge-ingestEngine")),
	}
	var err error
	e.graph, err = e.buildIngestionGraph(embedder)
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (e *IngestEngine) buildIngestionGraph(embedder embedding.Embedder) (compose.Runnable[document.Source, []string], error) {
	return e.buildIngestionGraphWithConfig(embedder, nil)
}

func (e *IngestEngine) BuildIngestionGraphWithConfig(embedder embedding.Embedder, graphConf *IngestionGraphConfig) (compose.Runnable[document.Source, []string], error) {
	return e.buildIngestionGraphWithConfig(embedder, graphConf)
}

func (e *IngestEngine) buildIngestionGraphWithConfig(embedder embedding.Embedder, graphConf *IngestionGraphConfig) (compose.Runnable[document.Source, []string], error) {
	loader, err := NewRemoteLoader(e.kdc, e.fc, &RemoteLoaderConfig{
		Client: request.NewClient(constants.MasterMode, request.WithLogger(e.l.Logger())),
		Parser: e.parser,
	})
	if err != nil {
		e.l.Errorf("failed to initialize ollama embedder: %v", err)
	}
	ctx := context.Background()
	semanticSplitter, err := NewSemanticSplitter(e.ksc, embedder)
	if err != nil {
		return nil, err
	}
	mhs, err := NewMarkdownSplitter(e.ksc)
	if err != nil {
		return nil, err
	}
	ds, err := NewDynamicSplitter(e.ksc, &DynamicSplitterConfig{})
	if err != nil {
		return nil, err
	}
	enableEnhancer := graphConf != nil && graphConf.EnableEnhancer
	var enhancer document.Transformer
	if enableEnhancer {
		enhancer = NewEnhancer(graphConf.Enhancement)
	}

	g := compose.NewGraph[document.Source, []string]()

	g.AddLoaderNode("loader", loader)
	if enableEnhancer {
		g.AddDocumentTransformerNode(EnhancerNode, enhancer)
	}
	g.AddDocumentTransformerNode(SemanticSplitterNode, semanticSplitter)
	g.AddDocumentTransformerNode(MarkdownSplitterNode, mhs)
	g.AddDocumentTransformerNode(RecursiveSplitterNode, ds)
	g.AddIndexerNode("indexer", e.indexer)

	branchStart := "loader"
	if enableEnhancer {
		branchStart = EnhancerNode
	}
	g.AddBranch(branchStart, compose.NewGraphBranch(
		func(ctx context.Context, docs []*schema.Document) (endNode string, err error) {
			info := GetDocumentInfo(ctx)
			switch info.SplitStrategy {
			case types.StrategyMarkdownHeader:
				return MarkdownSplitterNode, nil
			case types.StrategySemantic:
				return SemanticSplitterNode, nil
			default:
				return RecursiveSplitterNode, nil
			}
		},
		map[string]bool{
			MarkdownSplitterNode:  true,
			SemanticSplitterNode:  true,
			RecursiveSplitterNode: true,
		},
	))

	g.AddEdge(compose.START, "loader")
	if enableEnhancer {
		g.AddEdge("loader", EnhancerNode)
	}
	g.AddEdge(SemanticSplitterNode, "indexer")
	g.AddEdge(RecursiveSplitterNode, "indexer")
	g.AddEdge(MarkdownSplitterNode, "indexer")
	g.AddEdge("indexer", compose.END)

	compileOptions := make([]compose.GraphCompileOption, 0, 1)
	if e.store != nil {
		compileOptions = append(compileOptions, compose.WithCheckPointStore(e.store))
	}
	return g.Compile(ctx, compileOptions...)
}

// Ingest 文档索引, info 文档信息, url 文档下载链接, progressDocumentFunc 文档进度回调函数， opts 索引选项
func (e *IngestEngine) Ingest(ctx context.Context, info *DocumentInfo, url string, progressDocumentFunc ProgressDocumentFunc, opts ...IngestOption) error {
	handler := callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
			e.l.Debugf("start to ingest document: %v", info)
			return ctx
		}).
		OnErrorFn(func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
			e.l.Debugf("failed to ingest document: %v", err)
			return ctx
		}).
		Build()
	ctx = WithDocumentInfo(ctx, info)

	ingestOptions := &ingestOptions{
		checkPointID: IngestCheckPointID(info.ID),
	}
	for _, opt := range opts {
		opt(ingestOptions)
	}
	if len(ingestOptions.resumeData) > 0 {
		ctx = compose.BatchResumeWithData(ctx, ingestOptions.resumeData)
	}

	callOptions := []compose.Option{compose.WithCallbacks(handler)}
	if e.store != nil && ingestOptions.checkPointID != "" {
		callOptions = append(callOptions, compose.WithCheckPointID(ingestOptions.checkPointID))
	}
	if ingestOptions.forceNewRun {
		callOptions = append(callOptions, compose.WithForceNewRun())
	}

	_, runErr := e.graph.Invoke(ctx, document.Source{URI: url}, callOptions...)
	if interruptInfo, ok := compose.ExtractInterruptInfo(runErr); ok {
		return &IngestInterruptedError{
			CheckPointID: ingestOptions.checkPointID,
			Info:         interruptInfo,
		}
	}

	var doc *ent.AiKnowledgeDocument
	if runErr != nil {
		e.l.Errorf("failed to ingest document %d: %v", info.ID, runErr)
		var err error
		doc, err = e.kdc.UpdateProcess(ctx, info.ID, types.DocumentFailed)
		if err != nil {
			return err
		}
	} else {
		var err error
		if info.IndexStats != nil {
			info.IndexStats.Process = types.DocumentSuccess
			doc, err = e.kdc.UpdateIndexStats(ctx, info.ID, info.IndexStats)
		} else {
			doc, err = e.kdc.UpdateProcess(ctx, info.ID, types.DocumentSuccess)
		}
		if err != nil {
			return err
		}
	}

	if progressDocumentFunc != nil && doc != nil {
		progressDocumentFunc(doc)
	}
	return runErr
}

func (e *IngestEngine) BatchIngest(ctx context.Context, infos []DocumentInfo, urls []string, progressDocumentFunc ProgressDocumentFunc) (int, error) {
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		failed int
		err    error
	)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, IndexConcurrent)
	for i, info := range infos {
		select {
		case <-ctx.Done():
			return failed, err
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(info DocumentInfo, downloadUrl string) {
			defer func() {
				<-sem
				wg.Done()
			}()

			if ingestErr := e.Ingest(ctx, &info, downloadUrl, progressDocumentFunc); ingestErr != nil {
				mu.Lock()
				defer mu.Unlock()
				if _, ok := IsIngestInterrupted(ingestErr); ok {
					if err == nil {
						err = ingestErr
					}
					cancel()
					return
				}
				if ctx.Err() != nil && err != nil {
					return
				}
				e.l.Warnf("Failed to index file %d (%s): %s", info.ID, info.Name, ingestErr)
				failed++
			}
		}(info, urls[i])
	}
	wg.Wait()
	return failed, err
}
