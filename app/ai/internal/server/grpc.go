package server

import (
	"ai/internal/conf"
	"ai/internal/service"
	adminapi "api/api/ai/admin/v1"
	chatapi "api/api/ai/chat/v1"
	imageapi "api/api/ai/image/v1"
	knowledgeapi "api/api/ai/knowledge/v1"
	roleapi "api/api/ai/role/v1"
	cm "api/external/middlewares"

	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// NewGRPCServer new a gRPC server.
func NewGRPCServer(bs *conf.Bootstrap, admin *service.AdminService, chat *service.ChatService, knowledge *service.KnowledgeService,
	image *service.ImageService, tracer trace.TracerProvider, propagator propagation.TextMapPropagator, role *service.RoleService) *grpc.Server {
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
	adminapi.RegisterAdminServer(srv, admin)
	chatapi.RegisterChatServer(srv, chat)
	knowledgeapi.RegisterKnowledgeServer(srv, knowledge)
	imageapi.RegisterImageServer(srv, image)
	roleapi.RegisterRoleServer(srv, role)
	return srv
}
