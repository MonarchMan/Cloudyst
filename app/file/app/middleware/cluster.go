package middleware

import (
	commonpb "api/api/common/v1"
	filepb "api/api/file/common/v1"
	"common/auth"
	"common/constants"
	"common/request"
	"context"
	"file/internal/biz/cluster"
	"file/internal/biz/downloader"
	"file/internal/biz/setting"
	"file/internal/conf"
	"file/internal/data/types"
	"file/internal/pkg/utils"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
)

type SlaveNodeSettingGetter interface {
	// GetNodeSetting returns the node settings and its hash
	GetNodeSetting() (*filepb.NodeSetting, string)
}

var downloaderPool = sync.Map{}

func PrepareSlaveDownloader(bs *conf.Bootstrap, settings setting.Provider, ctxKey interface{}) middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			nodeSettings, hash := ctx.Value(ctxKey).(SlaveNodeSettingGetter).GetNodeSetting()

			// try to get downloader from pool
			if d, ok := downloaderPool.Load(hash); ok {
				ctx = context.WithValue(ctx, downloader.DownloaderCtxKey, d)
				return handler(ctx, req)
			}

			// create a downloader
			l := log.GetLogger()
			reqClient := request.NewClient(bs.Server.Sys.Mode, request.WithContext(ctx), request.WithLogger(l))
			d, err := cluster.NewDownloader(l, reqClient, settings, nodeSettings)
			if err != nil {
				return nil, commonpb.ErrorParamInvalid("Failed to create downloader: %v", err)
			}

			// store downloader to pool
			downloaderPool.Store(hash, d)
			ctx = context.WithValue(ctx, downloader.DownloaderCtxKey, d)
			return handler(ctx, req)
		}
	}
}

func SlaveRPCSignRequired(np cluster.NodePool) middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			nodeId := cluster.NodeIdFromContext(ctx)
			if nodeId == 0 {
				return nil, commonpb.ErrorParamInvalid("NodeID is required")
			}

			slaveNode, err := np.Get(ctx, types.NodeCapabilityNone, nodeId)
			if slaveNode == nil || slaveNode.IsMaster() {
				return nil, commonpb.ErrorParamInvalid("Unknown node ID: %s", err)
			}
			if err := CheckRpcRequest(ctx, slaveNode.AuthInstance()); err != nil {
				return nil, err
			}
			return next(ctx, req)
		}
	}
}

func CheckRpcRequest(ctx context.Context, instance auth.Auth) error {
	reqHeader, _, err := utils.HeaderFromContext(ctx)
	if err != nil {
		return err
	}

	var sign string
	if sign = reqHeader.Get("Authorization"); sign == "" {
		return errors.Unauthorized("authorization header is missing", "Authorization header is required")
	}
	sign = strings.TrimPrefix(sign, constants.TokenHeaderPrefixCr)

	var signedHeader []string
	for _, k := range reqHeader.Keys() {
		if strings.HasPrefix(k, constants.CrHeaderPrefix) && k != constants.CrHeaderPrefix+"Filename" {
			signedHeader = append(signedHeader, fmt.Sprintf("%s=%s", k, reqHeader.Get(k)))
		}
	}
	sort.Strings(signedHeader)

	// 读取所有待签名Header
	rawSignString := strings.Join(signedHeader, "&")
	return instance.Check(rawSignString, sign)
}
