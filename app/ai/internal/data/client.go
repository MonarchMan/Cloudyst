package data

import (
	"ai/ent"
	_ "ai/ent/runtime"
	"ai/internal/conf"
	"ai/internal/data/rpc"
	"common/cache"
	"common/logging"
	"context"
	rawsql "database/sql"
	"database/sql/driver"
	"entmodule"
	"entmodule/debug"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"modernc.org/sqlite"
)

func NewDBClient(l log.Logger, userClient rpc.UserClient, kv cache.Driver, config *conf.Bootstrap) (*ent.Client, func(), error) {
	h := log.NewHelper(l, log.WithMessageKey("data"))
	rawClient, err := NewRawEntClient(h, config)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create raw ent client: %w", err)
	}
	client, err := initializeDBClient(h, rawClient, userClient, kv, config.Version)
	cleanup := func() {
		h.Info("Shutting down database connection...")
		if err := rawClient.Close(); err != nil {
			h.Error("Failed to close database connection: %s", err)
		}
	}
	return client, cleanup, err
}

func initializeDBClient(h *log.Helper, client *ent.Client, userClient rpc.UserClient, kv cache.Driver, requiredDbVersion string) (*ent.Client, error) {
	ctx := context.WithValue(context.Background(), logging.LoggerCtx{}, h)
	if needMigration(ctx, userClient, requiredDbVersion) {
		if err := migrate(ctx, client, userClient, requiredDbVersion, h); err != nil {
			return nil, fmt.Errorf("failed to migrate database: %w", err)
		}
	} else {
		h.Info("Database schema is up to date.")
	}

	return client, nil
}

func NewRawEntClient(l *log.Helper, config *conf.Bootstrap) (*ent.Client, error) {
	l.Info("Initializing database connection...")
	dbConfig := config.GetData().GetDatabase()
	client, err := entmodule.SqlDriver(
		dbConfig.DbType,
		dbConfig.Source,
		dbConfig.DbFile,
		dbConfig.Host,
		dbConfig.Username,
		dbConfig.Password,
		dbConfig.Name,
		dbConfig.Port,
		dbConfig.Charset,
		dbConfig.UnixSocket,
		l,
	)
	if err != nil {
		return nil, err
	}

	driverOpt := ent.Driver(client)
	// Enable verbose logging for debug mode.
	if config.GetServer().GetSys().Debug {
		l.Debug("Debug mode is enabled for DB client.")
		driverOpt = ent.Driver(debug.DebugWithContext(client, func(ctx context.Context, i ...any) {
			//h := log.NewHelper(logging.FromContext(ctx))
			l.Debugf(i[0].(string), i[1:]...)
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
