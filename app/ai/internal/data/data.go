package data

import (
	"ai/internal/conf"
	"ai/internal/data/rpc"
	"ai/internal/data/vector"
	"common/cache"
	"common/db"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(
	NewAIModelClient,
	NewKnowledgeClient,
	NewKnowledgeDocumentClient,
	NewKnowledgeSegmentClient,
	NewChatRoleClient,
	NewChatConversationClient,
	NewChatMessageClient,
	NewAIImageClient,
	vector.NewMilvusClient,
	rpc.NewFileClient,
	rpc.RawUserClient,
	rpc.NewUserClient,
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
