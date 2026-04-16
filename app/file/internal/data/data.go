package data

import (
	"common/cache"
	"common/db"
	"file/internal/conf"
	"file/internal/data/rpc"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	rpc.NewUserClient,
	rpc.NewUserAdminClient,
	NewDirectLinkClient,
	NewFileClient,
	NewNodeClient,
	NewSettingClient,
	NewShareClient,
	NewStoragePolicyClient,
	NewTaskClient,
	NewFsEventClient,
	NewDBClient,
	KVWrapper,
	DbTypeWrapper,
)

func KVWrapper(config *conf.Bootstrap, l log.Logger) cache.Driver {
	if config.Data.Redis.Addr == "" {
		return cache.NewMemoStore(cache.DefaultCacheFile, l)
	}

	redisConf := config.Data.Redis
	return cache.NewRedisStore(l, 10, int(redisConf.Db), redisConf.Network, redisConf.Addr,
		redisConf.User, redisConf.Password, redisConf.UseTls, redisConf.TlsSkipVerify)
}

func DbTypeWrapper(config *conf.Bootstrap) db.DBType {
	return db.DBType(config.Data.Database.DbType)
}
