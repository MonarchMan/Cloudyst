package app

import (
	pbuser "api/api/user/users/v1"
	"common/cache"
	"common/constants"
	"common/util"
	"context"
	"file/internal/biz/credmanager"
	"file/internal/biz/crontab"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/driver/onedrive"
	"file/internal/biz/filemanager/fs/mime"
	"file/internal/biz/mediameta"
	"file/internal/biz/queue"
	"file/internal/biz/setting"
	"file/internal/conf"
	"file/internal/data"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
	"go.opentelemetry.io/otel/trace"
)

type (
	Server interface {
		Start() error
		PrintBanner()
		Close()
	}

	server struct {
		logger   *log.Helper
		config   *conf.Bootstrap
		kv       cache.Driver
		pc       data.StoragePolicyClient
		uc       pbuser.UserClient
		credM    credmanager.CredManager
		qm       *queue.QueueManager
		settings setting.Provider
		dep      filemanager.ManagerDep
		dbfsDep  filemanager.DbfsDep
		mm       mime.MimeManager
		em       mediameta.ExtractorStateManager
		tracer   trace.Tracer
	}
)

func NewServer(logger log.Logger, config *conf.Bootstrap, kv cache.Driver, pc data.StoragePolicyClient,
	uc pbuser.UserClient, credM credmanager.CredManager, qm *queue.QueueManager, settings setting.Provider,
	mm mime.MimeManager, em mediameta.ExtractorStateManager, tracerProvider trace.TracerProvider,
	dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep) (Server, func()) {
	s := &server{
		logger:   log.NewHelper(logger, log.WithMessageKey("app-server")),
		config:   config,
		kv:       kv,
		pc:       pc,
		uc:       uc,
		credM:    credM,
		qm:       qm,
		settings: settings,
		dep:      dep,
		dbfsDep:  dbfsDep,
		mm:       mm,
		em:       em,
		tracer:   tracerProvider.Tracer("app-server"),
	}
	return s, s.Close
}

func (s *server) Start() error {
	_ = s.kv.Delete(setting.KvSettingPrefix)
	if memKv, ok := s.kv.(*cache.MemoStore); ok {
		memKv.GarbageCollect(s.logger.Logger())
	}
	ctx := context.Background()
	if s.config.Server.Sys.Mode == constants.MasterMode {
		credentials, err := onedrive.RetrieveOneDriveCredentials(ctx, s.pc)
		if err != nil {
			return fmt.Errorf("faield to retrieve OneDrive credentials for CredManager: %w", err)
		}
		if err := s.credM.Upsert(ctx, credentials...); err != nil {
			return fmt.Errorf("failed to upsert OneDrive credentials to CredManager: %w", err)
		}
		crontab.Register(setting.CronTypeOauthCredRefresh, func(ctx context.Context, q queue.Queue) {
			s.credM.RefreshAll(ctx)
		})

		// start dependencies
		s.mm.Reload(ctx)
		s.em.Reload(ctx)

		// Start all queues
		ctx = context.WithValue(ctx, filemanager.ManagerDepCtx{}, s.dep)
		ctx = context.WithValue(ctx, filemanager.DbfsDepCtx{}, s.dbfsDep)
		s.qm.ReloadMediaMetaQueue(ctx)
		s.qm.GetMediaMetaQueue().Start()
		s.qm.ReloadEntityRecycleQueue(ctx)
		s.qm.GetEntityRecycleQueue().Start()
		s.qm.ReloadIoIntenseQueue(ctx)
		s.qm.GetIoIntenseQueue().Start()
		s.qm.ReloadRemoteDownloadQueue(ctx)
		s.qm.GetRemoteDownloadQueue().Start()

		// Start cron jobs
		//s.logger.Infof("Start cron jobs")
		//_, err = s.uc.GetAnonymousUser(ctx, &emptypb.Empty{})
		//s.logger.WithContext(ctx).Infof("Successfully get anonymous user")
		c, err := crontab.NewCron(ctx, s.settings, s.uc, s.logger, s.qm.GetEntityRecycleQueue(), s.tracer, s.dep, s.dbfsDep)
		if err != nil {
			return err
		}
		c.Start()

	} else {
		s.qm.ReloadMediaMetaQueue(ctx)
		s.qm.GetSlaveQueue().Start()
	}
	s.qm.ReloadThumbQueue(ctx)
	s.qm.GetThumbQueue().Start()

	return nil
}

func (s *server) PrintBanner() {
	fmt.Print(`
   	___ _                _                    
  / __\ | ___  _   _  __| |__ __ _____ _____ 
 / /  | |/ _ \| | | |/ _  | |_| |_  __|_____| 
/ /___| | (_) | |_| | (_| |     |_\ \_  | |
\____/|_|\___/ \__,_|\__,_|\_  /|_____| |_|
                            / /
                           /_/

   V` + constants.BackendVersion + `
================================================

`)
}

func (s *server) Close() {
	if s.kv != nil {
		if err := s.kv.Persist(util.DataPath(cache.DefaultCacheFile)); err != nil {
			s.logger.Warn("Failed to persist cache: %s", err)
		}
	}
}
