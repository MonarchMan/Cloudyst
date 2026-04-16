package main

import (
	"common/constants"
	"context"
	"flag"
	"fmt"
	"os"

	"ai/internal/conf"

	creg "github.com/go-kratos/kratos/contrib/registry/consul/v2"
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-kratos/kratos/v2/transport/http"
	"github.com/hashicorp/consul/api"

	_ "go.uber.org/automaxprocs"
)

// go build -ldflags "-X main.Version=x.y.z"
var (
	// flagconf is the config flag.
	flagconf string
	// flagConsul is the consul dsn flag.
	flagConsul string

	id, _ = os.Hostname()
)

func init() {
	flag.StringVar(&flagconf, "conf", "../../../configs", "config path, eg: -conf common.yaml")
	flag.StringVar(&flagConsul, "consul", "127.0.0.1:8500", "consul address, eg: -consul 127.0.0.1:8500")
}

func newApp(gs *grpc.Server, hs *http.Server, bs *conf.Bootstrap, client *api.Client, l log.Logger) (*kratos.App, error) {
	// prepare kratos app
	h := log.NewHelper(l, log.WithMessageKey("app"))
	reg := creg.New(client)
	var options []kratos.Option
	if bs.Server.Sys.GracePeriod != nil {
		options = append(options, kratos.StopTimeout(bs.Server.Sys.GracePeriod.AsDuration()))
	}
	return kratos.New(
		kratos.ID(constants.AiServicePrefix),
		kratos.Name(bs.Name),
		kratos.Version(bs.Version),
		kratos.Metadata(map[string]string{}),
		kratos.Server(
			gs,
			hs,
		),
		kratos.Logger(l),
		kratos.Registrar(reg),
		kratos.BeforeStop(func(ctx context.Context) error {
			err := hs.Shutdown(ctx)
			if err != nil {
				h.Error("Failed to gracefully shutdown http server", "error", err)
			}
			err = gs.Stop(ctx)
			if err != nil {
				h.Error("Failed to gracefully shutdown grpc server", "error", err)
			}
			return err
		}),
	), nil
}

func printBanner() {
	fmt.Print(`
   	___ _                _                    
  / __\ | ___  _   _  __| |__ __ _____ _____           ____     _____
 / /  | |/ _ \| | | |/ _  | |_| |_  __|_____|         / __ \   |_   _|
/ /___| | (_) | |_| | (_| |     |_\ \_  | |	 ------- / /__\ \   _| |_
\____/|_|\___/ \__,_|\__,_|\_  /|_____| |_|         /_/    \_\ |_____|
                            / /
                           /_/

   V` + constants.BackendVersion + `
================================================

`)
}

func main() {
	//flag.Parse()
	//consulClient, err := api.NewClient(&api.Config{
	//	Address: flagConsul,
	//})
	//if err != nil {
	//	panic(err)
	//}
	//cs, err := consul.New(consulClient, consul.WithPath("cloudyst/common.yaml"))
	//if err != nil {
	//	panic(err)
	//}
	//c := config.New(
	//	config.WithSource(
	//		cs,
	//		file.NewSource(flagconf),
	//	),
	//)
	//defer c.Close()
	//
	//if err := c.Load(); err != nil {
	//	panic(err)
	//}
	//
	//var bs conf.Bootstrap
	//if err := c.Scan(&bs); err != nil {
	//	panic(err)
	//}
	//
	//app, cleanup, err := wireApp(&bs, consulClient)
	//if err != nil {
	//	panic(err)
	//}
	//defer cleanup()
	//
	//// start and wait for stop signal
	//if err := app.Run(); err != nil {
	//	panic(err)
	//}
}
