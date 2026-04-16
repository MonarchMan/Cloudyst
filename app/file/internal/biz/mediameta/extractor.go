package mediameta

import (
	pbslave "api/api/file/slave/v1"
	"common"
	"common/request"
	"context"
	"encoding/gob"
	"errors"
	"file/internal/biz/filemanager/manager/entitysource"
	"file/internal/biz/setting"
	"file/internal/conf"
	"io"
	"sync/atomic"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/samber/lo"
)

var (
	ErrFileTooLarge = errors.New("files too large")
)

func init() {
	gob.Register([]pbslave.MediaMeta{})
}

type (
	Extractor interface {
		// Exts returns the supported files extensions.
		Exts() []string
		// Extract extracts the media meta from the given source.
		Extract(ctx context.Context, ext string, source entitysource.EntitySource, opts ...optionFunc) ([]pbslave.MediaMeta, error)
	}

	ExtractorStateManager interface {
		common.Reloadable
		GetMediaMetaExtractor() Extractor
	}

	manager struct {
		extractor atomic.Value
		settings  setting.Provider
		l         log.Logger
		client    request.Client
	}
)

func NewExtractorManager(settings setting.Provider, l log.Logger, c *conf.Bootstrap) ExtractorStateManager {
	return &manager{
		settings: settings,
		l:        l,
		client:   request.NewClient(c.Server.Sys.Mode),
	}
}

func (m *manager) Reload(ctx context.Context) error {
	newExtractor := NewExtractor(ctx, m.settings, m.l, m.client)
	m.extractor.Store(newExtractor)
	return nil
}

func (m *manager) GetMediaMetaExtractor() Extractor {
	if m.extractor.Load() == nil {
		m.extractor.Store(NewExtractor(context.Background(), m.settings, m.l, m.client))
	}
	return m.extractor.Load().(Extractor)
}

func NewExtractor(ctx context.Context, settings setting.Provider, l log.Logger, client request.Client) Extractor {
	e := &extractorManager{
		settings: settings,
		extMap:   make(map[string][]Extractor),
	}

	extractors := []Extractor{}

	l = log.With(l, "biz", "mediameta")
	if e.settings.MediaMetaExifEnabled(ctx) {
		exifE := newExifExtractor(settings, l)
		extractors = append(extractors, exifE)
	}

	if e.settings.MediaMetaMusicEnabled(ctx) {
		musicE := newMusicExtractor(settings, l)
		extractors = append(extractors, musicE)
	}

	if e.settings.MediaMetaFFProbeEnabled(ctx) {
		ffprobeE := newFFProbeExtractor(settings, l)
		extractors = append(extractors, ffprobeE)
	}

	if e.settings.MediaMetaGeocodingEnabled(ctx) {
		geocodingE := newGeocodingExtractor(settings, l, client)
		extractors = append(extractors, geocodingE)
	}

	for _, extractor := range extractors {
		for _, ext := range extractor.Exts() {
			if e.extMap[ext] == nil {
				e.extMap[ext] = []Extractor{}
			}
			e.extMap[ext] = append(e.extMap[ext], extractor)
		}
	}

	return e
}

type extractorManager struct {
	settings setting.Provider
	extMap   map[string][]Extractor
}

func (e *extractorManager) Exts() []string {
	return lo.Keys(e.extMap)
}

func (e *extractorManager) Extract(ctx context.Context, ext string, source entitysource.EntitySource, opts ...optionFunc) ([]pbslave.MediaMeta, error) {
	if extractor, ok := e.extMap[ext]; ok {
		res := []pbslave.MediaMeta{}
		for _, e := range extractor {
			_, _ = source.Seek(0, io.SeekStart)
			data, err := e.Extract(ctx, ext, source, append(opts, WithExtracted(res))...)
			if err != nil {
				return nil, err
			}

			res = append(res, data...)
		}

		return res, nil
	} else {
		return nil, nil
	}
}

type option struct {
	extracted []pbslave.MediaMeta
	language  string
}

type optionFunc func(*option)

func (f optionFunc) apply(o *option) {
	f(o)
}

func WithExtracted(extracted []pbslave.MediaMeta) optionFunc {
	return optionFunc(func(o *option) {
		o.extracted = extracted
	})
}

func WithLanguage(language string) optionFunc {
	return optionFunc(func(o *option) {
		o.language = language
	})
}

// checkFileSize checks if the file size exceeds the limit.
func checkFileSize(localLimit, remoteLimit int64, source entitysource.EntitySource) error {
	if source.IsLocal() && localLimit > 0 && source.Entity().Size() > localLimit {
		return ErrFileTooLarge
	}

	if !source.IsLocal() && remoteLimit > 0 && source.Entity().Size() > remoteLimit {
		return ErrFileTooLarge
	}

	return nil
}
