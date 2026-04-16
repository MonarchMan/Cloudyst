package workflows

import (
	pb "api/api/file/common/v1"
	pbexplorer "api/api/file/workflow/v1"
	"common/util"
	"context"
	"file/internal/biz/cluster"
	"file/internal/biz/filemanager"
	"file/internal/biz/queue"
	"file/internal/data/types"
	"fmt"
	"path"
	"strconv"
	"time"
)

const (
	TaskTempPath                 = "fm_workflows"
	slaveProgressRefreshInterval = 5 * time.Second
)

type NodeState struct {
	NodeID int `json:"node_id"`

	progress *pbexplorer.TaskPhaseProgressResponse
}

// allocateNode allocates a node for the task.
func allocateNode(ctx context.Context, np cluster.NodePool, state *NodeState, capability types.NodeCapability) (cluster.Node, error) {
	if np == nil {
		return nil, fmt.Errorf("node pool is nil")
	}

	node, err := np.Get(ctx, capability, state.NodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	state.NodeID = node.ID()
	return node, nil
}

// prepareSlaveTaskCtx prepares the context for the slave task.
func prepareSlaveTaskCtx(ctx context.Context, props *pb.SlaveTaskProps) context.Context {
	ctx = context.WithValue(ctx, cluster.SlaveNodeIDCtx{}, strconv.Itoa(int(props.NodeId)))
	ctx = context.WithValue(ctx, cluster.MasterSiteUrlCtx{}, props.MasterSiteUrl)
	ctx = context.WithValue(ctx, cluster.MasterSiteVersionCtx{}, props.MasterSiteVersion)
	ctx = context.WithValue(ctx, cluster.MasterSiteIDCtx{}, props.MasterSiteId)
	return ctx
}

func prepareTempFolder(ctx context.Context, dep filemanager.ManagerDep, t queue.Task) (string, error) {
	settings := dep.SettingProvider()
	tempPath := util.DataPath(path.Join(settings.TempPath(ctx), TaskTempPath, strconv.Itoa(t.ID())))
	if err := util.CreateNestedFolder(tempPath); err != nil {
		return "", fmt.Errorf("failed to create temp folder: %w", err)
	}

	dep.Logger().Info("Temp folder created: %s", tempPath)
	return tempPath, nil
}
