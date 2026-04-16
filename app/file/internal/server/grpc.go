package server

import (
	adminapi "api/api/file/admin/v1"
	fileapi "api/api/file/files/v1"
	shareapi "api/api/file/share/v1"
	slaveapi "api/api/file/slave/v1"
	sysapi "api/api/file/sys/v1"
	cm "api/external/middlewares"
	"file/internal/conf"
	"file/internal/service"
	"file/internal/service/admin"

	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// NewGRPCServer new a gRPC server.
func NewGRPCServer(bs *conf.Bootstrap, file *service.FileService, share *service.ShareService, sys *service.SysService,
	admin *admin.AdminService, slave *service.SlaveService, tracer trace.TracerProvider, propagator propagation.TextMapPropagator) *grpc.Server {
	traceServer := tracing.Server(
		tracing.WithTracerProvider(tracer),
		tracing.WithPropagator(propagator),
	)
	var opts = []grpc.ServerOption{
		grpc.Middleware(
			recovery.Recovery(),
			traceServer,
			cm.Logger(),
		),
		grpc.StreamMiddleware(
			traceServer,
			cm.Logger(),
		),
	}
	c := bs.Server
	if c.Grpc.Network != "" {
		opts = append(opts, grpc.Network(c.Grpc.Network))
	}
	if c.Grpc.Addr != "" {
		opts = append(opts, grpc.Address(c.Grpc.Addr))
	}
	if c.Grpc.Timeout != nil {
		opts = append(opts, grpc.Timeout(c.Grpc.Timeout.AsDuration()))
	}
	srv := grpc.NewServer(opts...)
	fileapi.RegisterFileServer(srv, file)
	shareapi.RegisterShareServer(srv, share)
	sysapi.RegisterSysServer(srv, sys)
	adminapi.RegisterAdminServer(srv, admin)
	slaveapi.RegisterSlaveServer(srv, slave)
	return srv
}
