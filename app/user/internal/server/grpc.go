package server

import (
	"api/api/user/users/v1"
	im "user/app/middleware"
	"user/internal/conf"
	"user/internal/service"

	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// NewGRPCServer new a gRPC server.
func NewGRPCServer(c *conf.Bootstrap, userService *service.UserService, tracer trace.TracerProvider,
	propagator propagation.TextMapPropagator) *grpc.Server {
	var opts = []grpc.ServerOption{
		grpc.Middleware(
			recovery.Recovery(),
			tracing.Server(
				tracing.WithTracerProvider(tracer),
				tracing.WithPropagator(propagator),
			),
			im.Logger(),
		),
	}
	if c.Server.Grpc.Network != "" {
		opts = append(opts, grpc.Network(c.Server.Grpc.Network))
	}
	if c.Server.Grpc.Addr != "" {
		opts = append(opts, grpc.Address(c.Server.Grpc.Addr))
	}
	if c.Server.Grpc.Timeout != nil {
		opts = append(opts, grpc.Timeout(c.Server.Grpc.Timeout.AsDuration()))
	}
	srv := grpc.NewServer(opts...)
	v1.RegisterUserServer(srv, userService)
	return srv
}
