package dependency

import (
	"common/auth"
	"common/cache"
	"common/hashid"
	"common/logging"
	"file/ent"
	"file/internal/biz/setting"
	"file/internal/conf"
	"file/internal/data"

	"github.com/hashicorp/consul/api"
)

// Option 发送请求的额外设置
type Option interface {
	apply(*dependency)
}

type optionFunc func(*dependency)

func (f optionFunc) apply(o *dependency) {
	f(o)
}

// WithConfigPath Set the path of the config files.
func WithConfigPath(p string) Option {
	return optionFunc(func(o *dependency) {
		o.configPath = p
	})
}

// WithLogger Set the default logging.
func WithLogger(l *log.Helper) Option {
	return optionFunc(func(o *dependency) {
		o.logger = l
	})
}

// WithConfigProvider Set the default config provider.
func WithConfigProvider(c *conf.Bootstrap) Option {
	return optionFunc(func(o *dependency) {
		o.configProvider = c
	})
}

func WithLicenseKey(c string) Option {
	return optionFunc(func(o *dependency) {
		o.licenseKey = c
	})
}

// WithRawEntClient Set the default raw ent client.
func WithRawEntClient(c *ent.Client) Option {
	return optionFunc(func(o *dependency) {
		o.rawEntClient = c
	})
}

// WithDbClient Set the default ent client.
func WithDbClient(c *ent.Client) Option {
	return optionFunc(func(o *dependency) {
		o.dbClient = c
	})
}

// WithRequiredDbVersion Set the required data version.
func WithRequiredDbVersion(c string) Option {
	return optionFunc(func(o *dependency) {
		o.requiredDbVersion = c
	})
}

// WithKV Set the default KV store driverold
func WithKV(c cache.Driver) Option {
	return optionFunc(func(o *dependency) {
		o.kv = c
	})
}

// WithSettingClient Set the default setting client
func WithSettingClient(s data.SettingClient) Option {
	return optionFunc(func(o *dependency) {
		o.settingClient = s
	})
}

// WithSettingProvider Set the default setting provider
func WithSettingProvider(s setting.Provider) Option {
	return optionFunc(func(o *dependency) {
		o.settingProvider = s
	})
}

// WithGeneralAuth Set the default general auth
func WithGeneralAuth(s auth.Auth) Option {
	return optionFunc(func(o *dependency) {
		o.generalAuth = s
	})
}

// WithHashIDEncoder Set the default hash id encoder
func WithHashIDEncoder(s hashid.Encoder) Option {
	return optionFunc(func(o *dependency) {
		o.hashidEncoder = s
	})
}

// WithTokenAuth Set the default token auth
//func WithTokenAuth(s auth.TokenAuth) Option {
//	return optionFunc(func(o *dependency) {
//		o.tokenAuth = s
//	})
//}

// WithFileClient Set the default files client
func WithFileClient(s data.FileClient) Option {
	return optionFunc(func(o *dependency) {
		o.fileClient = s
	})
}

// WithShareClient Set the default share client
func WithShareClient(s data.ShareClient) Option {
	return optionFunc(func(o *dependency) {
		o.shareClient = s
	})
}

func WithServiceCenter(s *api.Client) Option {
	return optionFunc(func(o *dependency) {
		o.serviceCenter = s
	})
}
