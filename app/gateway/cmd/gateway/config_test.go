package main

import (
	"testing"

	v1 "gateway/api/gateway/config/v1"
	circuitbreakerv1 "gateway/api/gateway/middleware/circuitbreaker/v1"
	corsv1 "gateway/api/gateway/middleware/cors/v1"
	rewritev1 "gateway/api/gateway/middleware/rewrite/v1"
	tracingv1 "gateway/api/gateway/middleware/tracing/v1"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
)

func equalTo() *v1.Gateway {
	return &v1.Gateway{
		Name:    "files",
		Version: "v1",
		// Hosts: []string{
		// 	"localhost",
		// 	"127.0.0.1",
		// },
		Endpoints: []*v1.Endpoint{
			{
				Path:     "/files/*",
				Protocol: v1.Protocol_HTTP,
				Host:     "localhost",
				Timeout:  &durationpb.Duration{Seconds: 1},
				Backends: []*v1.Backend{
					{
						Target: "127.0.0.1:8000",
					},
				},
				Middlewares: []*v1.Middleware{
					{
						Name: "circuitbreaker",
						Options: asAny(&circuitbreakerv1.CircuitBreaker{
							Trigger: &circuitbreakerv1.CircuitBreaker_SuccessRatio{
								SuccessRatio: &circuitbreakerv1.SuccessRatio{
									Success: 0.6,
									Request: 1,
									Bucket:  10,
									Window:  &durationpb.Duration{Seconds: 3},
								},
							},
							Action: &circuitbreakerv1.CircuitBreaker_BackupService{
								BackupService: &circuitbreakerv1.BackupService{
									Endpoint: &v1.Endpoint{
										Backends: []*v1.Backend{
											{
												Target: "127.0.0.1:8001",
											},
										},
									},
								},
							},
							AssertCondtions: []*v1.Condition{
								{
									Condition: &v1.Condition_ByStatusCode{
										ByStatusCode: "200",
									},
								},
							},
						}),
					},
					{
						Name:    "rewrite",
						Options: asAny(&rewritev1.Rewrite{}),
					},
				},
			},
			{
				Path:     "/files.v1.Greeter/*",
				Method:   "POST",
				Protocol: v1.Protocol_GRPC,
				Timeout:  &durationpb.Duration{Seconds: 1},
				Backends: []*v1.Backend{
					{
						Target: "127.0.0.1:9000",
					},
				},
				Retry: &v1.Retry{
					Attempts:      3,
					PerTryTimeout: &durationpb.Duration{Nanos: 100000000},
					Conditions: []*v1.Condition{
						{Condition: &v1.Condition_ByStatusCode{ByStatusCode: "502-504"}},
						{Condition: &v1.Condition_ByHeader{ByHeader: &v1.ConditionHeader{
							Name:  "Grpc-Status",
							Value: "14",
						}}},
					},
				},
			},
			{
				Path:     "/ws",
				Timeout:  &durationpb.Duration{Seconds: 10},
				Protocol: v1.Protocol_HTTP,
				Backends: []*v1.Backend{
					{
						Target: "127.0.0.1:18080",
					},
				},
				Stream: true,
			},
		},
		Middlewares: []*v1.Middleware{
			{
				Name: "tracing",
				Options: asAny(&tracingv1.Tracing{
					Endpoint: &tracingv1.Tracing_HttpEndpoint{
						HttpEndpoint: "localhost:4318",
					},
				}),
			},
			{
				Name: "logging",
			},
			{
				Name: "transcoder",
			},
			{
				Name: "cors",
				Options: asAny(&corsv1.Cors{
					AllowCredentials: true,
					AllowOrigins:     []string{".google.com"},
					AllowMethods:     []string{"GET", "POST", "OPTIONS"},
				}),
			},
			{
				Name: "streamrecorder",
			},
		},
	}
}

func asAny(in proto.Message) *anypb.Any {
	out, err := anypb.New(in)
	if err != nil {
		panic(err)
	}
	return out
}

func TestConfigUnmarshaler(t *testing.T) {
	cfg := config.New(
		config.WithSource(
			file.NewSource("common.yaml"),
		),
	)
	if err := cfg.Load(); err != nil {
		t.Fatal(err)
	}
	gateway := &v1.Gateway{}
	if err := cfg.Scan(gateway); err != nil {
		t.Fatal(err)
	}

	left, err := protojson.Marshal(gateway)
	if err != nil {
		t.Fatal(err)
	}
	right, err := protojson.Marshal(equalTo())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("gateway config:\nloaded: %s\nshould equal to: %s\n", left, right)

	if !proto.Equal(gateway, equalTo()) {
		t.Errorf("inconsistent gateway config")
	}
}
