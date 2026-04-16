//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"ai/internal/biz"
	"ai/internal/conf"
	"ai/internal/data"
	"ai/internal/pkg"
	"ai/internal/server"
	"ai/internal/service"

	"github.com/go-kratos/kratos/v2"
	"github.com/google/wire"
	"github.com/hashicorp/consul/api"
)

// wireApp init kratos application.
func wireApp(*conf.Bootstrap, *api.Client) (*kratos.App, func(), error) {
	panic(wire.Build(server.ProviderSet, data.ProviderSet, biz.ProviderSet, service.ProviderSet, pkg.ProviderSet, newApp))
}
