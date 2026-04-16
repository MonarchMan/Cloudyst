package email

import (
	"common"
	"context"
	"errors"
	"sync/atomic"
	"user/internal/biz/setting"

	"github.com/go-kratos/kratos/v2/log"
)

type (
	// Driver 邮件发送驱动
	Driver interface {
		// Close 关闭驱动
		Close()
		// Send 发送邮件
		Send(ctx context.Context, to, title, body string) error
	}

	DriverManager interface {
		common.Reloadable
		Init()
		Driver() Driver
	}

	manager struct {
		driver   atomic.Value
		settings setting.Provider
		logger   log.Logger
	}
)

var (
	// ErrChanNotOpen 邮件队列未开启
	ErrChanNotOpen = errors.New("email queue is not started")
	// ErrNoActiveDriver 无可用邮件发送服务
	ErrNoActiveDriver = errors.New("no avaliable email provider")
)

func NewEmailManager(settings setting.Provider, logger log.Logger) (DriverManager, func()) {
	m := &manager{
		settings: settings,
		logger:   logger,
	}
	cleanup := func() {
		if m.Driver() != nil {
			m.Driver().Close()
		}
		logger.Log(log.LevelInfo, "emailManager", "email driver closed")
	}
	return m, cleanup
}

func (m *manager) Init() {
	emailClient := NewSMTPPool(m.settings, m.logger)
	m.driver.Store(emailClient)
}

func (m *manager) Driver() Driver {
	if v, ok := m.driver.Load().(Driver); ok {
		return v
	}
	return nil
}

func (m *manager) Reload(ctx context.Context) error {
	if d := m.Driver(); d != nil {
		d.Close()
	}
	emailClient := NewSMTPPool(m.settings, m.logger)
	m.driver.Store(emailClient)
	return nil
}
