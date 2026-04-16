package crontab

import (
	userpb "api/api/user/common/v1"
	pbuser "api/api/user/users/v1"
	"api/external/trans"
	"common/logging"
	"context"
	"file/internal/biz/filemanager"
	"file/internal/biz/queue"
	"file/internal/biz/setting"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/emptypb"
)

type (
	CronTaskFunc     func(ctx context.Context, q queue.Queue)
	cornRegistration struct {
		t      setting.CronType
		config string
		fn     CronTaskFunc
	}
)

var (
	registrations []cornRegistration
)

// Register registers a cron task.
func Register(t setting.CronType, fn CronTaskFunc) {
	registrations = append(registrations, cornRegistration{
		t:  t,
		fn: fn,
	})
}

// NewCron constructs a new cron instance with given dependency.
func NewCron(ctx context.Context, settings setting.Provider, uc pbuser.UserClient, l *log.Helper, q queue.Queue,
	tracer trace.Tracer, dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep) (*cron.Cron, error) {
	anonymous, err := uc.GetAnonymousUser(context.Background(), &emptypb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("cron: faield to get anonymous users: %w", err)
	}

	l.WithContext(ctx).Infof("Initialize crontab jobs...")
	c := cron.New()

	for _, r := range registrations {
		cronConfig := settings.Cron(ctx, r.t)
		if _, err := c.AddFunc(cronConfig, taskWrapper(string(r.t), cronConfig, anonymous, l, r.fn, q, tracer, dep, dbfsDep)); err != nil {
			l.WithContext(ctx).Warnf("Failed to start crontab job %q: %s", cronConfig, err)
		}
	}

	return c, nil
}

func taskWrapper(name, config string, user *userpb.User, l *log.Helper, task CronTaskFunc, q queue.Queue,
	tracer trace.Tracer, dep filemanager.ManagerDep, dbfsDep filemanager.DbfsDep) func() {
	l.Infof("Cron task %s started with config %q", name, config)
	return func() {
		//cid := uuid.Must(uuid.NewV4())
		ctx := context.Background()
		l := log.NewHelper(log.With(l.Logger(), "Cron", name))
		//ctx = context.WithValue(ctx, logging.CorrelationIDCtx{}, cid)
		ctx, span := tracer.Start(ctx, fmt.Sprintf("cron-%s", name))
		defer span.End()
		l.WithContext(ctx).Infof("Executing Cron task %q", name)

		ctx = context.WithValue(ctx, logging.LoggerCtx{}, l)
		ctx = context.WithValue(ctx, trans.UserCtx{}, user)
		ctx = context.WithValue(ctx, filemanager.ManagerDepCtx{}, dep)
		ctx = context.WithValue(ctx, filemanager.DbfsDepCtx{}, dbfsDep)
		task(ctx, q)
	}
}
