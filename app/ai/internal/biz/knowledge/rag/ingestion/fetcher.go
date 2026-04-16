package ingestion

import (
	"ai/internal/biz/types"
	"ai/internal/data"
	"ai/internal/data/rpc"
	icb "ai/internal/pkg/eino/callbacks"
	"ai/internal/pkg/eino/doc"
	"ai/internal/pkg/utils"
	"common/constants"
	"common/request"
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/document"
)

type (
	FetcherNode struct {
		kdc  data.KnowledgeDocumentClient
		fc   rpc.FileClient
		conf *RemoteFetcherConfig
	}

	RemoteFetcherConfig struct {
		// optional, default: request.NewClient(constants.MasterMode)
		Client request.Client
	}
)

func NewFetcher(kdc data.KnowledgeDocumentClient, fc rpc.FileClient, conf *RemoteFetcherConfig) doc.Fetcher {
	if conf.Client == nil {
		conf.Client = request.NewClient(constants.MasterMode)
	}

	return &FetcherNode{
		kdc:  kdc,
		fc:   fc,
		conf: conf,
	}
}

func (n *FetcherNode) Fetch(ctx context.Context, src document.Source) (info *doc.FileInfo, err error) {
	ctx = callbacks.EnsureRunInfo(ctx, n.GetType(), components.ComponentOfLoader)
	ctx = callbacks.OnStart(ctx, &document.LoaderCallbackInput{
		Source: src,
	})
	defer func() {
		if err != nil {
			_ = callbacks.OnError(ctx, err)
		}
	}()

	info, err = n.fetch(ctx, src)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch content from uri [%s]: %w", src.URI, err)
	}

	documentInfo := GetDocumentInfo(ctx)
	_, err = n.kdc.UpdateContentLenVersionAndProcess(ctx, documentInfo.ID, int(info.Size), documentInfo.Version, types.DocumentProcessing)
	if err != nil {
		return nil, err
	}
	//docInfo.Model = d
	_ = callbacks.OnEnd(ctx, &icb.FetcherCallbackOutput{
		Source: src,
		Extra:  info.Metadata,
		FileInfo: &icb.FileInfo{
			Type: info.Type,
			Size: info.Size,
		},
	})
	return info, nil
}

// fetch file content from uri
// if uri is multiple urls, it should be separated by ";"
func (n *FetcherNode) fetch(ctx context.Context, src document.Source) (info *doc.FileInfo, err error) {
	// 1. 获取文件信息
	fileInfo, err := n.fc.GetFileInfo(ctx, src.URI)
	if err != nil {
		return nil, err
	}
	info = &doc.FileInfo{
		Type:     filepath.Ext(fileInfo.Name),
		Metadata: utils.StringMapToAnyMap(fileInfo.Metadata),
		Size:     fileInfo.Size,
	}

	// 2. 获取文件URL，支持多个URL
	uris := strings.Split(src.URI, ";")
	urlResp, err := n.fc.GetFileUrl(ctx, uris)
	if err != nil {
		return nil, err
	}
	url := urlResp.Urls[0].Url

	// 3. 下载文件内容
	response := n.conf.Client.Request(http.MethodGet, url, nil, request.WithContext(ctx)).
		CheckHTTPResponse(http.StatusOK)
	if response.Err != nil {
		return nil, response.Err
	}
	info.Reader = response.Response.Body
	return info, nil
}

func (n *FetcherNode) GetType() string {
	return "RemoteFetcher"
}

func (n *FetcherNode) IsCallbacksEnabled() bool {
	return true
}
