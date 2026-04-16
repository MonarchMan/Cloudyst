package db

import (
	pb "api/api/common/v1"
	"fmt"

	"entgo.io/ent/dialect/sql"
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
		Page:     int(args.Page),
		PageSize: int(args.PageSize),
		OrderBy:  args.OrderBy,
		OrderDir: OrderDirection(args.OrderDirection),
	}
}

func ConvertPaginationResults(args *PaginationResults) *pb.PaginationResults {
	return &pb.PaginationResults{
		Page:       int32(args.Page),
		PageSize:   int32(args.PageSize),
		TotalItems: int32(args.TotalItems),
	}
}
