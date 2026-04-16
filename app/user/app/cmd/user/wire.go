//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"user/internal/biz"
	"user/internal/conf"
	"user/internal/data"
	"user/internal/pkg"
	"user/internal/server"
	"user/internal/service"

	"github.com/go-kratos/kratos/v2"
	"github.com/google/wire"
	"github.com/hashicorp/consul/api"
	"github.com/ua-parser/uap-go/uaparser"
)

// wireApp init kratos application.
func wireApp(*conf.Bootstrap, *uaparser.Parser, *api.Client) (*kratos.App, func(), error) {
	panic(wire.Build(server.ProviderSet, data.ProviderSet, biz.ProviderSet, service.ProviderSet, pkg.ProviderSet, newApp))
}
