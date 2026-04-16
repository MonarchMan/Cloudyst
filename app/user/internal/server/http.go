package server

import (
	pb "api/api/common/v1"
	adminapi "api/api/user/admin/v1"
	userpb "api/api/user/common/v1"
	deviceapi "api/api/user/device/v1"
	sessionapi "api/api/user/session/v1"
	siteapi "api/api/user/site/v1"
	userapi "api/api/user/users/v1"
	"api/external/middlewares/filters"
	"common/auth"
	xerrors "common/errors"
	im "user/app/middleware"
	"user/app/response"
	"user/internal/conf"
	"user/internal/data"
	"user/internal/service"

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
	xerrors.Register(userpb.ErrorReason_value)
	xerrors.Register(adminapi.ErrorReason_value)
}

// NewHTTPServer new an HTTP server.
func NewHTTPServer(c *conf.Bootstrap, user *service.UserService, session *service.SessionService, admin *service.AdminService,
	deviceService *service.DeviceService, site *service.SiteService, uc data.UserClient, tracerProvider trace.TracerProvider,
	propagator propagation.TextMapPropagator, instance auth.Auth) *khttp.Server {
	var opts = []khttp.ServerOption{
		khttp.Middleware(
			getMiddlewares(uc, tracerProvider, propagator, instance)...,
		),
		khttp.ResponseEncoder(response.ResponseEncoder),
		khttp.ErrorEncoder(response.ErrorEncoder),
	}
	if c.Server.Sys.Addr != "" {
		opts = append(opts, khttp.Address(c.Server.Sys.Addr))
	}
	if c.Server.Sys.Timeout != nil {
		opts = append(opts, khttp.Timeout(c.Server.Sys.Timeout.AsDuration()))
	}
	srv := khttp.NewServer(opts...)
	sessionapi.RegisterSessionHTTPServer(srv, session)
	userapi.RegisterUserHTTPServer(srv, user)
	userapi.RegisterUserNoAuthHTTPServer(srv, user)
	deviceapi.RegisterDeviceHTTPServer(srv, deviceService)
	siteapi.RegisterSiteHTTPServer(srv, site)
	adminapi.RegisterAdminHTTPServer(srv, admin)

	// route
	tracer := tracerProvider.Tracer("http-route")
	route := srv.Route("/user", filters.Trace(tracer), filters.Logger())
	route.GET("/avatar/{id}", user.GetAvatar)

	return srv
}

func getMiddlewares(uc data.UserClient, tracer trace.TracerProvider, propagator propagation.TextMapPropagator,
	auth auth.Auth) []middleware.Middleware {
	trace := tracing.Server(
		tracing.WithTracerProvider(tracer),
		tracing.WithPropagator(propagator),
	)
	sr := selector.Server(im.SignRequired(auth)).
		Path(userapi.OperationUserNoAuthActivate).
		Build()

	lr := selector.Server(im.LoginRequired()).
		Prefix("/user.admin", "/user.device", "/user.users.v1.User/").
		Path().
		Build()
	admin := selector.Server(im.CheckAdminPermission()).
		Prefix("/user.admin").
		Build()
	return []middleware.Middleware{recovery.Recovery(), trace, im.Logger(), sr, im.CurrentUser(uc), lr, admin}
}
