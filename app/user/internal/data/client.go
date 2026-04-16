package data

import (
	"common/cache"
	"common/db"
	"common/util"
	"context"
	rawsql "database/sql"
	"database/sql/driver"
	"entmodule/debug"
	"fmt"
	"time"
	"user/ent"
	_ "user/ent/runtime"
	"user/internal/conf"

	"entgo.io/ent/dialect/sql"
	"github.com/go-kratos/kratos/v2/log"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"modernc.org/sqlite"
)

const (
	DBVersionPrefix           = "db_version_"
	EnvDefaultOverwritePrefix = "CR_SETTING_DEFAULT_"
	EnvEnableAria2            = "CR_ENABLE_ARIA2"
)

func NewDBClient(l log.Logger, kv cache.Driver, config *conf.Bootstrap) (*ent.Client, func(), error) {
	h := log.NewHelper(l, log.WithMessageKey("data"))
	rawClient, err := NewRawEntClient(h, config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create raw ent client: %w", err)
	}
	client, err := InitializeDBClient(h, rawClient, kv, config.Version)
	cleanup := func() {
		h.Info("Shutting down database connection...")
		if err := rawClient.Close(); err != nil {
			h.Error("Failed to close database connection: %s", err)
		}
	}
	return client, cleanup, err
}

// InitializeDBClient runs migration and returns a new ent.Client with additional configurations
// for hooks and interceptors.
func InitializeDBClient(l *log.Helper, client *ent.Client, kv cache.Driver, requiredDbVersion string) (*ent.Client, error) {
	ctx := context.Background()
	if needMigration(client, ctx, requiredDbVersion) {
		// Run the auto migration tool.
		if err := migrate(l, client, ctx, kv, requiredDbVersion); err != nil {
			return nil, fmt.Errorf("failed to migrate database: %w", err)
		}
	} else {
		l.Info("Database schema is up to date.")
	}

	//createMockData(client, ctx)
	return client, nil
}

// NewRawEntClient returns a new ent.Client without additional configurations.
func NewRawEntClient(l *log.Helper, config *conf.Bootstrap) (*ent.Client, error) {
	l.Info("Initializing database connection...")
	dbConfig := config.GetData().GetDatabase()
	confDBType := db.DBType(dbConfig.DbType)
	if confDBType == db.SQLite3DB || confDBType == "" {
		confDBType = db.SQLiteDB
	}
	if confDBType == db.MariaDB {
		confDBType = db.MySqlDB
	}

	var (
		err    error
		client *sql.Driver
	)

	// Check if the database type is supported.
	if confDBType != db.SQLiteDB && confDBType != db.MySqlDB && confDBType != db.PostgresDB {
		return nil, fmt.Errorf("unsupported database type: %s", confDBType)
	}
	// If Database connection string provided, use it directly.
	if dbConfig.Source != "" {
		l.Info("Connect to database with connection string %q.", dbConfig.Source)
		client, err = sql.Open(string(confDBType), dbConfig.Source)
	} else {

		switch confDBType {
		case db.SQLiteDB:
			dbFile := util.RelativePath(dbConfig.DbFile)
			l.Info("Connect to SQLite database %q.", dbFile)
			client, err = sql.Open("sqlite3", util.RelativePath(dbConfig.DbFile))
		case db.PostgresDB:
			l.Info("Connect to Postgres database %q.", dbConfig.Host)
			client, err = sql.Open("postgres", fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable",
				dbConfig.Host,
				dbConfig.Username,
				dbConfig.Password,
				dbConfig.Name,
				dbConfig.Port))
		case db.MySqlDB, db.MsSqlDB:
			l.Info("Connect to MySQL/SQLServer database %q.", dbConfig.Host)
			var host string
			if dbConfig.UnixSocket {
				host = fmt.Sprintf("unix(%s)",
					dbConfig.Host)
			} else {
				host = fmt.Sprintf("(%s:%d)",
					dbConfig.Host,
					dbConfig.Port)
			}

			client, err = sql.Open(string(confDBType), fmt.Sprintf("%s:%s@%s/%s?charset=%s&parseTime=True&loc=Local",
				dbConfig.Username,
				dbConfig.Password,
				host,
				dbConfig.Name,
				dbConfig.Charset))
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

	driverOpt := ent.Driver(client)

	// Enable verbose logging for debug mode.
	if config.GetServer().GetSys().Debug {
		l.Debug("Debug mode is enabled for DB client.")
		driverOpt = ent.Driver(debug.DebugWithContext(client, func(ctx context.Context, i ...any) {
			//h := log.NewHelper(logging.FromContext(ctx))
			l.WithContext(ctx).Debugf(i[0].(string), i[1:]...)
		}))
	}

	return ent.NewClient(driverOpt), nil
}

type sqlite3Driver struct {
	*sqlite.Driver
}

type sqlite3DriverConn interface {
	Exec(string, []driver.Value) (driver.Result, error)
}

func (d sqlite3Driver) Open(name string) (conn driver.Conn, err error) {
	conn, err = d.Driver.Open(name)
	if err != nil {
		return
	}
	_, err = conn.(sqlite3DriverConn).Exec("PRAGMA foreign_keys = ON;", nil)
	if err != nil {
		_ = conn.Close()
	}
	return
}

func init() {
	rawsql.Register("sqlite3", sqlite3Driver{Driver: &sqlite.Driver{}})
}
