package callbacks

import (
	icb "ai/internal/pkg/eino/callbacks"
	"ai/internal/pkg/eino/doc"
	"context"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/schema"
	ucb "github.com/cloudwego/eino/utils/callbacks"
)

type HandlerHelper struct {
	*ucb.HandlerHelper
	fetcherHandler *icb.FetcherCallbackHandler
}

func NewHandlerHelper() *HandlerHelper {
	return &HandlerHelper{
		HandlerHelper: ucb.NewHandlerHelper(),
	}
}

func (h *HandlerHelper) Fetcher(handler *icb.FetcherCallbackHandler) *HandlerHelper {
	h.fetcherHandler = handler
	return h
}

func (h *HandlerHelper) Handler() callbacks.Handler {
	return &handlerTemplate{
		HandlerHelper: h,
		Handler:       h.Handler(),
	}
}

type handlerTemplate struct {
	*HandlerHelper
	callbacks.Handler
}

func (t *handlerTemplate) OnStart(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
	switch info.Component {
	case doc.ComponentOfFetcher:
		return t.fetcherHandler.OnStart(ctx, info, icb.ConvLoaderFetcherCallbackInput(input))
	default:
		return t.Handler.OnStart(ctx, info, input)
	}
}
func (t *handlerTemplate) OnEnd(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
	switch info.Component {
	case doc.ComponentOfFetcher:
		return t.fetcherHandler.OnEnd(ctx, info, icb.ConvFetcherCallbackOutput(output))
	default:
		return t.Handler.OnEnd(ctx, info, output)
	}
}

func (t *handlerTemplate) OnError(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
	switch info.Component {
	case doc.ComponentOfFetcher:
		return t.fetcherHandler.OnError(ctx, info, err)
	default:
		return t.Handler.OnError(ctx, info, err)
	}
}
func (t *handlerTemplate) OnStartWithStreamInput(ctx context.Context, info *callbacks.RunInfo,
	input *schema.StreamReader[callbacks.CallbackInput]) context.Context {
	return t.Handler.OnStartWithStreamInput(ctx, info, input)
}
func (t *handlerTemplate) OnEndWithStreamOutput(ctx context.Context, info *callbacks.RunInfo,
	output *schema.StreamReader[callbacks.CallbackOutput]) context.Context {
	return t.Handler.OnEndWithStreamOutput(ctx, info, output)
}
