package dbfs

import (
	userpb "api/api/user/common/v1"
	pbuser "api/api/user/users/v1"
	"common/boolset"
	"common/cache"
	"common/hashid"
	"context"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/setting"
	"file/internal/data"
	"file/internal/data/rpc"
	"file/internal/data/types"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
)

var myNavigatorCapability = &boolset.BooleanSet{}

// NewMyNavigator creates a navigator for userId's "my" files system.
func NewMyNavigator(u *userpb.User, fileClient data.FileClient, userClient pbuser.UserClient, l log.Logger,
	config *setting.DBFS, hasher hashid.Encoder) Navigator {
	return &myNavigator{
		user:          u,
		l:             log.NewHelper(l, log.WithMessageKey("myNavigator")),
		fileClient:    fileClient,
		userClient:    userClient,
		config:        config,
		baseNavigator: newBaseNavigator(fileClient, defaultFilter, u.Id, hasher, config),
	}
}

type myNavigator struct {
	l          *log.Helper
	user       *userpb.User
	fileClient data.FileClient
	userClient pbuser.UserClient

	config *setting.DBFS
	*baseNavigator
	root           *File
	disableRecycle bool
	persist        func()
}

func (n *myNavigator) Recycle() {
	if n.persist != nil {
		n.persist()
		n.persist = nil
	}
	if n.root != nil && !n.disableRecycle {
		n.root.Recycle()
	}
}

func (n *myNavigator) PersistState(kv cache.Driver, key string) {
	n.disableRecycle = true
	n.persist = func() {
		kv.Set(key, n.root, ContextHintTTL)
	}
}

func (n *myNavigator) RestoreState(s State) error {
	n.disableRecycle = true
	if state, ok := s.(*File); ok {
		n.root = state
		return nil
	}

	return fmt.Errorf("invalid state type: %T", s)
}

func (n *myNavigator) To(ctx context.Context, path *fs.URI) (*File, error) {
	if n.root == nil {
		// Anonymous userId does not have a root folder.
		if data.IsAnonymousUser(n.user) {
			return nil, ErrLoginRequired
		}

		fsUid, err := n.hasher.Decode(path.ID(hashid.EncodeUserID(n.hasher, int(n.user.Id))), hashid.UserID)
		if err != nil {
			return nil, fs.ErrPathNotExist.WithCause(fmt.Errorf("invalid userId id"))
		}
		if fsUid != int(n.user.Id) {
			return nil, ErrPermissionDenied
		}

		targetUser, err := rpc.GetUserInfo(ctx, int(n.user.Id), n.userClient)
		if err != nil {
			return nil, fs.ErrPathNotExist.WithCause(fmt.Errorf("userId not found: %w", err))
		}

		permissions := boolset.BooleanSet(n.user.Group.Permissions)
		if targetUser.Status != userpb.User_STATUS_ACTIVE && !(&permissions).Enabled(int(types.GroupPermissionIsAdmin)) {
			return nil, fs.ErrPathNotExist.WithCause(fmt.Errorf("inactive userId"))
		}

		rootFile, err := n.fileClient.Root(ctx, int(targetUser.Id))
		if err != nil {
			n.l.WithContext(ctx).Infof("User's root folder not found: %s, will initialize it.", err)
			return nil, ErrFsNotInitialized
		}

		n.root = newFile(nil, rootFile)
		rootPath := path.Root()
		n.root.Path[pathIndexRoot], n.root.Path[pathIndexUser] = rootPath, rootPath
		n.root.OwnerModel = targetUser
		n.root.disableView = fsUid != int(n.user.Id)
		n.root.IsUserRoot = true
		n.root.CapabilitiesBs = n.Capabilities(false).Capability
	}

	current, lastAncestor := n.root, n.root
	elements := path.Elements()
	var err error
	for index, element := range elements {
		lastAncestor = current
		current, err = n.walkNext(ctx, current, element, index == len(elements)-1)
		if err != nil {
			return lastAncestor, fmt.Errorf("failed to walk into %q: %w", element, err)
		}
	}

	return current, nil
}

func (n *myNavigator) Children(ctx context.Context, parent *File, args *ListArgs) (*ListResult, error) {
	return n.baseNavigator.children(ctx, parent, args)
}

func (n *myNavigator) walkNext(ctx context.Context, root *File, next string, isLeaf bool) (*File, error) {
	return n.baseNavigator.walkNext(ctx, root, next, isLeaf)
}

func (n *myNavigator) Capabilities(isSearching bool) *fs.NavigatorProps {
	res := &fs.NavigatorProps{
		Capability:            myNavigatorCapability,
		OrderDirectionOptions: fullOrderDirectionOption,
		OrderByOptions:        fullOrderByOption,
		MaxPageSize:           n.config.MaxPageSize,
	}
	if isSearching {
		res.OrderByOptions = nil
		res.OrderDirectionOptions = nil
	}

	return res
}

func (n *myNavigator) Walk(ctx context.Context, levelFiles []*File, limit, depth int, f WalkFunc) error {
	return n.baseNavigator.walk(ctx, levelFiles, limit, depth, f)
}

func (n *myNavigator) FollowTx(ctx context.Context) (func(), error) {
	if _, ok := ctx.Value(data.TxCtx{}).(*data.Tx); !ok {
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

func (n *myNavigator) ExecuteHook(ctx context.Context, hookType fs.HookType, file *File) error {
	return nil
}

func (n *myNavigator) GetView(ctx context.Context, file *File) *types.ExplorerView {
	return file.View()
}
