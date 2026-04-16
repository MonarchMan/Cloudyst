package data

import (
	"ai/ent"
	"ai/internal/data/rpc"
	"common/constants"
	"context"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
)

func needMigration(ctx context.Context, client rpc.UserClient, requiredDbVersion string) bool {
	key := constants.DBVersionPrefix + constants.AiServicePrefix + requiredDbVersion
	resp, _ := client.GetSettings(ctx, []string{key})
	return resp.Settings == nil || resp.Settings[key] == ""
}

func migrate(ctx context.Context, client *ent.Client, userClient rpc.UserClient, requiredDbVersion string, l *log.Helper) error {
	l.Info("Start initializing database schema...")
	l.Info("Creating basic table schema...")
	if err := client.Schema.Create(ctx); err != nil {
		return fmt.Errorf("failed creating schema resources: %w", err)
	}

	key := constants.DBVersionPrefix + constants.AiServicePrefix + requiredDbVersion
	_, err := userClient.SetSettings(ctx, map[string]any{
		key: "installed",
	})
	return err
}
