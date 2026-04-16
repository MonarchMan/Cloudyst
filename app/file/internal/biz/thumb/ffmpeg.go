package thumb

import (
	"bytes"
	"common/util"
	"context"
	"file/internal/biz/filemanager/driver"
	"file/internal/biz/filemanager/manager/entitysource"
	"file/internal/biz/setting"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/gofrs/uuid"
)

const (
	urlTimeout = time.Duration(1) * time.Hour
)

func NewFfmpegGenerator(l log.Logger, settings setting.Provider) *FfmpegGenerator {
	return &FfmpegGenerator{l: log.NewHelper(l, log.WithMessageKey("biz-thumb")), settings: settings}
}

type FfmpegGenerator struct {
	l        *log.Helper
	settings setting.Provider
}

func (f *FfmpegGenerator) Generate(ctx context.Context, es entitysource.EntitySource, ext string, previous *Result) (*Result, error) {
	if !util.IsInExtensionListExt(f.settings.FFMpegThumbExts(ctx), ext) {
		return nil, fmt.Errorf("unsupported video format: %w", ErrPassThrough)
	}

	if es.Entity().Size() > f.settings.FFMpegThumbMaxSize(ctx) {
		return nil, fmt.Errorf("files is too big: %w", ErrPassThrough)
	}

	tempOutputPath := filepath.Join(
		util.DataPath(f.settings.TempPath(ctx)),
		thumbTempFolder,
		fmt.Sprintf("thumb_%s.png", uuid.Must(uuid.NewV4()).String()),
	)

	if err := util.CreateNestedFolder(filepath.Dir(tempOutputPath)); err != nil {
		return nil, fmt.Errorf("failed to create temp folder: %w", err)
	}

	input := ""
	expire := time.Now().Add(urlTimeout)
	if es.IsLocal() {
		input = es.LocalPath(ctx)
	} else {
		src, err := es.Url(driver.WithForcePublicEndpoint(ctx, false), entitysource.WithNoInternalProxy(), entitysource.WithContext(ctx), entitysource.WithExpire(&expire))
		if err != nil {
			return &Result{Path: tempOutputPath}, fmt.Errorf("failed to get entity url: %w", err)
		}

		input = src.Url
	}

	// Invoke ffmpeg
	w, h := f.settings.ThumbSize(ctx)
	scaleOpt := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", w, h)
	args := []string{
		"-ss", f.settings.FFMpegThumbSeek(ctx),
	}

	extraArgs := f.settings.FFMpegExtraArgs(ctx)
	if extraArgs != "" {
		args = append(args, strings.Split(extraArgs, " ")...)
	}

	args = append(args, []string{
		"-i", input,
		"-vf", scaleOpt,
		"-vframes", "1",
		tempOutputPath,
	}...)
	cmd := exec.CommandContext(ctx, f.settings.FFMpegPath(ctx), args...)

	// Redirect IO
	var stdErr bytes.Buffer
	cmd.Stderr = &stdErr

	if err := cmd.Run(); err != nil {
		f.l.WithContext(ctx).Warnf("Failed to invoke ffmpeg: %s", stdErr.String())
		return &Result{Path: tempOutputPath}, fmt.Errorf("failed to invoke ffmpeg: %w, raw output: %s", err, stdErr.String())
	}

	return &Result{Path: tempOutputPath}, nil
}

func (f *FfmpegGenerator) Priority() int {
	return 200
}

func (f *FfmpegGenerator) Enabled(ctx context.Context) bool {
	return f.settings.FFMpegThumbGeneratorEnabled(ctx)
}
