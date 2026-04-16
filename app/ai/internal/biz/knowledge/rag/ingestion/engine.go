package ingestion

import (
	"ai/internal/biz/types"
	"ai/internal/conf"
	"ai/internal/data"
	"ai/internal/data/rpc"
	"common/constants"
	"common/request"
	"context"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/document/parser"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/go-kratos/kratos/v2/log"
	"golang.org/x/sync/errgroup"
)

const (
	SemanticSplitterNode  = "semantic_splitter"
	RecursiveSplitterNode = "dynamic_splitter"
	MarkdownSplitterNode  = "markdown_splitter"
)

type IngestEngine struct {
	kdc     data.KnowledgeDocumentClient
	ksc     data.KnowledgeSegmentClient
	kc      data.KnowledgeClient
	fc      rpc.FileClient
	conf    *conf.Bootstrap
	parser  parser.Parser
	indexer indexer.Indexer
	l       *log.Helper

	graph compose.Runnable[document.Source, []string]
}

func NewIngestEngine(kdc data.KnowledgeDocumentClient, ksc data.KnowledgeSegmentClient, kc data.KnowledgeClient,
	embedder embedding.Embedder, parser parser.Parser, indexer indexer.Indexer, conf *conf.Bootstrap, logger log.Logger) (*IngestEngine, error) {
	e := &IngestEngine{
		kdc:     kdc,
		ksc:     ksc,
		kc:      kc,
		conf:    conf,
		parser:  parser,
		indexer: indexer,
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

	g := compose.NewGraph[document.Source, []string]()

	g.AddLoaderNode("loader", loader)
	g.AddDocumentTransformerNode(SemanticSplitterNode, semanticSplitter)
	g.AddDocumentTransformerNode(MarkdownSplitterNode, mhs)
	g.AddDocumentTransformerNode(RecursiveSplitterNode, ds)
	g.AddIndexerNode("indexer", e.indexer)

	g.AddBranch("loader", compose.NewGraphBranch(
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
	g.AddEdge(SemanticSplitterNode, "indexer")
	g.AddEdge(RecursiveSplitterNode, "indexer")
	g.AddEdge("indexer", compose.END)
	return g.Compile(ctx)
}

func (e *IngestEngine) Ingest(ctx context.Context, info *DocumentInfo, url string) error {
	ctx = WithDocumentInfo(ctx, info)
	_, graphErr := e.graph.Invoke(ctx, document.Source{URI: url})
	var err error
	if graphErr != nil {
		e.l.Errorf("failed to rag document %d: %v", info.ID, graphErr)
		_, err = e.kdc.UpdateProcess(ctx, info.ID, types.DocumentFailed)
	}

	_, err = e.kdc.UpdateProcess(ctx, info.ID, types.DocumentSuccess)
	return err
}

func (e *IngestEngine) BatchIngest(ctx context.Context, infos []*DocumentInfo, urls []string) error {
	var eg errgroup.Group
	// 设置最大并发度，比如同时最多下载和切分 5 个文件，保护服务器内存
	eg.SetLimit(5)

	for i, info := range infos {
		eg.Go(func() error {
			err := e.Ingest(ctx, info, urls[i])
			e.l.Errorf("failed to update document %d status: %v", info.ID, err)
			return err
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	return nil
}
