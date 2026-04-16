package dbfs

import (
	pb "api/api/file/common/v1"
	userpb "api/api/user/common/v1"
	"common/boolset"
	"common/cache"
	"common/constants"
	"common/hashid"
	"context"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/setting"
	"file/internal/data"
	"file/internal/data/types"
	"file/internal/pkg/utils"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
)

var (
	trashNavigatorCapability = &boolset.BooleanSet{}
	defaultTrashView         = &pb.ExplorerView{
		View: pb.ViewType_VIEW_TYPE_LIST,
		Columns: []*pb.ListViewColumn{
			{
				Type: 0,
			},
			{
				Type: 2,
			},
			{
				Type: 8,
			},
			{
				Type: 7,
			},
		},
	}
)

// NewTrashNavigator creates a navigator for userId's "trash" files system.
func NewTrashNavigator(u *userpb.User, fileClient data.FileClient, l log.Logger, config *setting.DBFS,
	hasher hashid.Encoder) Navigator {
	return &trashNavigator{
		user:          u,
		l:             log.NewHelper(l, log.WithMessageKey("trashNavigator")),
		fileClient:    fileClient,
		config:        config,
		baseNavigator: newBaseNavigator(fileClient, defaultFilter, u.Id, hasher, config),
	}
}

type trashNavigator struct {
	l          *log.Helper
	user       *userpb.User
	fileClient data.FileClient
	config     *setting.DBFS

	*baseNavigator
}

func (n *trashNavigator) Recycle() {

}

func (n *trashNavigator) PersistState(kv cache.Driver, key string) {
}

func (n *trashNavigator) RestoreState(s State) error {
	return nil
}

func (n *trashNavigator) To(ctx context.Context, path *fs.URI) (*File, error) {
	// Anonymous userId does not have a trash folder.
	if data.IsAnonymousUser(n.user) {
		return nil, ErrLoginRequired
	}

	elements := path.Elements()
	if len(elements) > 1 {
		// Trash folder is a flatten tree, only 1 layer is supported.
		return nil, fs.ErrPathNotExist.WithCause(fmt.Errorf("invalid Path %q", path))
	}

	if len(elements) == 0 {
		// Trash folder has no root.
		return nil, nil
	}

	current, err := n.walkNext(ctx, nil, elements[0], true)
	if err != nil {
		return nil, fmt.Errorf("failed to walk into %q: %w", elements[0], err)
	}

	current.Path[pathIndexUser] = newTrashUri(current.Model.Name)
	current.Path[pathIndexRoot] = current.Path[pathIndexUser]
	current.OwnerModel = n.user
	return current, nil
}

func (n *trashNavigator) Children(ctx context.Context, parent *File, args *ListArgs) (*ListResult, error) {
	if parent != nil {
		return nil, fs.ErrPathNotExist
	}

	res, err := n.baseNavigator.children(ctx, nil, args)
	if err != nil {
		return nil, err
	}

	// Adding userId uri for each files.
	for i := 0; i < len(res.Files); i++ {
		res.Files[i].Path[pathIndexUser] = newTrashUri(res.Files[i].Model.Name)
	}

	return res, nil
}

func (n *trashNavigator) Capabilities(isSearching bool) *fs.NavigatorProps {
	res := &fs.NavigatorProps{
		Capability:            trashNavigatorCapability,
		OrderDirectionOptions: fullOrderDirectionOption,
		OrderByOptions:        fullOrderByOption,
		MaxPageSize:           n.config.MaxPageSize,
	}

	if isSearching {
		res.OrderByOptions = searchLimitedOrderByOption
	}

	return res
}

func (n *trashNavigator) Walk(ctx context.Context, levelFiles []*File, limit, depth int, f WalkFunc) error {
	return n.baseNavigator.walk(ctx, levelFiles, limit, depth, f)
}

func (n *trashNavigator) FollowTx(ctx context.Context) (func(), error) {
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

func (n *trashNavigator) ExecuteHook(ctx context.Context, hookType fs.HookType, file *File) error {
	return nil
}

func (n *trashNavigator) GetView(ctx context.Context, file *File) *types.ExplorerView {
	if view, ok := n.user.Settings.FsViewMap[string(constants.FileSystemTrash)]; ok {
		return utils.FromProtoView(view)
	}
	return getDefaultView()
}
