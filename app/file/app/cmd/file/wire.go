//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"file/app"
	"file/internal/biz"
	"file/internal/conf"
	"file/internal/data"
	"file/internal/pkg"
	"file/internal/server"
	"file/internal/service"

	"github.com/go-kratos/kratos/v2"
	"github.com/google/wire"
	"github.com/hashicorp/consul/api"
)

// wireApp init kratos application.
func wireApp(*conf.Bootstrap, *api.Client) (*kratos.App, func(), error) {
	panic(wire.Build(server.ProviderSet, service.MasterProviderSet, service.SalveProviderSet, biz.ProviderSet,
		pkg.ProviderSet, data.ProviderSet, app.NewServer, newApp))
}
