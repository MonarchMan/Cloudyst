package filemanager

import (
	pbadmin "api/api/user/admin/v1"
	"common/auth"
	"common/cache"
	"common/hashid"
	"common/request"
	"context"
	"file/internal/biz/credmanager"
	"file/internal/biz/filemanager/encrypt"
	"file/internal/biz/filemanager/eventhub"
	"file/internal/biz/filemanager/fs/mime"
	"file/internal/biz/filemanager/lock"
	"file/internal/biz/mediameta"
	"file/internal/biz/queue"
	"file/internal/biz/setting"
	"file/internal/biz/thumb"
	"file/internal/conf"
	"file/internal/data"
	"file/internal/data/rpc"

	"github.com/go-kratos/kratos/v2/log"
)

type (
	ManagerDep interface {
		UserClient() rpc.UserClient
		FileClient() data.FileClient
		ShareClient() data.ShareClient
		PolicyClient() data.StoragePolicyClient
		RequestClient() request.Client
		TaskClient() data.TaskClient
		MimeManager() mime.MimeManager
		CredManager() credmanager.CredManager
		QueuManager() *queue.QueueManager
		ExtractorManager() mediameta.ExtractorStateManager
		ThumbnailPipeline() thumb.Generator

		Logger() *log.Helper
		SettingProvider() setting.Provider
		KV() cache.Driver
		Config() *conf.Bootstrap
		GeneralAuth() auth.Auth
		Encoder() hashid.Encoder
		EncryptorFactory() encrypt.CryptorFactory
		EventHub() eventhub.EventHub
		MasterEncryptKeyVault() encrypt.MasterEncryptKeyVault
	}

	managerDependency struct {
		l                     *log.Helper
		settings              setting.Provider
		kv                    cache.Driver
		config                *conf.Bootstrap
		auth                  auth.Auth
		hasher                hashid.Encoder
		policyClient          data.StoragePolicyClient
		userClient            rpc.UserClient
		fileClient            data.FileClient
		shareClient           data.ShareClient
		requestClient         request.Client
		taskClient            data.TaskClient
		mimeManager           mime.MimeManager
		credManager           credmanager.CredManager
		extractorManager      mediameta.ExtractorStateManager
		thumbnailPipeline     thumb.Generator
		queueManager          *queue.QueueManager
		encryptorFactory      encrypt.CryptorFactory
		eventHub              eventhub.EventHub
		masterEncryptKeyVault encrypt.MasterEncryptKeyVault
	}
	ManagerDepCtx struct{}
)

func NewManagerDependency(logger log.Logger, settings setting.Provider, kv cache.Driver, au auth.Auth,
	config *conf.Bootstrap, hasher hashid.Encoder, policyClient data.StoragePolicyClient, userClient rpc.UserClient,
	fileClient data.FileClient, shareClient data.ShareClient, taskClient data.TaskClient, mimeManager mime.MimeManager,
	credManager credmanager.CredManager, extractorManager mediameta.ExtractorStateManager, queueManager *queue.QueueManager,
	thumbnailPipeline thumb.Generator, encryptorFactory encrypt.CryptorFactory, eventHub eventhub.EventHub,
) ManagerDep {
	masterEncryptKeyVault := encrypt.NewMasterEncryptKeyVault(context.Background(), settings)
	return &managerDependency{
		l:                     log.NewHelper(logger, log.WithMessageKey("biz-fileManager")),
		settings:              settings,
		kv:                    kv,
		config:                config,
		auth:                  au,
		hasher:                hasher,
		policyClient:          policyClient,
		userClient:            userClient,
		fileClient:            fileClient,
		shareClient:           shareClient,
		requestClient:         request.NewClient(config.Server.Sys.Mode),
		taskClient:            taskClient,
		mimeManager:           mimeManager,
		credManager:           credManager,
		extractorManager:      extractorManager,
		thumbnailPipeline:     thumbnailPipeline,
		queueManager:          queueManager,
		encryptorFactory:      encryptorFactory,
		eventHub:              eventHub,
		masterEncryptKeyVault: masterEncryptKeyVault,
	}
}

func (d *managerDependency) QueuManager() *queue.QueueManager {
	return d.queueManager
}

func (d *managerDependency) RequestClient() request.Client {
	return d.requestClient
}

func (d *managerDependency) TaskClient() data.TaskClient {
	return d.taskClient
}

func (d *managerDependency) MimeManager() mime.MimeManager {
	return d.mimeManager
}

func (d *managerDependency) CredManager() credmanager.CredManager {
	return d.credManager
}

func (d *managerDependency) ExtractorManager() mediameta.ExtractorStateManager {
	return d.extractorManager
}

func (d *managerDependency) ThumbnailPipeline() thumb.Generator {
	return d.thumbnailPipeline
}

func (d *managerDependency) UserClient() rpc.UserClient {
	return d.userClient
}

func (d *managerDependency) FileClient() data.FileClient {
	return d.fileClient
}

func (d *managerDependency) ShareClient() data.ShareClient {
	return d.shareClient
}

func (d *managerDependency) PolicyClient() data.StoragePolicyClient {
	return d.policyClient
}

func (d *managerDependency) Logger() *log.Helper {
	return d.l
}

func (d *managerDependency) SettingProvider() setting.Provider {
	return d.settings
}

func (d *managerDependency) KV() cache.Driver {
	return d.kv
}

func (d *managerDependency) Config() *conf.Bootstrap {
	return d.config
}

func (d *managerDependency) GeneralAuth() auth.Auth {
	return d.auth
}

func (d *managerDependency) Encoder() hashid.Encoder {
	return d.hasher
}

func (d *managerDependency) EncryptorFactory() encrypt.CryptorFactory {
	return d.encryptorFactory
}
func (d *managerDependency) EventHub() eventhub.EventHub {
	return d.eventHub
}
func (d *managerDependency) MasterEncryptKeyVault() encrypt.MasterEncryptKeyVault {
	return d.masterEncryptKeyVault
}

func ManagerDepFromContext(ctx context.Context) ManagerDep {
	return ctx.Value(ManagerDepCtx{}).(ManagerDep)
}

type (
	DbfsDep interface {
		LockSystem() lock.LockSystem
		StateKV() cache.Driver
		DirectLinkClient() data.DirectLinkClient
	}

	dbfsDependency struct {
		directLinkClient data.DirectLinkClient
		ls               lock.LockSystem
		stateKv          cache.Driver
		userAdmimClient  pbadmin.AdminClient
	}
	DbfsDepCtx struct{}
)

func NewDBFSDependency(ls lock.LockSystem, directLinkClient data.DirectLinkClient, l log.Logger) DbfsDep {
	return &dbfsDependency{
		ls:               ls,
		stateKv:          cache.NewMemoStore("", l),
		directLinkClient: directLinkClient,
	}
}
func (d *dbfsDependency) LockSystem() lock.LockSystem {
	return d.ls
}
func (d *dbfsDependency) StateKV() cache.Driver {
	return d.stateKv
}
func (d *dbfsDependency) DirectLinkClient() data.DirectLinkClient {
	return d.directLinkClient
}

func DBFSDepFromContext(ctx context.Context) DbfsDep {
	return ctx.Value(DbfsDepCtx{}).(DbfsDep)
}
