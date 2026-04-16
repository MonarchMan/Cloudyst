package doc

import (
	"context"
	"io"

	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/compose"
)

const ComponentOfFetcher components.Component = "fetcher"

type (
	Fetcher interface {
		Fetch(ctx context.Context, src document.Source) (*FileInfo, error)
	}

	FileInfo struct {
		Metadata map[string]any
		Type     string
		Size     int64
		Reader   io.ReadCloser
	}
)
type (
	RemoteFetcher struct {
	}
)

func AddFetcherNode[I, O any](g compose.Graph[I, O], key string, fetcher Fetcher, opts ...compose.GraphAddNodeOpt) error {
	return g.AddLambdaNode(key, compose.InvokableLambda(func(ctx context.Context, input document.Source) (output *FileInfo, err error) {
		return fetcher.Fetch(ctx, input)
	}), opts...)
}
