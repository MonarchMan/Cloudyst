package biz

import (
	"common/cache"
	"common/constants"
	"context"
	"file/internal/biz/cluster"
	"file/internal/biz/credmanager"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/encrypt"
	"file/internal/biz/filemanager/eventhub"
	"file/internal/biz/filemanager/fs/mime"
	"file/internal/biz/mediameta"
	"file/internal/biz/queue"
	"file/internal/biz/setting"
	"file/internal/biz/thumb"
	"file/internal/conf"
	"file/internal/data"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

// ProviderSet is biz providers.
var ProviderSet = wire.NewSet(
	filemanager.NewManagerDependency, // auth.Auth, hashid.Encoder
	filemanager.NewDBFSDependency,    // lock.LockSystem
	queue.NewTaskRegistry,
	queue.NewQueueManager,
	thumb.NewPipeline,
	mime.NewMimeManager,
	mediameta.NewExtractorManager,
	eventhub.NewEventHub,
	encrypt.NewCryptorFactory,
	MasterEncryptKeyVaultWrapper,
	NodePoolWrapper,
	CredManagerWrapper,
	SettingProviderWrapper,
)

func NodePoolWrapper(config *conf.Bootstrap, l log.Logger, settings setting.Provider, nc data.NodeClient) (cluster.NodePool, error) {
	if config.Server.Sys.Mode == constants.MasterMode {
		np, err := cluster.NewNodePool(l, config, settings, nc)
		if err != nil {
			return nil, err
		}
		return np, nil
	} else {
		np := cluster.NewSlaveDummyNodePool(config, settings, l)
		return np, nil
	}
}

func CredManagerWrapper(config *conf.Bootstrap, kv cache.Driver, pc data.StoragePolicyClient, l log.Logger) credmanager.CredManager {
	if config.Server.Sys.Mode == constants.MasterMode {
		return credmanager.New(kv, pc, config, l)
	} else {
		return credmanager.NewSlaveManager(kv, config, l)
	}
}

func SettingProviderWrapper(config *conf.Bootstrap, kv cache.Driver, client data.SettingClient) setting.Provider {
	if config.Server.Sys.Mode == constants.MasterMode {
		// For master mode, setting value will be retrieved in order:
		// KV Store -> DB Setting Store
		return setting.NewProvider(
			setting.NewKvSettingStore(kv,
				setting.NewDbSettingStore(client, nil)),
		)
	} else {
		// For slave mode, setting value will be retrieved in order:
		// Setting defaults in DB schema
		return setting.NewProvider(
			setting.NewDbDefaultStore(nil),
		)
	}
}

func MasterEncryptKeyVaultWrapper(settings setting.Provider) encrypt.MasterEncryptKeyVault {
	return encrypt.NewMasterEncryptKeyVault(context.Background(), settings)
}
