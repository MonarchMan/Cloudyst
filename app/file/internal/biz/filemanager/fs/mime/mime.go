package mime

import (
	"common"
	"context"
	"encoding/json"
	"file/internal/biz/setting"
	"mime"
	"path"
	"sync/atomic"

	"github.com/go-kratos/kratos/v2/log"
)

type (
	MimeDetector interface {
		// TypeByName returns the mime type by files name.
		TypeByName(ext string) string
	}
	MimeManager interface {
		common.Reloadable
		MimeDetector() MimeDetector
	}
	manager struct {
		detector atomic.Value
		settings setting.Provider
		l        *log.Helper
	}
)

func NewMimeManager(settings setting.Provider, l log.Logger) MimeManager {
	return &manager{
		settings: settings,
		l:        log.NewHelper(l, log.WithMessageKey("biz-fileManager")),
	}
}

func (m *manager) Reload(ctx context.Context) error {
	newDetector := NewMimeDetector(ctx, m.settings, m.l)
	m.detector.Store(newDetector)
	return nil
}

func (m *manager) MimeDetector() MimeDetector {
	if m.detector.Load() == nil {
		m.detector.Store(NewMimeDetector(context.Background(), m.settings, m.l))
	}
	return m.detector.Load().(MimeDetector)
}

type mimeDetector struct {
	mapping map[string]string
}

func NewMimeDetector(ctx context.Context, settings setting.Provider, l *log.Helper) MimeDetector {
	mappingStr := settings.MimeMapping(ctx)
	mapping := make(map[string]string)
	if err := json.Unmarshal([]byte(mappingStr), &mapping); err != nil {
		l.Errorf("Failed to unmarshal mime mapping: %s, fallback to empty mapping", err)
	}

	return &mimeDetector{
		mapping: mapping,
	}
}

func (d *mimeDetector) TypeByName(p string) string {
	ext := path.Ext(p)
	if m, ok := d.mapping[ext]; ok {
		return m
	}

	m := mime.TypeByExtension(ext)
	if m != "" {
		return m
	}

	// Fallback
	return "application/octet-stream"
}
