package service

import (
	commonpb "api/api/common/v1"
	filepb "api/api/file/common/v1"
	pbexplorer "api/api/file/workflow/v1"
	pbadmin "api/api/user/admin/v1"
	"api/external/trans"
	"common/boolset"
	"common/hashid"
	"common/serializer"
	"context"
	"file/ent"
	"file/ent/task"
	"file/internal/biz/filemanager"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/fs/dbfs"
	"file/internal/biz/filemanager/manager"
	"file/internal/biz/filemanager/workflows"
	"file/internal/biz/queue"
	"file/internal/data"
	"file/internal/data/types"
	"time"

	"github.com/gofrs/uuid"
	"github.com/samber/lo"
	"golang.org/x/tools/container/intsets"
	"google.golang.org/protobuf/types/known/emptypb"
)

type WorkflowService struct {
	pbexplorer.UnimplementedWorkflowServer
	dep      filemanager.ManagerDep
	dbfsDep  filemanager.DbfsDep
	hasher   hashid.Encoder
	qm       *queue.QueueManager
	tc       data.TaskClient
	nc       data.NodeClient
	ac       pbadmin.AdminClient
	registry queue.TaskRegistry
}

func NewWorkflowService(dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep, hasher hashid.Encoder, qm *queue.QueueManager,
	tc data.TaskClient, nc data.NodeClient, ac pbadmin.AdminClient, registry queue.TaskRegistry) *WorkflowService {
	return &WorkflowService{
		dep:      dep,
		dbfsDep:  dbfsDep,
		hasher:   hasher,
		qm:       qm,
		tc:       tc,
		nc:       nc,
		ac:       ac,
		registry: registry,
	}
}

func (s *WorkflowService) ImportFiles(ctx context.Context, req *pbexplorer.ImportWorkflowRequest) (*pbexplorer.TaskResponse, error) {
	user := trans.FromContext(ctx)
	hasher := s.hasher
	m := manager.NewFileManager(s.dep, s.dbfsDep, user)
	defer m.Recycle()

	permissions := boolset.BooleanSet(user.Group.Permissions)
	if !permissions.Enabled(int(types.GroupPermissionIsAdmin)) {
		return nil, pbadmin.ErrorGroupNotAllowed("Only admin can import files")
	}

	userId, err := hasher.Decode(req.UserId, hashid.UserID)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("invalid users id: %w", err)
	}

	owner, err := s.ac.AdminGetUser(ctx, &pbadmin.SimpleUserRequest{
		Id: int32(userId),
	})
	if err != nil || owner.User.Id == 0 {
		return nil, commonpb.ErrorRpcFailed("Failed to get user: %w", err)
	}

	dst, err := fs.NewUriFromString(fs.NewMyUri(req.UserId))
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("invalid destination: %w", err)
	}

	t, err := workflows.NewImportTask(ctx, user, req.Src, req.Recursive, dst.Join(req.Dst).String(), int(req.PolicyId))
	if err != nil {
		return nil, filepb.ErrorCreateTask("create task error: %w", err)
	}

	if err := s.qm.GetIoIntenseQueue().QueueTask(ctx, t); err != nil {
		return nil, filepb.ErrorCreateTask("Failed to queue task: %w", err)
	}

	return buildTaskResponse(t, nil, hasher), nil
}
func (s *WorkflowService) ListTasks(ctx context.Context, req *pbexplorer.ListTasksRequest) (*pbexplorer.ListTasksResponse, error) {
	user := trans.FromContext(ctx)
	hasher := s.hasher
	taskClient := s.tc

	args := &data.ListTaskArgs{
		PaginationArgs: &data.PaginationArgs{
			UseCursorPagination: true,
			PageToken:           req.NextPageToken,
			PageSize:            int(req.PageSize),
		},
		Types:  []string{queue.CreateArchiveTaskType, queue.ExtractArchiveTaskType, queue.RelocateTaskType, queue.ImportTaskType},
		UserID: int(user.Id),
	}

	if req.Category != "general" {
		args.Types = []string{queue.RemoteDownloadTaskType}
		if req.Category == "downloading" {
			args.PageSize = intsets.MaxInt
			args.Status = []task.Status{task.StatusSuspending, task.StatusProcessing, task.StatusQueued}
		} else if req.Category == "downloaded" {
			args.Status = []task.Status{task.StatusCanceled, task.StatusError, task.StatusCompleted}
		}
	}

	// Get tasks
	res, err := taskClient.List(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to query tasks: %w", err)
	}

	tasks := make([]queue.Task, 0, len(res.Tasks))
	nodeMap := make(map[int]*ent.Node)
	for _, t := range res.Tasks {
		task, err := queue.NewTaskFromModel(t)
		if err != nil {
			return nil, commonpb.ErrorDb("Failed to parse task: %w", err)
		}

		summary := task.Summarize(hasher)
		if summary != nil && summary.NodeId > 0 {
			if _, ok := nodeMap[int(summary.NodeId)]; !ok {
				nodeMap[int(summary.NodeId)] = nil
			}
		}
		tasks = append(tasks, task)
	}

	// Get nodes
	nodes, err := s.nc.ListActiveNodes(ctx, lo.Keys(nodeMap))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to query nodes: %w", err)
	}
	for _, n := range nodes {
		nodeMap[n.ID] = n
	}

	// Build response
	return buildTaskListResponse(tasks, res, nodeMap, hasher), nil
}
func (s *WorkflowService) GetTaskPhaseProgress(ctx context.Context, req *pbexplorer.SimpleWorkflowRequest) (*pbexplorer.TaskPhaseProgressResponse, error) {
	taskId, err := s.hasher.Decode(req.Id, hashid.TaskID)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Invalid task id: %w", err)
	}

	user := trans.FromContext(ctx)
	registry := s.registry
	permissions := boolset.BooleanSet(user.Group.Permissions)
	t, found := registry.Get(taskId)
	if !found || t.Owner().Id != user.Id && !permissions.Enabled(int(types.GroupPermissionIsAdmin)) {
		return &pbexplorer.TaskPhaseProgressResponse{}, nil
	}

	return t.Progress(ctx), nil
}
func (s *WorkflowService) CreateArchive(ctx context.Context, req *pbexplorer.ArchiveWorkflowRequest) (*pbexplorer.TaskResponse, error) {
	user := trans.FromContext(ctx)
	hasher := s.hasher
	m := manager.NewFileManager(s.dep, s.dbfsDep, user)
	defer m.Recycle()

	dst, err := fs.NewUriFromString(req.Dst)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("invalid destination: %w", err)
	}

	// Create a placeholder files then delete it to validate the destination
	session, err := m.PrepareUpload(ctx, &fs.UploadRequest{
		Props: &fs.UploadProps{
			Uri:             dst,
			Size:            0,
			UploadSessionID: uuid.Must(uuid.NewV4()).String(),
			ExpireAt:        time.Now().Add(time.Second * 3600),
		},
	})
	if err != nil {
		return nil, err
	}
	m.OnUploadFailed(ctx, session)

	// Create task
	t, err := workflows.NewCreateArchiveTask(ctx, req.Src, req.Dst)
	if err != nil {
		return nil, filepb.ErrorCreateTask("Failed to create task: %w", err)
	}

	if err := s.qm.GetIoIntenseQueue().QueueTask(ctx, t); err != nil {
		return nil, filepb.ErrorCreateTask("Failed to queue task: %w", err)
	}

	return buildTaskResponse(t, nil, hasher), nil
}
func (s *WorkflowService) ExtractArchive(ctx context.Context, req *pbexplorer.ArchiveWorkflowRequest) (*pbexplorer.TaskResponse, error) {
	user := trans.FromContext(ctx)
	hasher := s.hasher
	m := manager.NewFileManager(s.dep, s.dbfsDep, user)
	defer m.Recycle()

	dst, err := fs.NewUriFromString(req.Dst)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("invalid destination: %w", err)
	}

	if len(req.Src) == 0 {
		return nil, commonpb.ErrorParamInvalid("No source files")
	}

	// Validate destination
	if _, err := m.Get(ctx, dst, dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityCreateFile)); err != nil {
		return nil, commonpb.ErrorParamInvalid("Invalid destination: %w", err)
	}

	// Create task
	t, err := workflows.NewExtractArchiveTask(ctx, req.Src[0], req.Dst, req.Encoding, req.Password, req.FileMask)
	if err != nil {
		return nil, filepb.ErrorCreateTask("Failed to create task: %w", err)
	}

	if err := s.qm.GetIoIntenseQueue().QueueTask(ctx, t); err != nil {
		return nil, filepb.ErrorCreateTask("Failed to queue task: %w", err)
	}

	return buildTaskResponse(t, nil, hasher), nil
}
func (s *WorkflowService) CreateRemoteDownload(ctx context.Context, req *pbexplorer.DownloadWorkflowRequest) (*pbexplorer.ListTasksResponse, error) {
	user := trans.FromContext(ctx)
	hasher := s.hasher
	m := manager.NewFileManager(s.dep, s.dbfsDep, user)
	defer m.Recycle()

	if req.SrcFile == "" && len(req.Src) == 0 {
		return nil, commonpb.ErrorParamInvalid("No source files")
	}

	if req.SrcFile != "" && len(req.Src) > 0 {
		return nil, commonpb.ErrorParamInvalid("Invalid source files")
	}

	dst, err := fs.NewUriFromString(req.Dst)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Invalid destination: %w", err)
	}

	// Validate dst
	_, err = m.Get(ctx, dst, dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityCreateFile))
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Invalid destination: %w", err)
	}

	// Validate src files
	if req.SrcFile != "" {
		src, err := fs.NewUriFromString(req.SrcFile)
		if err != nil {
			return nil, commonpb.ErrorParamInvalid("Invalid source files uri: %w", err)
		}

		_, err = m.Get(ctx, src, dbfs.WithRequiredCapabilities(dbfs.NavigatorCapabilityDownloadFile))
		if err != nil {
			return nil, commonpb.ErrorParamInvalid("Invalid source files: %w", err)
		}
	}

	// batch creating tasks
	ae := serializer.NewAggregateError()
	tasks := make([]queue.Task, 0, len(req.Src))
	for _, src := range req.Src {
		if src == "" {
			continue
		}

		t, err := workflows.NewRemoteDownloadTask(ctx, src, req.SrcFile, req.Dst)
		if err != nil {
			ae.Add(src, err)
			continue
		}

		if err := s.qm.GetRemoteDownloadQueue().QueueTask(ctx, t); err != nil {
			ae.Add(src, err)
		}

		tasks = append(tasks, t)
	}

	if req.SrcFile != "" {
		t, err := workflows.NewRemoteDownloadTask(ctx, "", req.SrcFile, req.Dst)
		if err != nil {
			ae.Add(req.SrcFile, err)
		}

		if err := s.qm.GetRemoteDownloadQueue().QueueTask(ctx, t); err != nil {
			ae.Add(req.SrcFile, err)
		}

		tasks = append(tasks, t)
	}

	return &pbexplorer.ListTasksResponse{
		Tasks: lo.Map(tasks, func(item queue.Task, index int) *pbexplorer.TaskResponse {
			return buildTaskResponse(item, nil, hasher)
		}),
	}, ae.Aggregate()
}
func (s *WorkflowService) SetDownloadTaskTarget(ctx context.Context, req *pbexplorer.SetDownloadFilesRequest) (*emptypb.Empty, error) {
	taskId, err := s.hasher.Decode(req.Id, hashid.TaskID)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Invalid task id: %w", err)
	}

	user := trans.FromContext(ctx)
	registry := s.registry
	t, found := registry.Get(taskId)
	if !found || t.Owner().Id != user.Id {
		return nil, commonpb.ErrorParamInvalid("Task not found")
	}

	status := t.Status()
	summary := t.Summarize(s.hasher)
	// Task must be in processing state
	if status != task.StatusSuspending && status != task.StatusProcessing {
		return nil, commonpb.ErrorParamInvalid("Task not in processing state")
	}

	// Task must in monitoring loop
	if summary.Phase != workflows.RemoteDownloadTaskPhaseMonitor {
		return nil, commonpb.ErrorParamInvalid("Task not in monitoring loop")
	}

	if downloadTask, ok := t.(*workflows.RemoteDownloadTask); ok {
		if err := downloadTask.SetDownloadTarget(ctx, req.Files...); err != nil {
			return nil, commonpb.ErrorInternalSetting("Failed to set download files: %w", err)
		}
	}

	return &emptypb.Empty{}, nil
}
func (s *WorkflowService) CancelDownloadTask(ctx context.Context, req *pbexplorer.SimpleWorkflowRequest) (*emptypb.Empty, error) {
	// Validate task id
	taskId, err := s.hasher.Decode(req.Id, hashid.TaskID)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Invalid task id: %w", err)
	}

	user := trans.FromContext(ctx)
	registry := s.registry
	t, found := registry.Get(taskId)
	if !found || t.Owner().Id != user.Id {
		return nil, commonpb.ErrorParamInvalid("Task not found")
	}

	if downloadTask, ok := t.(*workflows.RemoteDownloadTask); ok {
		if err := downloadTask.CancelDownload(ctx); err != nil {
			return nil, commonpb.ErrorInternalSetting("Failed to cancel download task: %w", err)
		}
	}

	return &emptypb.Empty{}, nil
}
