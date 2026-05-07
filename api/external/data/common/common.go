package common

import (
	pb "api/api/common/v1"
	"fmt"
	"queue"

	"entgo.io/ent/dialect/sql"
	"google.golang.org/protobuf/types/known/durationpb"
)

type DBType string

var (
	SQLiteDB   DBType = "sqlite"
	SQLite3DB  DBType = "sqlite3"
	MySqlDB    DBType = "mysql"
	MsSqlDB    DBType = "mssql"
	PostgresDB DBType = "postgres"
	MariaDB    DBType = "mariadb"
)

type (
	OrderDirection string

	UserIDCtx struct{}

	PaginationArgs struct {
		Page     int
		PageSize int
		OrderBy  string
		OrderDir OrderDirection
	}

	PaginationResults struct {
		Page       int
		PageSize   int
		TotalItems int
	}
)

const (
	OrderDirectionAsc  = OrderDirection("asc")
	OrderDirectionDesc = OrderDirection("desc")
)

var (
	ErrTooManyArguments = fmt.Errorf("too many arguments")
)

// SqlParamLimit returns the max number of sql parameters.
func SqlParamLimit(dbType DBType) int {
	switch dbType {
	case PostgresDB:
		return 34464
	case SQLiteDB, SQLite3DB:
		// https://www.sqlite.org/limits.html
		return 32766
	default:
		return 32766
	}
}

// GetOrderTerm returns the order term for ent.
func GetOrderTerm(d OrderDirection) sql.OrderTermOption {
	switch d {
	case OrderDirectionDesc:
		return sql.OrderDesc()
	default:
		return sql.OrderAsc()
	}
}

func CapPageSize(maxSQlParam, preferredSize, margin int) int {
	// Page size should not be bigger than max SQL parameter
	pageSize := preferredSize
	if maxSQlParam > 0 && pageSize > maxSQlParam-margin || pageSize == 0 {
		pageSize = maxSQlParam - margin
	}

	return pageSize
}

func ConvertPaginationArgs(args *pb.PaginationArgs) *PaginationArgs {
	return &PaginationArgs{
		Page:     int(args.Page - 1),
		PageSize: int(args.PageSize),
		OrderBy:  args.OrderBy,
		OrderDir: OrderDirection(args.OrderDirection),
	}
}

func PaginationResultsToProto(args *PaginationResults) *pb.PaginationResults {
	return &pb.PaginationResults{
		Page:       int32(args.Page),
		PageSize:   int32(args.PageSize),
		TotalItems: int32(args.TotalItems),
	}
}

func ConvertListRequestPaginationArgs(args *pb.ListRequest) *PaginationArgs {
	return &PaginationArgs{
		Page:     int(args.Page - 1),
		PageSize: int(args.PageSize),
		OrderBy:  args.OrderBy,
		OrderDir: OrderDirection(args.OrderDirection),
	}
}

func OrderDirectionFromProto(dir pb.OrderDirection) string {
	switch dir {
	case pb.OrderDirection_ORDER_DIRECTION_ASC:
		return "asc"
	case pb.OrderDirection_ORDER_DIRECTION_DESC:
		return "desc"
	default:
		return ""
	}
}

func OrderDirectionToProto(dir string) pb.OrderDirection {
	switch dir {
	case "asc":
		return pb.OrderDirection_ORDER_DIRECTION_ASC
	case "desc":
		return pb.OrderDirection_ORDER_DIRECTION_DESC
	default:
		return pb.OrderDirection_ORDER_DIRECTION_ASC
	}
}

func TaskStatusToProto(status queue.TaskStatus) pb.Task_Status {
	switch status {
	case queue.StatusSuspending:
		return pb.Task_STATUS_SUSPENDING
	case queue.StatusProcessing:
		return pb.Task_STATUS_PROCESSING
	case queue.StatusCompleted:
		return pb.Task_STATUS_COMPLETED
	case queue.StatusCanceled:
		return pb.Task_STATUS_CANCELED
	case queue.StatusError:
		return pb.Task_STATUS_ERROR
	default:
		return pb.Task_STATUS_QUEUED
	}
}

func TaskStatusFromProto(status pb.Task_Status) queue.TaskStatus {
	switch status {
	case pb.Task_STATUS_SUSPENDING:
		return queue.StatusSuspending
	case pb.Task_STATUS_PROCESSING:
		return queue.StatusProcessing
	case pb.Task_STATUS_COMPLETED:
		return queue.StatusCompleted
	case pb.Task_STATUS_CANCELED:
		return queue.StatusCanceled
	case pb.Task_STATUS_ERROR:
		return queue.StatusError
	default:
		return queue.StatusQueued
	}
}

func TaskStateFromProto(state *pb.TaskPublicState) *queue.TaskPublicState {
	return &queue.TaskPublicState{
		Error:            state.Error,
		ErrorHistory:     state.ErrorHistory,
		ExecutedDuration: state.ExecutedDuration.AsDuration(),
		RetryCount:       int(state.RetryCount),
		ResumeTime:       state.ResumeTime,
	}
}

func TaskStateToProto(publicState *queue.TaskPublicState) *pb.TaskPublicState {
	if publicState == nil {
		return nil
	}
	return &pb.TaskPublicState{
		Error:            publicState.Error,
		ErrorHistory:     publicState.ErrorHistory,
		ExecutedDuration: durationpb.New(publicState.ExecutedDuration),
		RetryCount:       int32(publicState.RetryCount),
		ResumeTime:       publicState.ResumeTime,
		//SlaveTaskProps:   SlaveTaskPropsToProto(publicState.SlaveTaskProps),
	}
}

func TaskProgressToProto(progress queue.Progresses) *pb.TaskPhaseProgressResponse {
	progressMap := make(map[string]*pb.Progress)
	for _, progress := range progress {
		progressMap[progress.Identifier] = &pb.Progress{
			Total:      progress.Total,
			Current:    progress.Current,
			Identifier: progress.Identifier,
		}
	}

	return &pb.TaskPhaseProgressResponse{
		ProgressMap: progressMap,
	}
}
