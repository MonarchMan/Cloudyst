package data

import (
	"common/cache"
	"common/db"
	"user/internal/conf"
	"user/internal/data/rpc"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	rpc.NewFileSysClient,
	rpc.NewFileClient,
	rpc.NewShareClient,
	rpc.NewFileAdminClient,
	NewUserClient,
	NewSettingClient,
	NewDBClient,
	NewDavAccountClient,
	KVStoreWrapper,
	NewGroupClient,
	DBTypeWrapper,
)

func KVStoreWrapper(config *conf.Bootstrap, l log.Logger) cache.Driver {
	if config.Data.Redis.Addr == "" {
		return cache.NewMemoStore(cache.DefaultCacheFile, l)
	}

	redisConf := config.Data.Redis
	return cache.NewRedisStore(l, 10, int(redisConf.Db), redisConf.Network, redisConf.Addr,
		redisConf.User, redisConf.Password, redisConf.UseTls, redisConf.TlsSkipVerify)
}

func DBTypeWrapper(config *conf.Bootstrap) db.DBType {
	return db.DBType(config.Data.Database.DbType)
}
