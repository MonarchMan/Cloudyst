package thumb

import (
	"common/util"
	"context"
	"errors"
	"file/internal/biz/filemanager/manager/entitysource"
	"file/internal/biz/setting"
	"file/internal/data/types"
	"fmt"
	"io"
	"reflect"
	"sort"

	"github.com/go-kratos/kratos/v2/log"
)

type (
	// Generator generates a thumbnail for a given reader.
	Generator interface {
		// Generate generates a thumbnail for a given reader. Src is the original files path, only provided
		// for local policy files. State is the result from previous generators, and can be read by current
		// generator for intermedia result.
		Generate(ctx context.Context, es entitysource.EntitySource, ext string, previous *Result) (*Result, error)

		// Priority of execution order, smaller value means higher priority.
		Priority() int

		// Enabled returns if current generator is enabled.
		Enabled(ctx context.Context) bool
	}
	Result struct {
		Path     string
		Ext      string
		Continue bool
		Cleanup  []func()
	}
	GeneratorType string

	generatorList []Generator
	pipeline      struct {
		generators generatorList
		settings   setting.Provider
		l          *log.Helper
	}
)

var (
	ErrPassThrough  = errors.New("pass through")
	ErrNotAvailable = fmt.Errorf("thumbnail not available: %w", ErrPassThrough)
)

func (g generatorList) Len() int {
	return len(g)
}

func (g generatorList) Less(i, j int) bool {
	return g[i].Priority() < g[j].Priority()
}

func (g generatorList) Swap(i, j int) {
	g[i], g[j] = g[j], g[i]
}

// NewPipeline creates a new pipeline with all available generators.
func NewPipeline(settings setting.Provider, l log.Logger) Generator {
	generators := generatorList{}
	generators = append(
		generators,
		NewBuiltinGenerator(settings),
		NewFfmpegGenerator(l, settings),
		NewVipsGenerator(l, settings),
		NewLibreOfficeGenerator(l, settings),
		NewMusicCoverGenerator(l, settings),
		NewLibRawGenerator(l, settings),
	)
	sort.Sort(generators)

	return pipeline{
		generators: generators,
		settings:   settings,
		l:          log.NewHelper(l, log.WithMessageKey("biz-thumb")),
	}
}

func (p pipeline) Generate(ctx context.Context, es entitysource.EntitySource, ext string, state *Result) (*Result, error) {
	e := es.Entity()
	for _, generator := range p.generators {
		if generator.Enabled(ctx) {
			if _, err := es.Seek(0, io.SeekStart); err != nil {
				return nil, fmt.Errorf("thumb: failed to seek to start of files: %w", err)
			}

			res, err := generator.Generate(ctx, es, ext, state)
			if errors.Is(err, ErrPassThrough) {
				p.l.WithContext(ctx).Debugf("Failed to generate thumbnail using %s for %s: %s, passing through to next generator.", reflect.TypeOf(generator).String(), e.Source(), err)
				continue
			}

			if res != nil && res.Continue {
				p.l.WithContext(ctx).Debugf("Generator %s for %s returned continue, passing through to next generator.", reflect.TypeOf(generator).String(), e.Source())

				// defer cleanup functions
				for _, cleanup := range res.Cleanup {
					defer cleanup()
				}

				// prepare files reader for next generator
				state = res
				es, err = es.CloneToLocalSrc(types.EntityTypeVersion, res.Path)
				if err != nil {
					return nil, fmt.Errorf("thumb: failed to clone to local source: %w", err)
				}

				defer es.Close()
				ext = util.Ext(res.Path)
				continue
			}

			return res, err
		}
	}
	return nil, ErrNotAvailable
}

func (p pipeline) Priority() int {
	return 0
}

func (p pipeline) Enabled(ctx context.Context) bool {
	return true
}
