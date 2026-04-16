package dependency

import (
	pbuser "api/api/user/users/v1"
	"common/auth"
	"common/cache"
	"common/constants"
	"common/hashid"
	"common/logging"
	"common/request"
	"common/util"
	"context"
	"errors"
	"file/ent"
	"file/internal/biz/cluster"
	"file/internal/biz/credmanager"
	"file/internal/biz/filemanager/fs/mime"
	"file/internal/biz/filemanager/lock"
	"file/internal/biz/mediameta"
	"file/internal/biz/queue"
	"file/internal/biz/setting"
	"file/internal/biz/thumb"
	"file/internal/conf"
	"file/internal/data"
	"file/internal/data/rpc"
	"file/internal/data/types"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/hashicorp/consul/api"
	"github.com/robfig/cron/v3"
	"github.com/ua-parser/uap-go/uaparser"
)

type (
	// DepCtx defines keys for dependency manager
	DepCtx struct{}
	// ReloadCtx force reload new dependency
	ReloadCtx struct{}
)

// Dep manages all dependencies of the server application. The default implementation is not
// concurrent safe, so all inner deps should be initialized before any goroutine starts.
type Dep interface {
	// ConfigProvider Get a singleton conf.ConfigProvider instance.
	ConfigProvider() *conf.Bootstrap
	// Logger Get a singleton *log.Helper instance.
	Logger() *log.Helper
	// DBClient Get a singleton ent.Client instance for database access.
	DBClient() *ent.Client
	// KV Get a singleton cache.Driver instance for KV store.
	KV() cache.Driver
	// NavigatorStateKV Get a singleton cache.Driver instance for navigator state store. It forces use in-memory
	// map instead of Redis to get better performance for complex nested linked list.
	NavigatorStateKV() cache.Driver

	// FileClient Creates a new data.FileClient instance for access DB files store.
	FileClient() data.FileClient
	// NodeClient Creates a new data.NodeClient instance for access DB node store.
	NodeClient() data.NodeClient
	// StoragePolicyClient Creates a new data.StoragePolicyClient instance for access DB storage policy store.
	StoragePolicyClient() data.StoragePolicyClient
	// LockSystem Get a singleton lock.LockSystem instance for files lock management.
	LockSystem() lock.LockSystem

	// DavAccountClient Creates a new data.DavAccountClient instance for access DB dav account store.
	DavAccountClient() data.DavAccountClient
	// DirectLinkClient Creates a new data.DirectLinkClient instance for access DB direct link store.
	DirectLinkClient() data.DirectLinkClient
	// ShareClient Creates a new data.ShareClient instance for access DB share store.
	ShareClient() data.ShareClient
	// SettingClient Get a singleton data.SettingClient instance for access DB setting store.
	SettingClient() data.SettingClient
	// UserClient Creates a new pbuser.UserClient instance for access users service.
	UserClient() pbuser.UserClient

	// ServiceCenter Get a singleton api.Client instance for service center.
	ServiceCenter() *api.Client
	// RequestClient Creates a new request.Client instance for HTTP requests.
	RequestClient(opts ...request.Option) request.Client
	// CredManager Get a singleton credmanager.CredManager instance for credential management.
	CredManager() credmanager.CredManager
	// NodePool Get a singleton cluster.NodePool instance for node pool management.
	NodePool(ctx context.Context) (cluster.NodePool, error)
	// MimeDetector Get a singleton fs.MimeDetector instance for MIME type detection.
	MimeDetector(ctx context.Context) mime.MimeDetector

	// TaskClient Creates a new data.TaskClient instance for access DB task store.
	TaskClient() data.TaskClient
	// TaskRegistry Get a singleton queue.TaskRegistry instance for task registration.
	TaskRegistry() queue.TaskRegistry
	// MediaMetaExtractor Get a singleton mediameta.Extractor instance for media metadata extraction.
	MediaMetaExtractor(ctx context.Context) mediameta.Extractor
	// SlaveQueue Get a singleton queue.Queue instance for slave tasks.
	SlaveQueue(ctx context.Context) queue.Queue
	// ThumbQueue Get a singleton queue.Queue instance for thumbnail generation.
	ThumbQueue(ctx context.Context) queue.Queue
	// EntityRecycleQueue Get a singleton queue.Queue instance for entity recycle.
	EntityRecycleQueue(ctx context.Context) queue.Queue
	// IoIntenseQueue Get a singleton queue.Queue instance for IO intense tasks.
	IoIntenseQueue(ctx context.Context) queue.Queue
	// RemoteDownloadQueue Get a singleton queue.Queue instance for remote download tasks.
	RemoteDownloadQueue(ctx context.Context) queue.Queue

	// MediaMetaQueue Get a singleton queue.Queue instance for media metadata processing.
	MediaMetaQueue(ctx context.Context) queue.Queue
	// ThumbPipeline Get a singleton thumb.Generator instance for chained thumbnail generation.
	ThumbPipeline() thumb.Generator

	// HashIDEncoder Get a singleton hashid.Encoder instance for encoding/decoding hashids.
	HashIDEncoder() hashid.Encoder
	SettingProvider() setting.Provider
	GeneralAuth() auth.Auth

	// ForkWithLogger create a shallow copy of dependency with a new correlated logger, used as per-request dep.
	ForkWithLogger(ctx context.Context, l *log.Helper) context.Context
	// Shutdown the dependencies gracefully.
	Shutdown(ctx context.Context) error
}

type dependency struct {
	configProvider      *conf.Bootstrap
	logger              *log.Helper
	dbClient            *ent.Client
	rawEntClient        *ent.Client
	kv                  cache.Driver
	navigatorStateKv    cache.Driver
	fileClient          data.FileClient
	shareClient         data.ShareClient
	settingClient       data.SettingClient
	settingProvider     setting.Provider
	storagePolicyClient data.StoragePolicyClient
	taskClient          data.TaskClient
	nodeClient          data.NodeClient
	userClient          pbuser.UserClient
	davAccountClient    data2.DavAccountClient
	directLinkClient    data.DirectLinkClient
	hashidEncoder       hashid.Encoder
	lockSystem          lock.LockSystem

	// ServiceCenter Get a singleton api.Client instance for service center.
	serviceCenter       *api.Client
	requestClient       request.Client
	ioIntenseQueue      queue.Queue
	thumbQueue          queue.Queue
	mediaMetaQueue      queue.Queue
	entityRecycleQueue  queue.Queue
	slaveQueue          queue.Queue
	remoteDownloadQueue queue.Queue
	ioIntenseQueueTask  queue.Task
	mediaMeta           mediameta.Extractor
	thumbPipeline       thumb.Generator
	mimeDetector        mime.MimeDetector
	credManager         credmanager.CredManager
	nodePool            cluster.NodePool
	taskRegistry        queue.TaskRegistry
	webauthn            *webauthn.WebAuthn
	parser              *uaparser.Parser
	cron                *cron.Cron
	generalAuth         auth.Auth

	configPath        string
	requiredDbVersion string
	licenseKey        string

	// Protects inner deps that can be reloaded at runtime.
	mu sync.Mutex
}

// NewDependency creates a new Dep instance for construct dependencies.
func NewDependency(opts ...Option) Dep {
	d := &dependency{}
	for _, o := range opts {
		o.apply(d)
	}

	return d
}

// FromContext retrieves a Dep instance from context.
func FromContext(ctx context.Context) Dep {
	return ctx.Value(DepCtx{}).(Dep)
}

func (d *dependency) ConfigProvider() *conf.Bootstrap {
	if d.configProvider != nil {
		return d.configProvider
	}

	var err error
	d.configProvider, err = conf.LoadConfig()
	if err != nil {
		d.panicError(err)
	}
	return d.configProvider
}

func (d *dependency) Logger() *log.Helper {
	if d.logger != nil {
		return d.logger
	}

	config := d.ConfigProvider()
	logLevel := logging.LogLevel(config.GetServer().GetSys().GetLogLevel())
	if config.GetServer().GetSys().GetDebug() {
		logLevel = logging.LevelDebug
	}

	d.logger = logging.NewConsoleLogger(logLevel)
	d.logger.Info("Logger initialized with LogLevel=%q.", logLevel)
	return d.logger
}

func (d *dependency) DBClient() *ent.Client {
	if d.dbClient != nil {
		return d.dbClient
	}

	if d.rawEntClient == nil {
		client, err := data.NewRawEntClient(d.Logger(), d.ConfigProvider())
		if err != nil {
			d.panicError(err)
		}
		d.rawEntClient = client
	}

	client, err := data.InitializeDBClient(d.Logger(), d.rawEntClient, d.KV(), d.requiredDbVersion)
	if err != nil {
		d.panicError(err)
	}

	d.dbClient = client
	return client
}

func (d *dependency) KV() cache.Driver {
	if d.kv != nil {
		return d.kv
	}

	config := d.ConfigProvider().GetData().GetRedis()
	if config.GetAddr() != "" {
		d.kv = cache.NewRedisStore(d.Logger(), 10, int(config.GetDb()), config.Network, config.GetAddr(),
			config.GetUser(), config.GetPassword(), config.GetUseTls(), config.GetTlsSkipVerify())
	} else {
		d.kv = cache.NewMemoStore(util.DataPath(cache.DefaultCacheFile), d.Logger())
	}

	return d.kv
}

func (d *dependency) NavigatorStateKV() cache.Driver {
	if d.navigatorStateKv != nil {
		return d.navigatorStateKv
	}
	d.navigatorStateKv = cache.NewMemoStore("", d.Logger())
	return d.navigatorStateKv
}

func (d *dependency) FileClient() data.FileClient {
	if d.fileClient != nil {
		return d.fileClient
	}
	dbType := types.DBType(d.ConfigProvider().GetData().GetDatabase().GetDbType())

	return data.NewFileClient(d.DBClient(), dbType, d.HashIDEncoder())
}

func (d *dependency) NodeClient() data.NodeClient {
	if d.nodeClient != nil {
		return d.nodeClient
	}

	return data.NewNodeClient(d.DBClient())
}

func (d *dependency) StoragePolicyClient() data.StoragePolicyClient {
	if d.storagePolicyClient != nil {
		return d.storagePolicyClient
	}

	return data.NewStoragePolicyClient(d.DBClient(), d.KV())
}

func (d *dependency) LockSystem() lock.LockSystem {
	if d.lockSystem != nil {
		return d.lockSystem
	}

	d.lockSystem = lock.NewMemLS(d.HashIDEncoder(), d.Logger())
	return d.lockSystem
}

func (d *dependency) DavAccountClient() data2.DavAccountClient {
	if d.davAccountClient != nil {
		return d.davAccountClient
	}

	dbType := types.DBType(d.ConfigProvider().GetData().GetDatabase().GetDbType())
	return data2.NewDavAccountClient(d.DBClient(), dbType, d.HashIDEncoder())
}

func (d *dependency) DirectLinkClient() data.DirectLinkClient {
	if d.directLinkClient != nil {
		return d.directLinkClient
	}

	dbType := types.DBType(d.ConfigProvider().GetData().GetDatabase().GetDbType())
	return data.NewDirectLinkClient(d.DBClient(), dbType, d.HashIDEncoder())
}

func (d *dependency) ShareClient() data.ShareClient {
	if d.shareClient != nil {
		return d.shareClient
	}

	dbType := types.DBType(d.ConfigProvider().GetData().GetDatabase().GetDbType())
	return data.NewShareClient(d.DBClient(), dbType, d.HashIDEncoder())
}

func (d *dependency) SettingClient() data.SettingClient {
	if d.settingClient != nil {
		return d.settingClient
	}

	d.settingClient = data.NewSettingClient(d.DBClient(), d.KV())
	return d.settingClient
}

func (d *dependency) UserClient() pbuser.UserClient {
	if d.userClient != nil {
		return d.userClient
	}

	d.userClient = rpc.NewUserClient(d.ServiceCenter())
	return d.userClient
}

func (d *dependency) ServiceCenter() *api.Client {
	return d.serviceCenter
}

// TODO 将自定义请求客户端改为grpc调用
func (d *dependency) RequestClient(opts ...request.Option) request.Client {
	if d.requestClient != nil {
		return d.requestClient
	}

	return request.NewClient(d.ConfigProvider(), opts...)
}

func (d *dependency) CredManager() credmanager.CredManager {
	if d.credManager != nil {
		return d.credManager
	}

	if d.ConfigProvider().GetServer().GetSys().GetMode() == constants.MasterMode {
		d.credManager = credmanager.New(d.KV())
	} else {
		d.credManager = credmanager.NewSlaveManager(d.KV(), d.ConfigProvider())
	}

	return d.credManager
}

func (d *dependency) NodePool(ctx context.Context) (cluster.NodePool, error) {
	reload, _ := ctx.Value(ReloadCtx{}).(bool)
	if d.nodePool != nil && !reload {
		return d.nodePool, nil
	}

	if d.ConfigProvider().GetServer().GetSys().GetMode() == constants.MasterMode {
		np, err := cluster.NewNodePool(ctx, d.Logger(), d.ConfigProvider(), d.SettingProvider(), d.NodeClient())
		if err != nil {
			return nil, err
		}

		d.nodePool = np
	} else {
		d.nodePool = cluster.NewSlaveDummyNodePool(ctx, d.ConfigProvider(), d.SettingProvider())
	}

	return d.nodePool, nil
}

func (d *dependency) MimeDetector(ctx context.Context) mime.MimeDetector {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, reload := ctx.Value(ReloadCtx{}).(bool)
	if d.mimeDetector != nil && !reload {
		return d.mimeDetector
	}

	d.mimeDetector = mime.NewMimeDetector(ctx, d.SettingProvider(), d.Logger())
	return d.mimeDetector
}

func (d *dependency) TaskClient() data.TaskClient {
	if d.taskClient != nil {
		return d.taskClient
	}

	dbType := types.DBType(d.ConfigProvider().GetData().GetDatabase().GetDbType())
	return data.NewTaskClient(d.DBClient(), dbType, d.HashIDEncoder())
}

func (d *dependency) TaskRegistry() queue.TaskRegistry {
	if d.taskRegistry != nil {
		return d.taskRegistry
	}

	d.taskRegistry = queue.NewTaskRegistry()
	return d.taskRegistry
}

func (d *dependency) MediaMetaExtractor(ctx context.Context) mediameta.Extractor {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, reload := ctx.Value(ReloadCtx{}).(bool)
	if d.mediaMeta != nil && !reload {
		return d.mediaMeta
	}

	d.mediaMeta = mediameta.NewExtractor(ctx, d.SettingProvider(), d.Logger())
	return d.mediaMeta
}

func (d *dependency) SlaveQueue(ctx context.Context) queue.Queue {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, reload := ctx.Value(ReloadCtx{}).(bool)
	if d.slaveQueue != nil && !reload {
		return d.slaveQueue
	}

	if d.slaveQueue != nil {
		d.slaveQueue.Shutdown()
	}

	settings := d.SettingProvider()
	queueSetting := settings.Queue(context.Background(), setting.QueueTypeSlave)

	d.slaveQueue = queue.New(d.Logger(), nil, nil,
		queue.WithBackoffFactor(queueSetting.BackoffFactor),
		queue.WithMaxRetry(queueSetting.MaxRetry),
		queue.WithBackoffMaxDuration(queueSetting.BackoffMaxDuration),
		queue.WithRetryDelay(queueSetting.RetryDelay),
		queue.WithWorkerCount(queueSetting.WorkerNum),
		queue.WithName("SlaveQueue"),
		queue.WithMaxTaskExecution(queueSetting.MaxExecution),
	)
	return d.slaveQueue
}

func (d *dependency) ThumbQueue(ctx context.Context) queue.Queue {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, reload := ctx.Value(ReloadCtx{}).(bool)
	if d.thumbQueue != nil && !reload {
		return d.thumbQueue
	}

	if d.thumbQueue != nil {
		d.thumbQueue.Shutdown()
	}

	settings := d.SettingProvider()
	queueSetting := settings.Queue(context.Background(), setting.QueueTypeThumb)
	var (
		t data.TaskClient
	)
	if d.ConfigProvider().GetServer().GetSys().GetMode() == constants.MasterMode {
		t = d.TaskClient()
	}

	d.thumbQueue = queue.New(d.Logger(), t, nil,
		queue.WithBackoffFactor(queueSetting.BackoffFactor),
		queue.WithMaxRetry(queueSetting.MaxRetry),
		queue.WithBackoffMaxDuration(queueSetting.BackoffMaxDuration),
		queue.WithRetryDelay(queueSetting.RetryDelay),
		queue.WithWorkerCount(queueSetting.WorkerNum),
		queue.WithName("ThumbQueue"),
		queue.WithMaxTaskExecution(queueSetting.MaxExecution),
	)
	return d.thumbQueue
}

func (d *dependency) EntityRecycleQueue(ctx context.Context) queue.Queue {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, reload := ctx.Value(ReloadCtx{}).(bool)
	if d.entityRecycleQueue != nil && !reload {
		return d.entityRecycleQueue
	}

	if d.entityRecycleQueue != nil {
		d.entityRecycleQueue.Shutdown()
	}

	settings := d.SettingProvider()
	queueSetting := settings.Queue(context.Background(), setting.QueueTypeEntityRecycle)

	d.entityRecycleQueue = queue.New(d.Logger(), d.TaskClient(), nil,
		queue.WithBackoffFactor(queueSetting.BackoffFactor),
		queue.WithMaxRetry(queueSetting.MaxRetry),
		queue.WithBackoffMaxDuration(queueSetting.BackoffMaxDuration),
		queue.WithRetryDelay(queueSetting.RetryDelay),
		queue.WithWorkerCount(queueSetting.WorkerNum),
		queue.WithName("EntityRecycleQueue"),
		queue.WithMaxTaskExecution(queueSetting.MaxExecution),
		queue.WithResumeTaskType(queue.EntityRecycleRoutineTaskType, queue.ExplicitEntityRecycleTaskType, queue.UploadSentinelCheckTaskType),
		queue.WithTaskPullInterval(10*time.Second),
	)
	return d.entityRecycleQueue
}

func (d *dependency) IoIntenseQueue(ctx context.Context) queue.Queue {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, reload := ctx.Value(ReloadCtx{}).(bool)
	if d.ioIntenseQueue != nil && !reload {
		return d.ioIntenseQueue
	}

	if d.ioIntenseQueue != nil {
		d.ioIntenseQueue.Shutdown()
	}

	settings := d.SettingProvider()
	queueSetting := settings.Queue(context.Background(), setting.QueueTypeIOIntense)

	d.ioIntenseQueue = queue.New(d.Logger(), d.TaskClient(), d.TaskRegistry(),
		queue.WithBackoffFactor(queueSetting.BackoffFactor),
		queue.WithMaxRetry(queueSetting.MaxRetry),
		queue.WithBackoffMaxDuration(queueSetting.BackoffMaxDuration),
		queue.WithRetryDelay(queueSetting.RetryDelay),
		queue.WithWorkerCount(queueSetting.WorkerNum),
		queue.WithName("IoIntenseQueue"),
		queue.WithMaxTaskExecution(queueSetting.MaxExecution),
		queue.WithResumeTaskType(queue.CreateArchiveTaskType, queue.ExtractArchiveTaskType, queue.RelocateTaskType, queue.ImportTaskType),
		queue.WithTaskPullInterval(10*time.Second),
	)
	return d.ioIntenseQueue
}

func (d *dependency) RemoteDownloadQueue(ctx context.Context) queue.Queue {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, reload := ctx.Value(ReloadCtx{}).(bool)
	if d.remoteDownloadQueue != nil && !reload {
		return d.remoteDownloadQueue
	}

	if d.remoteDownloadQueue != nil {
		d.remoteDownloadQueue.Shutdown()
	}

	settings := d.SettingProvider()
	queueSetting := settings.Queue(context.Background(), setting.QueueTypeRemoteDownload)

	d.remoteDownloadQueue = queue.New(d.Logger(), d.TaskClient(), d.TaskRegistry(),
		queue.WithBackoffFactor(queueSetting.BackoffFactor),
		queue.WithMaxRetry(queueSetting.MaxRetry),
		queue.WithBackoffMaxDuration(queueSetting.BackoffMaxDuration),
		queue.WithRetryDelay(queueSetting.RetryDelay),
		queue.WithWorkerCount(queueSetting.WorkerNum),
		queue.WithName("RemoteDownloadQueue"),
		queue.WithMaxTaskExecution(queueSetting.MaxExecution),
		queue.WithResumeTaskType(queue.RemoteDownloadTaskType),
		queue.WithTaskPullInterval(10*time.Second),
	)
	return d.remoteDownloadQueue
}

func (d *dependency) MediaMetaQueue(ctx context.Context) queue.Queue {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, reload := ctx.Value(ReloadCtx{}).(bool)
	if d.mediaMetaQueue != nil && !reload {
		return d.mediaMetaQueue
	}

	if d.mediaMetaQueue != nil {
		d.mediaMetaQueue.Shutdown()
	}

	settings := d.SettingProvider()
	queueSetting := settings.Queue(context.Background(), setting.QueueTypeMediaMeta)

	d.mediaMetaQueue = queue.New(d.Logger(), d.TaskClient(), nil,
		queue.WithBackoffFactor(queueSetting.BackoffFactor),
		queue.WithMaxRetry(queueSetting.MaxRetry),
		queue.WithBackoffMaxDuration(queueSetting.BackoffMaxDuration),
		queue.WithRetryDelay(queueSetting.RetryDelay),
		queue.WithWorkerCount(queueSetting.WorkerNum),
		queue.WithName("MediaMetadataQueue"),
		queue.WithMaxTaskExecution(queueSetting.MaxExecution),
		queue.WithResumeTaskType(queue.MediaMetaTaskType),
	)
	return d.mediaMetaQueue
}

func (d *dependency) ThumbPipeline() thumb.Generator {
	if d.thumbPipeline != nil {
		return d.thumbPipeline
	}

	d.thumbPipeline = thumb.NewPipeline(d.SettingProvider(), d.Logger())
	return d.thumbPipeline
}

func (d *dependency) HashIDEncoder() hashid.Encoder {
	if d.hashidEncoder != nil {
		return d.hashidEncoder
	}

	encoder, err := hashid.New(d.SettingProvider().HashIDSalt(context.Background()))
	if err != nil {
		d.panicError(err)
	}

	d.hashidEncoder = encoder
	return d.hashidEncoder
}

func (d *dependency) SettingProvider() setting.Provider {
	if d.settingProvider != nil {
		return d.settingProvider
	}

	if d.ConfigProvider().GetServer().GetSys().GetMode() == constants.MasterMode {
		// For master mode, setting value will be retrieved in order:
		// KV Store -> DB Setting Store
		d.settingProvider = setting.NewProvider(
			setting.NewKvSettingStore(d.KV(),
				setting.NewDbSettingStore(d.SettingClient(), nil),
			),
		)
	} else {
		// For slave mode, setting value will be retrieved in order:
		// Setting defaults in DB schema
		d.settingProvider = setting.NewProvider(
			setting.NewDbDefaultStore(nil),
		)
	}

	return d.settingProvider
}

func (d *dependency) GeneralAuth() auth.Auth {
	if d.generalAuth != nil {
		return d.generalAuth
	}

	var secretKey string
	if d.ConfigProvider().Server.Sys.Mode == constants.MasterMode {
		secretKey = d.SettingProvider().SecretKey(context.Background())
	} else {
		secretKey = d.ConfigProvider().Slave.Secret
		if secretKey == "" {
			d.panicError(errors.New("SlaveSecret is not set, please specify it in config files"))
		}
	}

	d.generalAuth = auth.HMACAuth{SecretKey: []byte(secretKey)}
	return d.generalAuth
}

func (d *dependency) ForkWithLogger(ctx context.Context, l *log.Helper) context.Context {
	dep := &dependencyCorrelated{
		l:          l,
		dependency: d,
	}
	return context.WithValue(ctx, DepCtx{}, dep)
}

func (d *dependency) Shutdown(ctx context.Context) error {
	d.mu.Lock()

	wg := sync.WaitGroup{}

	if d.mediaMetaQueue != nil {
		wg.Add(1)
		go func() {
			d.mediaMetaQueue.Shutdown()
			defer wg.Done()
		}()
	}

	if d.thumbQueue != nil {
		wg.Add(1)
		go func() {
			d.thumbQueue.Shutdown()
			defer wg.Done()
		}()
	}

	if d.ioIntenseQueue != nil {
		wg.Add(1)
		go func() {
			d.ioIntenseQueue.Shutdown()
			defer wg.Done()
		}()
	}

	if d.entityRecycleQueue != nil {
		wg.Add(1)
		go func() {
			d.entityRecycleQueue.Shutdown()
			defer wg.Done()
		}()
	}

	if d.slaveQueue != nil {
		wg.Add(1)
		go func() {
			d.slaveQueue.Shutdown()
			defer wg.Done()
		}()
	}

	if d.remoteDownloadQueue != nil {
		wg.Add(1)
		go func() {
			d.remoteDownloadQueue.Shutdown()
			defer wg.Done()
		}()
	}

	d.mu.Unlock()
	wg.Wait()

	return nil
}

func (d *dependency) panicError(err error) {
	if d.logger != nil {
		d.logger.Panic("Fatal error in dependency initialization: %s", err)
	}

	panic(err)
}

type dependencyCorrelated struct {
	l *log.Helper
	*dependency
}

func (d *dependencyCorrelated) Logger() *log.Helper {
	return d.l
}
