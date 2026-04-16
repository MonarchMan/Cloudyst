package server

import (
	pb "api/api/common/v1"
	adminapi "api/api/file/admin/v1"
	callbackapi "api/api/file/callback/v1"
	filepb "api/api/file/common/v1"
	fileapi "api/api/file/files/v1"
	shareapi "api/api/file/share/v1"
	explorerapi "api/api/file/workflow/v1"
	pbuser "api/api/user/users/v1"
	cm "api/external/middlewares"
	"api/external/middlewares/filters"
	"common/auth"
	"common/cache"
	xerrors "common/errors"
	"common/hashid"
	"crypto/tls"
	im "file/app/middleware"
	"file/app/response"
	"file/internal/biz/setting"
	"file/internal/conf"
	"file/internal/data/types"
	"file/internal/service"
	"file/internal/service/admin"
	"file/internal/service/webdav"
	"fmt"
	"net/http"
	"os"

	"github.com/NYTimes/gziphandler"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/selector"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func init() {
	xerrors.RegisterCommon(pb.ErrorReason_value)
	xerrors.Register(filepb.ErrorReason_value)
}

// NewHTTPServer new an khttp server.
func NewHTTPServer(bs *conf.Bootstrap, callback *service.CallbackService, file *service.FileService, share *service.ShareService,
	wopi *service.WopiService, workflow *service.WorkflowService, dav *webdav.WebDAVService, admin *admin.AdminService,
	hasher hashid.Encoder, kv cache.Driver, uc pbuser.UserClient, settings setting.Provider, generalAuth auth.Auth,
	tracerProvider trace.TracerProvider, propagator propagation.TextMapPropagator, l log.Logger) (*khttp.Server, error) {
	h := log.NewHelper(l, log.WithMessageKey("server"))

	var opts = []khttp.ServerOption{
		khttp.Network("tcp"),
		khttp.Filter(
			gziphandler.GzipHandler,
		),
		khttp.Middleware(
			getMiddlewares(kv, uc, tracerProvider, propagator)...,
		//middleware.CurrentUser(uc),
		),
		khttp.RequestDecoder(OctetStreamSkipper),
		khttp.ResponseEncoder(response.ResponseEncoder),
		khttp.ErrorEncoder(response.ErrorEncoder),
	}
	c := bs.Server
	if c.Sys.Addr != "" {
		opts = append(opts, khttp.Address(c.Sys.Addr))
	}
	if c.Sys.Timeout != nil {
		opts = append(opts, khttp.Timeout(c.Sys.Timeout.AsDuration()))
	}
	if c.Ssl != nil && c.Ssl.CertPath != "" {
		h.Info("Listening to %q", c.Ssl.Addr)
		cert, err := tls.LoadX509KeyPair(c.Ssl.CertPath, c.Ssl.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load ssl certificate: %s", err)
		}
		opts = append(opts, khttp.Address(c.Ssl.Addr), khttp.TLSConfig(&tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}))
	}

	if c.Unix != nil && c.Unix.Addr != "" {
		if err := verifyUnixConfig(c.Unix, h); err != nil {
			return nil, fmt.Errorf("failed to verify unix config: %s", err)
		}
		opts = append(opts, khttp.Network("unix"))
		opts = append(opts, khttp.Address(c.Unix.Addr))
	}
	srv := khttp.NewServer(opts...)

	callbackapi.RegisterCallbackHTTPServer(srv, callback)
	explorerapi.RegisterWorkflowHTTPServer(srv, workflow)
	fileapi.RegisterFileHTTPServer(srv, file)
	shareapi.RegisterShareHTTPServer(srv, share)
	adminapi.RegisterAdminHTTPServer(srv, admin)

	handlePrefix(srv, "/dav", dav, im.WebDavAuth(uc))

	// route
	tracer := tracerProvider.Tracer("http-route")
	root := srv.Route("/", filters.Trace(tracer), filters.Logger())
	// file
	fileRoute := root.Group("/file", filters.CurrentUser(uc), im.SignRequired(generalAuth))
	fileRoute.GET("/content/{id}/{speed}/{name}", file.ServeEntity)
	fileRoute.HEAD("/content/{id}/{speed}/{name}", file.ServeEntity)
	fileRoute.PUT("/user/avatar", file.UploadAvatar)
	// wopi
	wopiRoute := fileRoute.Group("/wopi", im.ViewerSessionValidator(hasher, kv, uc, settings))
	initWopiRoute(wopiRoute, wopi)
	// callback
	callbackRoute := root.Group("/callback")
	initCallbackRoute(callbackRoute, callback, kv, bs, uc)
	// share
	shareRoute := root.Group("/s")
	shareRoute.GET("/{id}", share.Redirect)
	shareRoute.GET("/{id}/{password}", share.Redirect)

	return srv, nil
}

func verifyUnixConfig(unix *conf.Unix, helper *log.Helper) error {
	if _, err := os.Stat(unix.Addr); err != nil {
		if err = os.Remove(unix.Addr); err != nil {
			return fmt.Errorf("failed to delete socket file %q: %w", unix.Addr, err)
		}
		helper.Info("Listening to %q", unix.Addr)
		defer os.Remove(unix.Addr)
		if unix.Perm > 0 {
			err = os.Chmod(unix.Addr, os.FileMode(unix.Perm))
			if err != nil {
				helper.Warn("Failed to set permission to %q for socket file %q: %s", unix.Perm, unix.Addr, err)
			}
		}
	}
	return nil
}

func initCallbackRoute(route *khttp.Router, callback *service.CallbackService, kv cache.Driver, c *conf.Bootstrap, uc pbuser.UserClient) {
	route.POST("/oss/{session_id}/{key}", callback.OssCallback,
		im.UseUploadSession(types.PolicyTypeOss, kv, uc),
		im.OssCallbackAuthFilter(kv, c))
	route.POST("/upyun/{session_id}/{key}", callback.UpyunCallback,
		im.UseUploadSession(types.PolicyTypeUpyun, kv, uc),
		im.UpyunCallbackAuth())
	route.POST("/onedrive/{session_id}/{key}", callback.OnedriveCallback,
		im.UseUploadSession(types.PolicyTypeOd, kv, uc))
	//route.GET("/googledrive/{session_id}/{key}", callback.GdriveCallback)
	route.GET("/cos/{session_id}/{key}", callback.CosCallback,
		im.UseUploadSession(types.PolicyTypeCos, kv, uc))
	route.GET("/s3/{session_id}/{key}", callback.AwsS3Callback,
		im.UseUploadSession(types.PolicyTypeS3, kv, uc))
	route.GET("/ks3/{session_id}/{key}", callback.Ks3Callback,
		im.UseUploadSession(types.PolicyTypeKs3, kv, uc))
	route.POST("/obs/{session_id}/{key}", callback.ObsCallback,
		im.UseUploadSession(types.PolicyTypeObs, kv, uc))
	route.POST("/qiniu/{session_id}/{key}", callback.QiniuCallback,
		im.UseUploadSession(types.PolicyTypeQiniu, kv, uc),
		im.QiniuCallbackAuth())
}

func initWopiRoute(route *khttp.Router, wopi *service.WopiService) {
	route.GET("/{id}", wopi.CheckFileInfo)
	route.GET("/{id}/contents", wopi.GetFile)
	route.POST("/{id}/contents", wopi.PutFile)
	route.POST("/{id}", wopi.ModifyFile)
}

func getMiddlewares(kv cache.Driver, uc pbuser.UserClient, tracer trace.TracerProvider,
	propagator propagation.TextMapPropagator) []middleware.Middleware {
	trace := tracing.Server(
		tracing.WithTracerProvider(tracer),
		tracing.WithPropagator(propagator),
	)
	cu := selector.Server(im.CurrentUser(uc)).
		Prefix("/file.callback", "/file.workflow", "/file.share", "/file.admin", "/file.files").
		Build()
	lr := selector.Server(im.LoginRequired()).
		Regex("/file\\.share[^/]*/(?!GetShare/).*$").
		Prefix("/file.explorer", "/file.admin").
		Build()
	isAdmin := selector.Server(im.CheckAdminPermission()).
		Prefix("/file.admin").
		Build()
	us := selector.Server(im.UseUploadSessionMid(types.PolicyTypeRemote, kv, uc), im.RemoteCallbackAuth()).
		Prefix("/file.callback").
		Build()

	return []middleware.Middleware{recovery.Recovery(), trace, cm.Logger(), us, cu, lr, isAdmin}
}

func handlePrefix(server *khttp.Server, prefix string, h http.Handler, filters ...khttp.FilterFunc) {
	server.HandlePrefix(prefix, khttp.FilterChain(filters...)(h))
}

func OctetStreamSkipper(r *khttp.Request, v interface{}) error {
	if r.Header.Get("Content-Type") == "application/octet-stream" {
		// 只绑定 URL 参数，完全忽略 Body
		return khttp.DefaultRequestVars(r, v)
	}
	return khttp.DefaultRequestDecoder(r, v)
}
