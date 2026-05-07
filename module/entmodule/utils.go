package entmodule

import (
	"api/external/data/common"
	"common/util"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/go-kratos/kratos/v2/log"
)

func SqlDriver(dbType string, source string, dbFilePath string, host string, username string, password string, name string,
	port int32, charset string, unixSocket bool, l *log.Helper) (*sql.Driver, error) {
	confDBType := common.DBType(dbType)
	if confDBType == common.SQLite3DB || confDBType == "" {
		confDBType = common.SQLiteDB
	}
	if confDBType == common.MariaDB {
		confDBType = common.MySqlDB
	}

	var (
		err    error
		client *sql.Driver
	)

	// Check if the database type is supported.
	if confDBType != common.SQLiteDB && confDBType != common.MySqlDB && confDBType != common.PostgresDB {
		return nil, fmt.Errorf("unsupported database type: %s", confDBType)
	}
	// If Database connection string provided, use it directly.
	if source != "" {
		l.Info("Connect to database with connection string %q.", source)
		client, err = sql.Open(string(confDBType), source)
	} else {

		switch confDBType {
		case common.SQLiteDB:
			dbFile := util.RelativePath(dbFilePath)
			l.Info("Connect to SQLite database %q.", dbFile)
			client, err = sql.Open("sqlite3", util.RelativePath(dbFile))
		case common.PostgresDB:
			l.Info("Connect to Postgres database %q.", host)
			client, err = sql.Open("postgres", fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable",
				host,
				username,
				password,
				name,
				port))
		case common.MySqlDB, common.MsSqlDB:
			l.Info("Connect to MySQL/SQLServer database %q.", host)
			var host string
			if unixSocket {
				host = fmt.Sprintf("unix(%s)",
					host)
			} else {
				host = fmt.Sprintf("(%s:%d)",
					host,
					port)
			}

			client, err = sql.Open(string(confDBType), fmt.Sprintf("%s:%s@%s/%s?charset=%s&parseTime=True&loc=Local",
				username,
				password,
				host,
				name,
				charset))
		default:
			return nil, fmt.Errorf("unsupported database type %q", confDBType)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to open database: %w", err)
		}

	}
	// Set connection pool
	db := client.DB()
	db.SetMaxIdleConns(50)
	if confDBType == "sqlite" || confDBType == "UNSET" {
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(100)
	}

	// Set timeout
	db.SetConnMaxLifetime(time.Second * 30)
	return client, nil
}
