package conf

import (
	"common/constants"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
)

func LoadConfig() (*Bootstrap, error) {
	c := config.New(
		config.WithSource(
			file.NewSource(constants.ConfigPath),
		),
	)
	defer c.Close()
	if err := c.Load(); err != nil {
		return nil, err
	}
	var bc Bootstrap
	err := c.Scan(&bc)
	return &bc, err
}
