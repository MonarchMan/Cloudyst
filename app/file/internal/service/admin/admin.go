package admin

import (
	pbuser "api/api/user/users/v1"
	"common/auth"
	"common/cache"
	"common/hashid"
	"common/request"
	"file/internal/biz/cluster"
	"file/internal/biz/credmanager"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/encrypt"
	"file/internal/biz/filemanager/fs/mime"
	"file/internal/biz/queue"
	"file/internal/biz/setting"
	"file/internal/conf"
	"file/internal/data"

	pb "api/api/file/admin/v1"

	"github.com/go-kratos/kratos/v2/log"
)

type AdminService struct {
	pb.UnimplementedAdminServer
	conf             *conf.Bootstrap
	kv               cache.Driver
	auth             auth.Auth
	dep              filemanager.ManagerDep
	dbfsDep          filemanager.DbfsDep
	fc               data.FileClient
	pc               data.StoragePolicyClient
	tc               data.TaskClient
	nc               data.NodeClient
	sc               data.ShareClient
	uc               pbuser.UserClient
	rc               request.Client
	hasher           hashid.Encoder
	mm               mime.MimeManager
	settings         setting.Provider
	credm            credmanager.CredManager
	qm               *queue.QueueManager
	np               cluster.NodePool
	l                *log.Helper
	encryptorFactory encrypt.CryptorFactory
}

func NewAdminService(config *conf.Bootstrap, kv cache.Driver, auth auth.Auth, dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep,
	fc data.FileClient, pc data.StoragePolicyClient, tc data.TaskClient, nc data.NodeClient, sc data.ShareClient, uc pbuser.UserClient,
	hasher hashid.Encoder, mm mime.MimeManager, settings setting.Provider, credm credmanager.CredManager, qm *queue.QueueManager,
	np cluster.NodePool, l log.Logger) *AdminService {
	return &AdminService{
		conf:     config,
		kv:       kv,
		auth:     auth,
		dep:      dep,
		dbfsDep:  dbfsDep,
		fc:       fc,
		pc:       pc,
		tc:       tc,
		nc:       nc,
		sc:       sc,
		uc:       uc,
		rc:       request.NewClient(config.Server.Sys.Mode),
		hasher:   hasher,
		mm:       mm,
		settings: settings,
		credm:    credm,
		qm:       qm,
		np:       np,
		l:        log.NewHelper(l, log.WithMessageKey("service-admin")),
	}
}
