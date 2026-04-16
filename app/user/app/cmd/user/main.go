package main

import (
	"flag"
	"os"
	"user/internal/conf"

	"github.com/go-kratos/kratos/contrib/config/consul/v2"
	creg "github.com/go-kratos/kratos/contrib/registry/consul/v2"
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-kratos/kratos/v2/transport/http"
	"github.com/hashicorp/consul/api"
	"github.com/ua-parser/uap-go/uaparser"

	_ "go.uber.org/automaxprocs"
)

// go build -ldflags "-X main.Version=x.y.z"
var (
	// flagconf is the config flag.
	flagconf   string
	flagConsul string

	id, _ = os.Hostname()
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../../configs", "config path, eg: -conf common.yaml")
	flag.StringVar(&flagConsul, "consul", "127.0.0.1:8500", "consul address, eg: -consul 127.0.0.1:8500")
}

func newApp(gs *grpc.Server, hs *http.Server, bs *conf.Bootstrap, client *api.Client, logger log.Logger) *kratos.App {
	reg := creg.New(client)
	return kratos.New(
		kratos.ID(id),
		kratos.Name(bs.Name),
		kratos.Version(bs.Version),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(
			gs,
			hs,
		),
		kratos.Registrar(reg),
	)
}

func main() {
	flag.Parse()
	//logger := log.With(log.NewStdLogger(os.Stdout),
	//	"ts", log.DefaultTimestamp,
	//	"caller", log.DefaultCaller,
	//	"service.id", id,
	//	"service.name", Name,
	//	"service.version", Version,
	//	"trace.id", tracing.TraceID(),
	//	"span.id", tracing.SpanID(),
	//)
	consulClient, err := api.NewClient(&api.Config{
		Address: flagConsul,
	})
	if err != nil {
		panic(err)
	}
	cs, err := consul.New(consulClient, consul.WithPath("cloudyst/common.yaml"))
	if err != nil {
		panic(err)
	}
	c := config.New(
		config.WithSource(
			cs,
			file.NewSource(flagconf),
		),
	)
	defer c.Close()

	if err := c.Load(); err != nil {
		panic(err)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		panic(err)
	}

	parser := uaparser.NewFromSaved()
	app, cleanup, err := wireApp(&bc, parser, consulClient)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	// start and wait for stop signal
	if err := app.Run(); err != nil {
		panic(err)
	}
}
