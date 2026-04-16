package dbfs

import (
	commonpb "api/api/common/v1"
	pb "api/api/file/common/v1"
	userpb "api/api/user/common/v1"
	"common/boolset"
	"common/cache"
	"common/constants"
	"common/hashid"
	"context"
	"file/ent"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/setting"
	"file/internal/data"
	"file/internal/data/types"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
)

var (
	ErrShareNotFound = commonpb.ErrorParamInvalid("Shared files does not exist")
	ErrNotPurchased  = userpb.ErrorPurchaseRequired("You need to purchased this share", "")
)

const (
	PurchaseTicketHeader = constants.CrHeaderPrefix + "Purchase-Ticket"
)

var shareNavigatorCapability = &boolset.BooleanSet{}

// NewShareNavigator creates a navigator for userId's "shared" files system.
func NewShareNavigator(u *userpb.User, fileClient data.FileClient, shareClient data.ShareClient,
	l log.Logger, config *setting.DBFS, hasher hashid.Encoder) Navigator {
	n := &shareNavigator{
		user:        u,
		l:           log.NewHelper(l, log.WithMessageKey("shareNavigator")),
		fileClient:  fileClient,
		shareClient: shareClient,
		config:      config,
	}
	n.baseNavigator = newBaseNavigator(fileClient, defaultFilter, u.Id, hasher, config)
	return n
}

type (
	shareNavigator struct {
		l           *log.Helper
		user        *userpb.User
		fileClient  data.FileClient
		shareClient data.ShareClient
		config      *setting.DBFS

		*baseNavigator
		shareRoot       *File
		singleFileShare bool
		ownerRoot       *File
		share           *ent.Share
		ownerId         int64
		disableRecycle  bool
		persist         func()
	}

	shareNavigatorState struct {
		ShareRoot       *File
		OwnerRoot       *File
		SingleFileShare bool
		Share           *ent.Share
		Owner           int64
	}
)

func (n *shareNavigator) PersistState(kv cache.Driver, key string) {
	n.disableRecycle = true
	n.persist = func() {
		kv.Set(key, shareNavigatorState{
			ShareRoot:       n.shareRoot,
			OwnerRoot:       n.ownerRoot,
			SingleFileShare: n.singleFileShare,
			Share:           n.share,
			Owner:           n.ownerId,
		}, ContextHintTTL)
	}
}

func (n *shareNavigator) RestoreState(s State) error {
	n.disableRecycle = true
	if state, ok := s.(shareNavigatorState); ok {
		n.shareRoot = state.ShareRoot
		n.ownerRoot = state.OwnerRoot
		n.singleFileShare = state.SingleFileShare
		n.share = state.Share
		n.ownerId = state.Owner
		return nil
	}

	return fmt.Errorf("invalid state type: %T", s)
}

func (n *shareNavigator) Recycle() {
	if n.persist != nil {
		n.persist()
		n.persist = nil
	}

	if !n.disableRecycle {
		if n.ownerRoot != nil {
			n.ownerRoot.Recycle()
		} else if n.shareRoot != nil {
			n.shareRoot.Recycle()
		}
	}
}

func (n *shareNavigator) Root(ctx context.Context, path *fs.URI) (*File, error) {
	ctx = context.WithValue(ctx, data.LoadShareFile{}, true)
	share, err := n.shareClient.GetByHashID(ctx, path.ID(hashid.EncodeUserID(n.hasher, int(n.user.Id))))
	if err != nil {
		return nil, ErrShareNotFound.WithCause(err)
	}

	if err := data.IsValidShare(share); err != nil {
		return nil, ErrShareNotFound.WithCause(err)
	}

	n.ownerId = int64(share.OwnerID)

	// Check password
	if share.Password != "" && share.Password != path.Password() {
		return nil, ErrShareIncorrectPassword
	}

	// Share permission setting should overwrite root folder's permission
	n.shareRoot = newFile(nil, share.Edges.File)

	// Find the userId side root of the files.
	ownerRoot, err := n.findRoot(ctx, n.shareRoot)
	if err != nil {
		return nil, err
	}

	if n.shareRoot.Type() == types.FileTypeFile {
		n.singleFileShare = true
		n.shareRoot = n.shareRoot.Parent
	}

	n.shareRoot.Path[pathIndexUser] = path.Root()
	//n.shareRoot.OwnerModel = n.ownerId
	n.shareRoot.IsUserRoot = true
	n.shareRoot.disableView = (share.Props == nil || !share.Props.ShareView) && n.user.Id != n.ownerId
	n.shareRoot.CapabilitiesBs = n.Capabilities(false).Capability

	// Check if any ancestors is deleted
	if ownerRoot.Name() != data.RootFolderName {
		return nil, ErrShareNotFound
	}

	permissions := boolset.BooleanSet(n.user.Group.Permissions)
	if n.user.Id != n.ownerId && !(&permissions).Enabled(int(types.GroupPermissionShareDownload)) {
		if data.IsAnonymousUser(n.user) {
			n.l.Debugf("Anonymous user does not have permission to access share links: %s", err)
			return nil, commonpb.ErrorAnonymousAccessDenied("You don't have permission to access share links")
		}

		n.l.Debugf("User does not have permission to access share links: %s", err)
		return nil, commonpb.ErrorForbidden("You don't have permission to access share links")
	}

	n.ownerRoot = ownerRoot
	n.ownerRoot.Path[pathIndexRoot] = newMyIDUri(hashid.EncodeUserID(n.hasher, int(n.ownerId)))
	n.share = share
	return n.shareRoot, nil
}

func (n *shareNavigator) To(ctx context.Context, path *fs.URI) (*File, error) {
	if n.shareRoot == nil {
		root, err := n.Root(ctx, path)
		if err != nil {
			return nil, err
		}

		n.shareRoot = root
	}

	current, lastAncestor := n.shareRoot, n.shareRoot
	elements := path.Elements()

	// If target is root of single files share, the root itself is the target.
	if len(elements) == 1 && n.singleFileShare {
		file, err := n.latestSharedSingleFile(ctx)
		if err != nil {
			return nil, err
		}

		if len(elements) == 1 && file.Name() != elements[0] {
			n.l.Debugf("shared single file name %q not match path element %q", file.Name(), elements[0])
			return nil, fs.ErrPathNotExist
		}

		return file, nil
	}

	var err error
	for index, element := range elements {
		lastAncestor = current
		current, err = n.walkNext(ctx, current, element, index == len(elements)-1)
		if err != nil {
			n.l.Debugf("failed to walk into %q: %s", element, err)
			return lastAncestor, fmt.Errorf("failed to walk into %q: %w", element, err)
		}
	}

	return current, nil
}

func (n *shareNavigator) walkNext(ctx context.Context, root *File, next string, isLeaf bool) (*File, error) {
	nextFile, err := n.baseNavigator.walkNext(ctx, root, next, isLeaf)
	if err != nil {
		return nil, err
	}

	return nextFile, nil
}

func (n *shareNavigator) Children(ctx context.Context, parent *File, args *ListArgs) (*ListResult, error) {
	if n.singleFileShare {
		file, err := n.latestSharedSingleFile(ctx)
		if err != nil {
			n.l.Debugf("failed to get shared single file: %s", err)
			return nil, err
		}

		return &ListResult{
			Files:          []*File{file},
			Pagination:     &pb.PaginationResults{},
			SingleFileView: true,
		}, nil
	}

	return n.baseNavigator.children(ctx, parent, args)
}

func (n *shareNavigator) latestSharedSingleFile(ctx context.Context) (*File, error) {
	if n.singleFileShare {
		file, err := n.fileClient.GetByID(ctx, n.share.Edges.File.ID)
		if err != nil {
			return nil, err
		}

		f := newFile(n.shareRoot, file)
		f.OwnerModel = n.shareRoot.OwnerModel

		return f, nil
	}

	return nil, fs.ErrPathNotExist
}

func (n *shareNavigator) Capabilities(isSearching bool) *fs.NavigatorProps {
	res := &fs.NavigatorProps{
		Capability:            sharedWithMeNavigatorCapability,
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

func (n *shareNavigator) FollowTx(ctx context.Context) (func(), error) {
	if _, ok := ctx.Value(data.TxCtx{}).(*data.Tx); !ok {
		n.l.Debugf("navigator: no inherited transaction found in context")
		return nil, fmt.Errorf("navigator: no inherited transaction found in context")
	}
	newFileClient, _, _, err := data.WithTx(ctx, n.fileClient)
	if err != nil {
		n.l.Debugf("failed to create transaction: %s", err)
		return nil, err
	}

	newSharClient, _, _, err := data.WithTx(ctx, n.shareClient)

	oldFileClient, oldShareClient := n.fileClient, n.shareClient
	revert := func() {
		n.fileClient = oldFileClient
		n.shareClient = oldShareClient
		n.baseNavigator.fileClient = oldFileClient
	}

	n.fileClient = newFileClient
	n.shareClient = newSharClient
	n.baseNavigator.fileClient = newFileClient
	return revert, nil
}

func (n *shareNavigator) ExecuteHook(ctx context.Context, hookType fs.HookType, file *File) error {
	switch hookType {
	case fs.HookTypeBeforeDownload:
		if n.singleFileShare {
			return n.shareClient.Downloaded(ctx, n.share)
		}
	}
	return nil
}

func (n *shareNavigator) Walk(ctx context.Context, levelFiles []*File, limit, depth int, f WalkFunc) error {
	return n.baseNavigator.walk(ctx, levelFiles, limit, depth, f)
}

func (n *shareNavigator) GetView(ctx context.Context, file *File) *types.ExplorerView {
	return file.View()
}
