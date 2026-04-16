package cluster

import (
	pb "api/api/file/common/v1"
	pbexplorer "api/api/file/workflow/v1"
	"bytes"
	"common/auth"
	"common/constants"
	"common/request"
	"common/serializer"
	"context"
	"encoding/json"
	"errors"
	"file/ent"
	"file/ent/node"
	"file/ent/task"
	"file/internal/biz/cluster/routes"
	"file/internal/biz/downloader"
	"file/internal/biz/downloader/aria2"
	"file/internal/biz/downloader/qbittorrent"
	"file/internal/biz/downloader/slave"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/setting"
	"file/internal/conf"
	"fmt"
	"strconv"

	"github.com/go-kratos/kratos/v2/log"
)

type (
	Node interface {
		fs.StatelessUploadManager
		ID() int
		Name() string
		IsMaster() bool
		// CreateTask creates a task on the node. It does not have effect on master node.
		CreateTask(ctx context.Context, taskType string, state string) (int, error)
		// GetTask returns the task summary of the task with the given id.
		GetTask(ctx context.Context, id int, clearOnComplete bool) (*SlaveTaskSummary, error)
		// CleanupFolders cleans up the given folders on the node.
		CleanupFolders(ctx context.Context, folders ...string) error
		// AuthInstance returns the auth instance for the node.
		AuthInstance() auth.Auth
		// CreateDownloader creates a downloader instance from the node for remote download tasks.
		CreateDownloader(ctx context.Context, c request.Client, settings setting.Provider) (downloader.Downloader, error)
		// Settings returns the settings of the node.
		Settings(ctx context.Context) *pb.NodeSetting
	}

	// Request body for creating tasks on slave node
	CreateSlaveTask struct {
		Type  string `json:"type"`
		State string `json:"state"`
	}

	// Request body for cleaning up folders on slave node
	FolderCleanup struct {
		Path []string `json:"path" binding:"required"`
	}

	SlaveTaskSummary struct {
		Status       task.Status                           `json:"status"`
		Error        string                                `json:"error"`
		PrivateState string                                `json:"private_state"`
		Progress     *pbexplorer.TaskPhaseProgressResponse `json:"progress,omitempty"`
	}

	MasterSiteUrlCtx     struct{}
	MasterSiteVersionCtx struct{}
	MasterSiteIDCtx      struct{}
	SlaveNodeIDCtx       struct{}
	masterNode           struct {
		nodeBase
		client request.Client
		l      *log.Helper
	}
)

func newNode(model *ent.Node, config *conf.Bootstrap, settings setting.Provider, l log.Logger) Node {
	if model.Type == node.TypeMaster {
		return newMasterNode(model, config, settings, l)
	}
	return newSlaveNode(model, config, settings, l)
}

func newMasterNode(model *ent.Node, config *conf.Bootstrap, settings setting.Provider, l log.Logger) *masterNode {
	n := &masterNode{
		nodeBase: nodeBase{
			model: model,
		},
		l: log.NewHelper(l, log.WithMessageKey("cluster-master")),
	}

	if config.Server.Sys.Mode == constants.SlaveMode {
		n.client = request.NewClient(config.Server.Sys.Mode,
			request.WithCorrelationID(),
			request.WithCredential(auth.HMACAuth{
				[]byte(config.Slave.Secret),
			}, int64(config.Slave.SignatureTtl)),
		)
	}

	return n
}

func (n *masterNode) PrepareUpload(ctx context.Context, args *fs.StatelessPrepareUploadService) (*fs.StatelessPrepareUploadResponse, error) {
	reqBody, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	requestDst := routes.MasterStatelessUrl(MasterSiteUrlFromContext(ctx), "prepare")
	resp, err := n.client.Request(
		"PUT",
		requestDst.String(),
		bytes.NewReader(reqBody),
		request.WithContext(ctx),
		request.WithSlaveMeta(NodeIdFromContext(ctx)),
		request.WithLogger(n.l.Logger()),
	).CheckHTTPResponse(200).DecodeResponse()
	if err != nil {
		return nil, err
	}

	// 处理列取结果
	if resp.Code != 0 {
		return nil, serializer.NewErrorFromResponse(resp)
	}

	uploadRequest := &fs.StatelessPrepareUploadResponse{}
	resp.GobDecode(uploadRequest)

	return uploadRequest, nil
}

func (n *masterNode) CompleteUpload(ctx context.Context, args *fs.StatelessCompleteUploadService) error {
	reqBody, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	requestDst := routes.MasterStatelessUrl(MasterSiteUrlFromContext(ctx), "complete")
	resp, err := n.client.Request(
		"POST",
		requestDst.String(),
		bytes.NewReader(reqBody),
		request.WithContext(ctx),
		request.WithSlaveMeta(NodeIdFromContext(ctx)),
		request.WithLogger(n.l.Logger()),
	).CheckHTTPResponse(200).DecodeResponse()
	if err != nil {
		return err
	}

	// 处理列取结果
	if resp.Code != 0 {
		return serializer.NewErrorFromResponse(resp)
	}

	return nil
}

func (n *masterNode) OnUploadFailed(ctx context.Context, args *fs.StatelessOnUploadFailedService) error {
	reqBody, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	requestDst := routes.MasterStatelessUrl(MasterSiteUrlFromContext(ctx), "failed")
	resp, err := n.client.Request(
		"POST",
		requestDst.String(),
		bytes.NewReader(reqBody),
		request.WithContext(ctx),
		request.WithSlaveMeta(NodeIdFromContext(ctx)),
		request.WithLogger(n.l.Logger()),
	).CheckHTTPResponse(200).DecodeResponse()
	if err != nil {
		return err
	}

	// 处理列取结果
	if resp.Code != 0 {
		return serializer.NewErrorFromResponse(resp)
	}

	return nil
}

func (n *masterNode) CreateFile(ctx context.Context, args *fs.StatelessCreateFileService) error {
	reqBody, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	requestDst := routes.MasterStatelessUrl(MasterSiteUrlFromContext(ctx), "create")
	resp, err := n.client.Request(
		"POST",
		requestDst.String(),
		bytes.NewReader(reqBody),
		request.WithContext(ctx),
		request.WithSlaveMeta(NodeIdFromContext(ctx)),
		request.WithLogger(n.l.Logger()),
	).CheckHTTPResponse(200).DecodeResponse()
	if err != nil {
		return err
	}

	// 处理列取结果
	if resp.Code != 0 {
		return serializer.NewErrorFromResponse(resp)
	}

	return nil

}

func (n *masterNode) CreateDownloader(ctx context.Context, c request.Client, settings setting.Provider) (downloader.Downloader, error) {
	return NewDownloader(log.GetLogger(), c, settings, n.Settings(ctx))
}

// NewDownloader creates a new downloader instance from the node for remote download tasks.
func NewDownloader(l log.Logger, c request.Client, settings setting.Provider, options *pb.NodeSetting) (downloader.Downloader, error) {
	if options.Provider == pb.DownloaderProvider_DOWNLOADER_PROVIDER_QBITTORRENT {
		return qbittorrent.NewClient(l, c, settings, options.Qbittorrent)
	} else if options.Provider == pb.DownloaderProvider_DOWNLOADER_PROVIDER_ARIA2 {
		return aria2.New(l, settings, options.Aria2), nil
	} else if options.Provider == pb.DownloaderProvider_DOWNLOADER_PROVIDER_UNSPECIFIED {
		return nil, errors.New("downloader not configured for this node")
	} else {
		return nil, errors.New("unknown downloader provider")
	}
}

type slaveNode struct {
	nodeBase
	client request.Client
	l      *log.Helper
}

func newSlaveNode(model *ent.Node, config *conf.Bootstrap, settings setting.Provider, l log.Logger) *slaveNode {
	ctx := context.Background()
	siteBasic := settings.SiteBasic(ctx)
	return &slaveNode{
		nodeBase: nodeBase{
			model: model,
		},
		l: log.NewHelper(l, log.WithMessageKey("cluster-slave")),
		client: request.NewClient(config.Server.Sys.Mode,
			request.WithCorrelationID(),
			request.WithSlaveMeta(model.ID),
			request.WithMasterMeta(siteBasic.ID, settings.SiteURL(setting.UseFirstSiteUrl(ctx)).String()),
			request.WithCredential(auth.HMACAuth{[]byte(model.SlaveKey)}, int64(settings.SlaveRequestSignTTL(ctx))),
			request.WithEndpoint(model.Server)),
	}
}

func (n *slaveNode) CreateTask(ctx context.Context, taskType string, state string) (int, error) {
	reqBody, err := json.Marshal(&CreateSlaveTask{
		Type:  taskType,
		State: state,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to marshal request body: %w", err)
	}

	resp, err := n.client.Request(
		"PUT",
		constants.APIPrefixSlave+"/task",
		bytes.NewReader(reqBody),
		request.WithContext(ctx),
		request.WithLogger(n.l.Logger()),
	).CheckHTTPResponse(200).DecodeResponse()
	if err != nil {
		return 0, err
	}

	// 处理列取结果
	if resp.Code != 0 {
		return 0, serializer.NewErrorFromResponse(resp)
	}

	taskId := 0
	if resp.GobDecode(&taskId); taskId > 0 {
		return taskId, nil
	}

	return 0, fmt.Errorf("unexpected response constants: %v", resp.Data)
}

func (n *slaveNode) GetTask(ctx context.Context, id int, clearOnComplete bool) (*SlaveTaskSummary, error) {
	resp, err := n.client.Request(
		"GET",
		routes.SlaveGetTaskRoute(id, clearOnComplete),
		nil,
		request.WithContext(ctx),
		request.WithLogger(n.l.Logger()),
	).CheckHTTPResponse(200).DecodeResponse()
	if err != nil {
		return nil, err
	}

	// 处理列取结果
	if resp.Code != 0 {
		return nil, serializer.NewErrorFromResponse(resp)
	}

	summary := &SlaveTaskSummary{}
	resp.GobDecode(summary)

	return summary, nil
}

func (n *slaveNode) CleanupFolders(ctx context.Context, folders ...string) error {
	args := &FolderCleanup{
		Path: folders,
	}
	reqBody, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	resp, err := n.client.Request(
		"POST",
		constants.APIPrefixSlave+"/task/cleanup",
		bytes.NewReader(reqBody),
		request.WithContext(ctx),
		request.WithLogger(n.l.Logger()),
	).CheckHTTPResponse(200).DecodeResponse()
	if err != nil {
		return err
	}

	// 处理列取结果
	if resp.Code != 0 {
		return serializer.NewErrorFromResponse(resp)
	}

	return nil
}

func (n *slaveNode) CreateDownloader(ctx context.Context, c request.Client, settings setting.Provider) (downloader.Downloader, error) {
	return slave.NewSlaveDownloader(n.client, n.Settings(ctx), n.l.Logger()), nil
}

type nodeBase struct {
	model *ent.Node
}

func (b *nodeBase) ID() int {
	return b.model.ID
}

func (b *nodeBase) Name() string {
	return b.model.Name
}

func (b *nodeBase) IsMaster() bool {
	return b.model.Type == node.TypeMaster
}

func (b *nodeBase) CreateTask(ctx context.Context, taskType string, state string) (int, error) {
	return 0, errors.New("not implemented")
}

func (b *nodeBase) AuthInstance() auth.Auth {
	return auth.HMACAuth{[]byte(b.model.SlaveKey)}
}

func (b *nodeBase) GetTask(ctx context.Context, id int, clearOnComplete bool) (*SlaveTaskSummary, error) {
	return nil, errors.New("not implemented")
}

func (b *nodeBase) CleanupFolders(ctx context.Context, folders ...string) error {
	return errors.New("not implemented")
}

func (b *nodeBase) PrepareUpload(ctx context.Context, args *fs.StatelessPrepareUploadService) (*fs.StatelessPrepareUploadResponse, error) {
	return nil, errors.New("not implemented")
}

func (b *nodeBase) CompleteUpload(ctx context.Context, args *fs.StatelessCompleteUploadService) error {
	return errors.New("not implemented")
}

func (b *nodeBase) OnUploadFailed(ctx context.Context, args *fs.StatelessOnUploadFailedService) error {
	return errors.New("not implemented")
}

func (b *nodeBase) CreateFile(ctx context.Context, args *fs.StatelessCreateFileService) error {
	return errors.New("not implemented")
}

func (b *nodeBase) CreateDownloader(ctx context.Context, c request.Client, settings setting.Provider) (downloader.Downloader, error) {
	return nil, errors.New("not implemented")
}

func (b *nodeBase) Settings(ctx context.Context) *pb.NodeSetting {
	return b.model.Settings
}

func NodeIdFromContext(ctx context.Context) int {
	nodeIdStr, ok := ctx.Value(SlaveNodeIDCtx{}).(string)
	if !ok {
		return 0
	}

	nodeId, _ := strconv.Atoi(nodeIdStr)
	return nodeId
}

func MasterSiteUrlFromContext(ctx context.Context) string {
	if u, ok := ctx.Value(MasterSiteUrlCtx{}).(string); ok {
		return u
	}

	return ""
}
