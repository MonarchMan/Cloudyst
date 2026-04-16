package dbfs

import (
	userpb "api/api/user/common/v1"
	"common/boolset"
	"common/cache"
	"common/constants"
	"common/hashid"
	"context"
	"errors"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/setting"
	"file/internal/data"
	"file/internal/data/types"
	"file/internal/pkg/utils"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
)

var sharedWithMeNavigatorCapability = &boolset.BooleanSet{}

// NewSharedWithMeNavigator creates a navigator for userId's "shared with me" files system.
func NewSharedWithMeNavigator(u *userpb.User, fileClient data.FileClient, l log.Logger,
	config *setting.DBFS, hasher hashid.Encoder) Navigator {
	n := &sharedWithMeNavigator{
		user:       u,
		l:          log.NewHelper(l, log.WithMessageKey("sharedWithMeNavigator")),
		fileClient: fileClient,
		config:     config,
		hasher:     hasher,
	}
	n.baseNavigator = newBaseNavigator(fileClient, defaultFilter, u.Id, hasher, config)
	return n
}

type sharedWithMeNavigator struct {
	l          *log.Helper
	user       *userpb.User
	fileClient data.FileClient
	config     *setting.DBFS
	hasher     hashid.Encoder

	root *File
	*baseNavigator
}

func (n *sharedWithMeNavigator) Recycle() {

}

func (n *sharedWithMeNavigator) PersistState(kv cache.Driver, key string) {
}

func (n *sharedWithMeNavigator) RestoreState(s State) error {
	return nil
}

func (n *sharedWithMeNavigator) To(ctx context.Context, path *fs.URI) (*File, error) {
	// Anonymous userId does not have a trash folder.
	if data.IsAnonymousUser(n.user) {
		return nil, ErrLoginRequired
	}

	elements := path.Elements()
	if len(elements) > 0 {
		// Shared with me folder is a flatten tree, only root can be accessed.
		n.l.Debugf("Shared with me folder is a flatten tree, only root can be accessed: %q", path)
		return nil, fs.ErrPathNotExist.WithCause(fmt.Errorf("invalid Path %q", path))
	}

	if n.root == nil {
		rootFile, err := n.fileClient.Root(ctx, int(n.user.Id))
		if err != nil {
			n.l.WithContext(ctx).Infof("User's root folder not found: %s, will initialize it.", err)
			return nil, ErrFsNotInitialized
		}

		n.root = newFile(nil, rootFile)
		rootPath := newSharedWithMeUri("")
		n.root.Path[pathIndexRoot], n.root.Path[pathIndexUser] = rootPath, rootPath
		n.root.OwnerModel = n.user
		n.root.IsUserRoot = true
		n.root.CapabilitiesBs = n.Capabilities(false).Capability
	}

	return n.root, nil
}

func (n *sharedWithMeNavigator) Children(ctx context.Context, parent *File, args *ListArgs) (*ListResult, error) {
	args.SharedWithMe = true
	res, err := n.baseNavigator.children(ctx, nil, args)
	if err != nil {
		return nil, err
	}

	// Adding userId uri for each files.
	for i := 0; i < len(res.Files); i++ {
		res.Files[i].Path[pathIndexUser] = newSharedWithMeUri(hashid.EncodeFileID(n.hasher, res.Files[i].Model.ID))
	}

	return res, nil
}

func (n *sharedWithMeNavigator) Capabilities(isSearching bool) *fs.NavigatorProps {
	res := &fs.NavigatorProps{
		Capability:            sharedWithMeNavigatorCapability,
		OrderDirectionOptions: fullOrderDirectionOption,
		OrderByOptions:        fullOrderByOption,
		MaxPageSize:           n.config.MaxPageSize,
	}

	if isSearching {
		res.OrderByOptions = searchLimitedOrderByOption
	}

	return res
}

func (n *sharedWithMeNavigator) Walk(ctx context.Context, levelFiles []*File, limit, depth int, f WalkFunc) error {
	return errors.New("not implemented")
}

func (n *sharedWithMeNavigator) FollowTx(ctx context.Context) (func(), error) {
	if _, ok := ctx.Value(data.TxCtx{}).(*data.Tx); !ok {
		n.l.Debugf("tx is not a transaction, can not inherit it: %v", ctx.Value(data.TxCtx{}))
		return nil, fmt.Errorf("navigator: no inherited transaction found in context")
	}
	newFileClient, _, _, err := data.WithTx(ctx, n.fileClient)
	if err != nil {
		return nil, err
	}

	oldFileClient := n.fileClient
	revert := func() {
		n.fileClient = oldFileClient
		n.baseNavigator.fileClient = oldFileClient
	}

	n.fileClient = newFileClient
	n.baseNavigator.fileClient = newFileClient
	return revert, nil
}

func (n *sharedWithMeNavigator) ExecuteHook(ctx context.Context, hookType fs.HookType, file *File) error {
	return nil
}

func (n *sharedWithMeNavigator) GetView(ctx context.Context, file *File) *types.ExplorerView {
	if view, ok := n.user.Settings.FsViewMap[string(constants.FileSystemSharedWithMe)]; ok {
		return utils.FromProtoView(view)
	}
	return getDefaultView()
}
