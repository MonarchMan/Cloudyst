package test

import (
	"api/external/conf"
	"testing"

	"github.com/go-kratos/kratos/contrib/config/consul/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/hashicorp/consul/api"
)

func TestConsul(t *testing.T) {
	consulClient, err := api.NewClient(&api.Config{
		Address: "simple-net.dynv6.net:8500",
	})
	if err != nil {
		t.Fatal(err)
	}
	cs, err := consul.New(consulClient, consul.WithPath("cloudyst/common.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	c := config.New(config.WithSource(cs))
	defer c.Close()

	if err := c.Load(); err != nil {
		t.Fatal(err)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		t.Fatal(err)
	}
	t.Log(bc.String())
}
