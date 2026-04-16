package server

import (
	"ai/internal/conf"
	"ai/internal/service"
	adminapi "api/api/ai/admin/v1"
	chatapi "api/api/ai/chat/v1"
	imageapi "api/api/ai/image/v1"
	knowledgeapi "api/api/ai/knowledge/v1"
	roleapi "api/api/ai/role/v1"
	pbuser "api/api/user/users/v1"
	cm "api/external/middlewares"
	"api/external/middlewares/filters"
	"crypto/tls"
	"file/app/response"
	"fmt"

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

// NewHTTPServer new an HTTP server.
func NewHTTPServer(as *service.AdminService, cs *service.ChatService, ks *service.KnowledgeService, uc pbuser.UserClient,
	is *service.ImageService, rs *service.RoleService, bs *conf.Bootstrap,
	tracerProvider trace.TracerProvider, propagator propagation.TextMapPropagator, logger log.Logger) (*khttp.Server, error) {
	h := log.NewHelper(logger, log.WithMessageKey("server"))
	var opts = []khttp.ServerOption{
		khttp.Network("tcp"),
		khttp.Filter(
			gziphandler.GzipHandler,
		),
		khttp.Middleware(
			getMiddlewares(uc, tracerProvider, propagator)...,
		),
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

	srv := khttp.NewServer(opts...)
	adminapi.RegisterAdminHTTPServer(srv, as)
	chatapi.RegisterChatHTTPServer(srv, cs)
	knowledgeapi.RegisterKnowledgeHTTPServer(srv, ks)
	imageapi.RegisterImageHTTPServer(srv, is)
	roleapi.RegisterRoleHTTPServer(srv, rs)

	// route
	tracer := tracerProvider.Tracer("khttp-route")
	root := srv.Route("/", filters.Trace(tracer), filters.Logger())
	chatRoute := root.Group("/chat", filters.CurrentUser(uc), filters.LoginRequired())
	chatRoute.GET("/ai/chat/message/send-stream", cs.StreamChatHandler)
	return srv, nil
}

func getMiddlewares(uc pbuser.UserClient, tracer trace.TracerProvider, propagator propagation.TextMapPropagator) []middleware.Middleware {
	trace := tracing.Server(
		tracing.WithTracerProvider(tracer),
		tracing.WithPropagator(propagator),
	)
	isAdmin := selector.Server(cm.CheckAdminPermission()).
		Prefix("/ai.admin").
		Build()

	return []middleware.Middleware{recovery.Recovery(), trace, cm.Logger(), cm.CurrentUser(uc), cm.LoginRequired(), isAdmin}
}
