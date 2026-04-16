package admin

import (
	commonpb "api/api/common/v1"
	pb "api/api/file/admin/v1"
	filepb "api/api/file/common/v1"
	pbexplorer "api/api/file/workflow/v1"
	ftypes "api/external/data/file"
	"common/hashid"
	"common/request"
	"common/util"
	"context"
	"errors"
	"file/ent"
	"file/ent/task"
	"file/internal/biz/filemanager/manager"
	"file/internal/biz/queue"
	"file/internal/biz/setting"
	"file/internal/biz/thumb"
	"file/internal/data"
	"file/internal/pkg/utils"
	"file/internal/pkg/wopi"
	"net/http"
	"strconv"

	"github.com/samber/lo"
	"github.com/wneessen/go-mail"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	taskTypeCondition    = "task_type"
	taskStatusCondition  = "task_status"
	taskTraceIDCondition = "task_correlation_id"
	taskUserIDCondition  = "task_user_id"
)

func (s *AdminService) FetchWopi(ctx context.Context, req *pb.FetchWopiDiscoveryRequest) (*filepb.ViewerGroup, error) {
	requestClient := request.NewClient(s.conf.Server.Sys.Mode, request.WithContext(ctx), request.WithLogger(s.l.Logger()))
	content, err := requestClient.Request(http.MethodGet, req.Endpoint, nil).CheckHTTPResponse(http.StatusOK).GetResponse()
	if err != nil {
		return nil, commonpb.ErrorInternalSetting("WOPI endpoint id unavailable: %w", err)
	}

	vg, err := wopi.DiscoveryXmlToViewerGroup(content)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Failed to parse WOPI response: %w", err)
	}

	return ftypes.ToProtoViewerGroup(vg), nil
}
func (s *AdminService) TestThumbGenerator(ctx context.Context, req *pb.TestThumbGeneratorRequest) (*pb.TestThumbGeneratorResponse, error) {
	version, err := thumb.TestGenerator(ctx, req.Name, req.Executable)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Failed to invoke generator: %w", err)
	}

	return &pb.TestThumbGeneratorResponse{
		Version: version,
	}, nil
}
func (s *AdminService) SendTestMail(ctx context.Context, req *pb.TestMailRequest) (*emptypb.Empty, error) {
	port, err := strconv.Atoi(req.Settings["smtpPort"])
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Invalid SMTP port: %w", err)
	}

	opts := []mail.Option{
		mail.WithPort(port),
		mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover),
		mail.WithTLSPolicy(mail.TLSOpportunistic),
		mail.WithUsername(req.Settings["smtpUser"]),
		mail.WithPassword(req.Settings["smtpPass"]),
	}
	if setting.IsTrueValue(req.Settings["smtpEncryption"]) {
		opts = append(opts, mail.WithSSL())
	}

	d, diaErr := mail.NewClient(req.Settings["smtpHost"], opts...)
	if diaErr != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to create SMTP client: %w", diaErr)
	}

	m := mail.NewMsg()
	if err := m.FromFormat(req.Settings["fromName"], req.Settings["fromAddress"]); err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to set FROM address: %w", err)
	}
	m.ReplyToFormat(req.Settings["fromName"], req.Settings["replyTo"])
	m.To(req.To)
	m.Subject("Cloudreve SMTP Test")
	m.SetMessageID()
	m.SetBodyString(mail.TypeTextHTML, "This is a test email from Cloudreve.")

	err = d.DialAndSendWithContext(ctx, m)
	if err != nil {
		// 检查是否是成功发送后SMTP重设错误
		var sendErr *mail.SendError
		var errParsed = errors.As(err, &sendErr)
		if errParsed && sendErr.Reason == mail.ErrSMTPReset {
			return nil, nil
		}

		return nil, commonpb.ErrorInternalSetting("Failed to send test email: %w", err)
	}
	// 检查是否是成功发送后SMTP重设错误
	return &emptypb.Empty{}, nil
}
func (s *AdminService) ClearEntityUrlCache(ctx context.Context, req *emptypb.Empty) (*emptypb.Empty, error) {
	s.kv.Delete(manager.EntityUrlCacheKeyPrefix)
	return &emptypb.Empty{}, nil
}
func (s *AdminService) GetQueueMetrics(ctx context.Context, req *emptypb.Empty) (*pb.QueueMetricsResponse, error) {
	var res []*pb.QueueMetric

	mediaMeta := s.qm.GetMediaMetaQueue()
	entityRecycle := s.qm.GetEntityRecycleQueue()
	ioIntense := s.qm.GetIoIntenseQueue()
	remoteDownload := s.qm.GetRemoteDownloadQueue()
	thumb := s.qm.GetThumbQueue()

	res = append(res, &pb.QueueMetric{
		Name:            string(setting.QueueTypeMediaMeta),
		BusyWorkers:     mediaMeta.BusyWorkers(),
		SuccessTasks:    mediaMeta.SuccessTasks(),
		FailedTasks:     mediaMeta.FailureTasks(),
		SubmittedTasks:  mediaMeta.SubmittedTasks(),
		SuspendingTasks: mediaMeta.SuspendingTasks(),
	})
	res = append(res, &pb.QueueMetric{
		Name:            string(setting.QueueTypeEntityRecycle),
		BusyWorkers:     entityRecycle.BusyWorkers(),
		SuccessTasks:    entityRecycle.SuccessTasks(),
		FailedTasks:     entityRecycle.FailureTasks(),
		SubmittedTasks:  entityRecycle.SubmittedTasks(),
		SuspendingTasks: entityRecycle.SuspendingTasks(),
	})
	res = append(res, &pb.QueueMetric{
		Name:            string(setting.QueueTypeIOIntense),
		BusyWorkers:     ioIntense.BusyWorkers(),
		SuccessTasks:    ioIntense.SuccessTasks(),
		FailedTasks:     ioIntense.FailureTasks(),
		SubmittedTasks:  ioIntense.SubmittedTasks(),
		SuspendingTasks: ioIntense.SuspendingTasks(),
	})
	res = append(res, &pb.QueueMetric{
		Name:            string(setting.QueueTypeRemoteDownload),
		BusyWorkers:     remoteDownload.BusyWorkers(),
		SuccessTasks:    remoteDownload.SuccessTasks(),
		FailedTasks:     remoteDownload.FailureTasks(),
		SubmittedTasks:  remoteDownload.SubmittedTasks(),
		SuspendingTasks: remoteDownload.SuspendingTasks(),
	})
	res = append(res, &pb.QueueMetric{
		Name:            string(setting.QueueTypeThumb),
		BusyWorkers:     thumb.BusyWorkers(),
		SuccessTasks:    thumb.SuccessTasks(),
		FailedTasks:     thumb.FailureTasks(),
		SubmittedTasks:  thumb.SubmittedTasks(),
		SuspendingTasks: thumb.SuspendingTasks(),
	})

	return &pb.QueueMetricsResponse{
		Metrics: res,
	}, nil
}
func (s *AdminService) ListTasks(ctx context.Context, req *filepb.ListRequest) (*pb.ListTaskResponse, error) {
	taskClient := s.tc
	hasher := s.hasher
	var (
		err      error
		userID   int
		traceID  string
		status   []task.Status
		taskType []string
	)

	if req.Conditions[taskTypeCondition] != "" {
		taskType = []string{req.Conditions[taskTypeCondition]}
	}

	if req.Conditions[taskStatusCondition] != "" {
		status = []task.Status{task.Status(req.Conditions[taskStatusCondition])}
	}

	if req.Conditions[taskTraceIDCondition] != "" {
		traceID = util.TraceID(ctx)
	}

	if req.Conditions[taskUserIDCondition] != "" {
		userID, err = strconv.Atoi(req.Conditions[taskUserIDCondition])
		if err != nil {
			return nil, commonpb.ErrorParamInvalid("Invalid task users ID: %w", err)
		}
	}

	res, err := taskClient.List(ctx, &data.ListTaskArgs{
		PaginationArgs: &data.PaginationArgs{
			Page:     int(req.Page) - 1,
			PageSize: int(req.PageSize),
			OrderBy:  req.OrderBy,
			Order:    data.OrderDirection(req.OrderDirection),
		},
		UserID:  userID,
		TraceID: traceID,
		Types:   taskType,
		Status:  status,
	})

	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list tasks: %w", err)
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
	nodes, err := s.nc.GetNodeByIds(ctx, lo.Keys(nodeMap))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to query nodes: %w", err)
	}
	for _, n := range nodes {
		nodeMap[n.ID] = n
	}

	return &pb.ListTaskResponse{
		Tasks: lo.Map(res.Tasks, func(task *ent.Task, index int) *pb.GetTaskResponse {
			var (
				uid     string
				node    *ent.Node
				summary *pbexplorer.Summary
			)

			uid = hashid.EncodeUserID(hasher, task.UserID)

			t := tasks[index]
			summary = t.Summarize(hasher)
			if summary != nil && summary.NodeId > 0 {
				node = nodeMap[int(summary.NodeId)]
			}

			return &pb.GetTaskResponse{
				Task:       utils.EntTaskToProto(task),
				TaskHashId: hashid.EncodeTaskID(hasher, task.ID),
				UserHashId: uid,
				Node:       utils.EntNodeToProto(node),
				Summary:    summary,
			}
		}),
		Pagination: res.PaginationResults,
	}, nil
}
func (s *AdminService) GetTask(ctx context.Context, req *pb.SimpleTaskRequest) (*pb.GetTaskResponse, error) {
	taskClient := s.tc
	hasher := s.hasher

	task, err := taskClient.GetTaskByID(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to query task: %w", err)
	}

	t, err := queue.NewTaskFromModel(task)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to parse task: %w", err)
	}

	summary := t.Summarize(hasher)
	var (
		node       *ent.Node
		userHashID string
	)

	if summary != nil && summary.NodeId > 0 {
		node, _ = s.nc.GetNodeById(ctx, int(summary.NodeId))
	}

	if task.UserID > 0 {
		userHashID = hashid.EncodeUserID(hasher, task.UserID)
	}

	return &pb.GetTaskResponse{
		Task:       utils.EntTaskToProto(task),
		TaskHashId: hashid.EncodeTaskID(hasher, task.ID),
		UserHashId: userHashID,
		Node:       utils.EntNodeToProto(node),
		Summary:    summary,
	}, nil
}
func (s *AdminService) BatchDeleteTasks(ctx context.Context, req *pb.BatchDeleteTasksRequest) (*emptypb.Empty, error) {
	taskIds := lo.Map(req.Ids, func(id int32, index int) int {
		return int(id)
	})
	err := s.tc.DeleteByIDs(ctx, taskIds...)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to batch delete tasks: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *AdminService) CleanupTask(ctx context.Context, req *pb.CleanupTaskRequest) (*emptypb.Empty, error) {
	status := lo.Map(req.Status, func(status string, index int) task.Status {
		return task.Status(status)
	})
	if len(req.Status) == 0 {
		status = []task.Status{task.StatusCanceled, task.StatusCompleted, task.StatusError}
	}

	if err := s.tc.DeleteBy(ctx, &data.DeleteTaskArgs{
		NotAfter: req.NoAfter.AsTime(),
		Types:    req.Types,
		Status:   status,
	}); err != nil {
		return nil, commonpb.ErrorDb("Failed to cleanup tasks: %w", err)
	}

	return &emptypb.Empty{}, nil
}

func (s *AdminService) DeleteTaskByUserIds(ctx context.Context, req *pb.DeleteTaskByUserIdsRequest) (*emptypb.Empty, error) {
	if len(req.Ids) == 0 {
		return nil, commonpb.ErrorParamInvalid("IDs are empty")
	}
	uids := lo.Map(req.Ids, func(id int32, index int) int {
		return int(id)
	})
	if err := s.tc.DeleteBy(ctx, &data.DeleteTaskArgs{
		Uids: uids,
	}); err != nil {
		return nil, commonpb.ErrorDb("Failed to delete tasks: %w", err)
	}

	return &emptypb.Empty{}, nil
}
