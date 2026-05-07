package service

import (
	"ai/internal/biz/queue"
	"ai/internal/biz/workflow"
	"ai/internal/data"
	pb "api/api/ai/workflow/v1"
	commonpb "api/api/common/v1"
	"api/external/data/common"
	"api/external/trans"
	"common/hashid"
	"context"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
)

type WorkflowService struct {
	pb.UnimplementedWorkflowServer
	wb     workflow.WorkflowBiz
	hasher hashid.Encoder
}

func NewWorkflowService() *WorkflowService {
	return &WorkflowService{}
}

func (s *WorkflowService) ListTasks(ctx context.Context, req *commonpb.ListTasksRequest) (*commonpb.ListTasksResponse, error) {
	user := trans.FromContext(ctx)

	args := &data.ListTaskArgs{
		PaginationArgs: &common.PaginationArgs{
			Page:     int(req.Page),
			PageSize: int(req.PageSize),
		},
		Types:  []string{},
		UserID: user.ID,
	}

	if req.Type == queue.IngestTaskType {
		args.Types = []string{queue.IngestTaskType}
	} else if req.Type == queue.ReindexTaskType {
		args.Types = []string{queue.ReindexTaskType}
	}

	// Get tasks
	res, err := s.wb.ListTasks(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list tasks: %w", err)
	}
	return buildTaskListResponse(s.hasher, res), nil
}
func (s *WorkflowService) GetTask(ctx context.Context, req *pb.SimpleRequest) (*commonpb.TaskResponse, error) {
	id, err := validateID(s.hasher, req.Id, hashid.TaskID, false)
	if err != nil {
		return nil, err
	}
	t, err := s.wb.GetTask(ctx, id)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get task: %w", err)
	}
	return buildTaskResponse(s.hasher, t), nil
}

func (s *WorkflowService) GetTaskPhaseProgress(ctx context.Context, req *pb.SimpleRequest) (*commonpb.TaskPhaseProgressResponse, error) {
	id, err := validateID(s.hasher, req.Id, hashid.TaskID, false)
	if err != nil {
		return nil, err
	}
	progress, err := s.wb.GetTaskPhaseProgress(ctx, id)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get task phase progress: %w", err)
	}
	return common.TaskProgressToProto(progress), nil
}

func (s *WorkflowService) CancelTasks(ctx context.Context, req *pb.CancelTaskRequest) (*emptypb.Empty, error) {
	rawIDs := lo.Uniq(req.Ids)
	ids := make([]int, len(rawIDs))
	var err error
	for i, rawID := range rawIDs {
		ids[i], err = validateID(s.hasher, rawID, hashid.TaskID, false)
		if err != nil {
			return nil, err
		}
	}
	err = s.wb.CancelTasks(ctx, ids, req.Type, req.Terminate)
	if err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to cancel tasks: %w", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *WorkflowService) ResumeTasks(ctx context.Context, req *pb.ResumeTaskRequest) (*emptypb.Empty, error) {
	rawIDs := lo.Uniq(req.Ids)
	ids := make([]int, len(rawIDs))
	var err error
	for i, rawID := range rawIDs {
		ids[i], err = validateID(s.hasher, rawID, hashid.TaskID, false)
		if err != nil {
			return nil, err
		}
	}
	err = s.wb.ResumeTasks(ctx, ids, req.Type)
	if err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to resume tasks: %w", err)
	}
	return &emptypb.Empty{}, nil
}
