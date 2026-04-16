package biz

import (
	"common/cache"
	"user/internal/biz/email"
	"user/internal/biz/setting"
	"user/internal/data"

	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	email.NewEmailManager,
	SettingProviderWrapper,
	NewTokenAuth,
)

func SettingProviderWrapper(kv cache.Driver, settingClient data.SettingClient) setting.Provider {
	// setting value will be retrieved in order: Env overwrite -> KV Store -> DB Setting Store
	return setting.NewProvider(setting.NewKvSettingStore(kv, setting.NewDbSettingStore(settingClient, nil)))
}
