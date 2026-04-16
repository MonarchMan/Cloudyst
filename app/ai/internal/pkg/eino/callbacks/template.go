package callbacks

import (
	"context"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/document"
)

type (
	FetcherCallbackInput struct {
		// Source is the source of the documents.
		Source document.Source

		// Extra is the extra information for the callback.
		Extra map[string]any
	}

	FetcherCallbackOutput struct {
		Source   document.Source
		Extra    map[string]any
		FileInfo *FileInfo
	}
	FileInfo struct {
		Type string
		Size int64
	}
)

// ConvLoaderFetcherCallbackInput converts the callback input to the loader callback input.
func ConvLoaderFetcherCallbackInput(src callbacks.CallbackInput) *FetcherCallbackInput {
	switch t := src.(type) {
	case *FetcherCallbackInput:
		return t
	case document.Source:
		return &FetcherCallbackInput{
			Source: t,
		}
	default:
		return nil
	}
}

// ConvFetcherCallbackOutput converts the callback output to the fetcher callback output.
func ConvFetcherCallbackOutput(src callbacks.CallbackOutput) *FetcherCallbackOutput {
	switch t := src.(type) {
	case *FetcherCallbackOutput:
		return t
	case *FileInfo:
		return &FetcherCallbackOutput{
			FileInfo: t,
		}
	default:
		return nil
	}
}

type FetcherCallbackHandler struct {
	OnStart func(ctx context.Context, runInfo *callbacks.RunInfo, input *FetcherCallbackInput) context.Context
	OnEnd   func(ctx context.Context, runInfo *callbacks.RunInfo, output *FetcherCallbackOutput) context.Context
	OnError func(ctx context.Context, runInfo *callbacks.RunInfo, err error) context.Context
}

// Needed checks if the callback handler is needed for the given timing.
func (ch *FetcherCallbackHandler) Needed(ctx context.Context, runInfo *callbacks.RunInfo, timing callbacks.CallbackTiming) bool {
	switch timing {
	case callbacks.TimingOnStart:
		return ch.OnStart != nil
	case callbacks.TimingOnEnd:
		return ch.OnEnd != nil
	case callbacks.TimingOnError:
		return ch.OnError != nil
	default:
		return false
	}
}
