package manager

import (
	pbfile "api/api/file/files/v1"
	userpb "api/api/user/common/v1"
	pbuser "api/api/user/users/v1"
	ftypes "api/external/data/file"
	"common/auth"
	"common/cache"
	"common/constants"
	"common/hashid"
	"common/serializer"
	"context"
	"file/ent"
	"file/internal/biz/credmanager"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/driver"
	"file/internal/biz/filemanager/encrypt"
	"file/internal/biz/filemanager/eventhub"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/fs/dbfs"
	"file/internal/biz/filemanager/fs/mime"
	"file/internal/biz/mediameta"
	"file/internal/biz/queue"
	"file/internal/biz/setting"
	"file/internal/biz/thumb"
	"file/internal/conf"
	"file/internal/data"
	"file/internal/data/rpc"
	"file/internal/data/types"
	"io"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

var (
	ErrUnknownPolicyType = serializer.NewError(serializer.CodeInternalSetting, "Unknown policy type", nil)
)

const (
	UploadSessionCachePrefix = "callback_"
	// Ctx key for upload session
	UploadSessionCtx = "uploadSession"
)

type (
	FileOperation interface {
		// Get gets files object by given path
		Get(ctx context.Context, path *fs.URI, opts ...fs.Option) (fs.File, error)
		// List lists files under given path
		List(ctx context.Context, path *fs.URI, args *ListArgs) (fs.File, *fs.ListFileResult, error)
		// Create creates a files or directory
		Create(ctx context.Context, path *fs.URI, fileType int, opt ...fs.Option) (fs.File, error)
		// Rename renames a files or directory
		Rename(ctx context.Context, path *fs.URI, newName string) (fs.File, error)
		// Delete deletes a group of files or directory. UnlinkOnly indicates whether to delete files record in DB only.
		Delete(ctx context.Context, path []*fs.URI, opts ...fs.Option) error
		// Restore restores a group of files
		Restore(ctx context.Context, path ...*fs.URI) error
		// MoveOrCopy moves or copies a group of files
		MoveOrCopy(ctx context.Context, src []*fs.URI, dst *fs.URI, isCopy bool) error
		// Update puts files content. If given files does not exist, it will create a new one.
		Update(ctx context.Context, req *fs.UploadRequest, opts ...fs.Option) (fs.File, error)
		// Walk walks through given path
		Walk(ctx context.Context, path *fs.URI, depth int, f fs.WalkFunc, opts ...fs.Option) error
		// UpsertMedata update or insert metadata of given files
		PatchMedata(ctx context.Context, path []*fs.URI, data ...*pbfile.MetadataPatch) error
		// CreateViewerSession creates a viewer session for given files
		CreateViewerSession(ctx context.Context, uri *fs.URI, version string, viewer *ftypes.Viewer) (*ViewerSession, error)
		// TraverseFile traverses a files to its root files, return the files with linked root.
		TraverseFile(ctx context.Context, fileID int) (fs.File, error)
	}

	FsManagement interface {
		// SharedAddressTranslation translates shared symbolic address to real address. If path does not exist,
		// most recent existing parent directory will be returned.
		SharedAddressTranslation(ctx context.Context, path *fs.URI, opts ...fs.Option) (fs.File, *fs.URI, error)
		// Capacity gets capacity of current files system
		Capacity(ctx context.Context) (*fs.Capacity, error)
		// CheckIfCapacityExceeded checks if given users's capacity exceeded, and send notification email
		CheckIfCapacityExceeded(ctx context.Context) error
		// LocalDriver gets local driver for operating local files.
		LocalDriver(policy *ent.StoragePolicy) driver.Handler
		// CastStoragePolicyOnSlave check if given storage policy need to be casted to another.
		// It is used on slave node, when local policy need to cast to remote policy;
		// Remote policy with same node ID can be casted to local policy.
		CastStoragePolicyOnSlave(ctx context.Context, policy *ent.StoragePolicy) *ent.StoragePolicy
		// GetStorageDriver gets storage driver for given policy
		GetStorageDriver(ctx context.Context, policy *ent.StoragePolicy) (driver.Handler, error)
		// PatchView patches the view setting of a files
		PatchView(ctx context.Context, uri *fs.URI, view *types.ExplorerView) error
	}

	ShareManagement interface {
		// CreateShare creates a share link for given path
		CreateOrUpdateShare(ctx context.Context, path *fs.URI, args *CreateShareArgs) (*ent.Share, error)
	}

	Archiver interface {
		// CreateArchive creates an archive
		CreateArchive(ctx context.Context, uris []*fs.URI, writer io.Writer, opts ...fs.Option) (int, error)
		// ListArchiveFiles lists files in an archive
		ListArchiveFiles(ctx context.Context, uri *fs.URI, entity, zipEncoding string) ([]*pbfile.ArchivedFile, error)
	}

	FileManager interface {
		fs.LockSystem
		FileOperation
		EntityManagement
		UploadManagement
		FsManagement
		ShareManagement
		Archiver

		// Recycle reset current FileManager object and put back to resource pool
		Recycle()
	}

	// GetEntityUrlArgs single args to get entity url
	GetEntityUrlArgs struct {
		URI               *fs.URI
		PreferredEntityID string
	}

	// CreateShareArgs args to create share link
	CreateShareArgs struct {
		ExistedShareID  int
		IsPrivate       bool
		Password        string
		RemainDownloads int
		Expire          *time.Time
		ShareView       bool
		ShowReadMe      bool
	}

	FullTextSearchResults struct {
		Hits  []FullTextSearchResult
		Total int64
	}

	FullTextSearchResult struct {
		File    fs.File
		Content string
	}
)

type manager struct {
	user             *userpb.User
	l                *log.Helper
	fs               fs.FileSystem
	settings         setting.Provider
	kv               cache.Driver
	config           *conf.Bootstrap
	stateless        bool
	auth             auth.Auth
	hasher           hashid.Encoder
	pc               data.StoragePolicyClient
	sc               data.ShareClient
	fc               data.FileClient
	uc               pbuser.UserClient
	tc               data.TaskClient
	mm               mime.MimeManager
	credm            credmanager.CredManager
	qm               *queue.QueueManager
	extractor        mediameta.ExtractorStateManager
	thumbPipe        thumb.Generator
	encryptorFactory encrypt.CryptorFactory
	eventHub         eventhub.EventHub

	aikc *rpc.KnowledgeClient
}

func NewFileManager(dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep, user *userpb.User) FileManager {
	if dep.Config().Server.Sys.Mode == constants.SlaveMode || user == nil {
		return newStatelessFileManager(dep)
	}
	return &manager{
		l:        dep.Logger(),
		user:     user,
		settings: dep.SettingProvider(),
		fs: dbfs.NewDatabaseFS(user, dbfsDep, dep.UserClient(), dep.FileClient(), dep.ShareClient(), dep.Logger(),
			dep.SettingProvider(), dep.PolicyClient(), dep.Encoder(), dep.KV(), dep.EncryptorFactory(), dep.EventHub()),
		kv:               dep.KV(),
		config:           dep.Config(),
		auth:             dep.GeneralAuth(),
		hasher:           dep.Encoder(),
		pc:               dep.PolicyClient(),
		sc:               dep.ShareClient(),
		fc:               dep.FileClient(),
		uc:               dep.UserClient(),
		tc:               dep.TaskClient(),
		mm:               dep.MimeManager(),
		credm:            dep.CredManager(),
		extractor:        dep.ExtractorManager(),
		thumbPipe:        dep.ThumbnailPipeline(),
		qm:               dep.QueuManager(),
		encryptorFactory: dep.EncryptorFactory(),
		eventHub:         dep.EventHub(),
	}
}

func newStatelessFileManager(dep filemanager.ManagerDep) FileManager {
	return &manager{
		l:         dep.Logger(),
		settings:  dep.SettingProvider(),
		kv:        dep.KV(),
		config:    dep.Config(),
		stateless: true,
		auth:      dep.GeneralAuth(),
		hasher:    dep.Encoder(),
	}
}

func (m *manager) Recycle() {
	if m.fs != nil {
		m.fs.Recycle()
	}
}

func newOption() *fs.FsOption {
	return &fs.FsOption{}
}
