package thumb

import (
	"bytes"
	"common/util"
	"context"
	"file/internal/biz/filemanager/manager/entitysource"
	"file/internal/biz/setting"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/gofrs/uuid"
)

func NewLibreOfficeGenerator(l log.Logger, settings setting.Provider) *LibreOfficeGenerator {
	return &LibreOfficeGenerator{l: log.NewHelper(l, log.WithMessageKey("biz-thumb")), settings: settings}
}

type LibreOfficeGenerator struct {
	settings setting.Provider
	l        *log.Helper
}

func (l *LibreOfficeGenerator) Generate(ctx context.Context, es entitysource.EntitySource, ext string, previous *Result) (*Result, error) {
	if !util.IsInExtensionListExt(l.settings.LibreOfficeThumbExts(ctx), ext) {
		return nil, fmt.Errorf("unsupported video format: %w", ErrPassThrough)
	}

	if es.Entity().Size() > l.settings.LibreOfficeThumbMaxSize(ctx) {
		return nil, fmt.Errorf("files is too big: %w", ErrPassThrough)
	}

	tempOutputPath := filepath.Join(
		util.DataPath(l.settings.TempPath(ctx)),
		thumbTempFolder,
		fmt.Sprintf("soffice_%s", uuid.Must(uuid.NewV4()).String()),
	)

	tempInputPath := ""
	if es.IsLocal() {
		tempInputPath = es.LocalPath(ctx)
	} else {
		// If not local policy files, download to temp folder
		tempInputPath = filepath.Join(
			util.DataPath(l.settings.TempPath(ctx)),
			"thumb",
			fmt.Sprintf("soffice_%s.%s", uuid.Must(uuid.NewV4()).String(), ext),
		)

		// Due to limitations of ffmpeg, we need to write the input files to disk first
		tempInputFile, err := util.CreateNestedFile(tempInputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create temp files: %w", err)
		}

		defer os.Remove(tempInputPath)
		defer tempInputFile.Close()

		if _, err = io.Copy(tempInputFile, es); err != nil {
			return &Result{Path: tempOutputPath}, fmt.Errorf("failed to write input files: %w", err)
		}

		tempInputFile.Close()
	}

	// Convert the document to an image
	cmd := exec.CommandContext(ctx, l.settings.LibreOfficePath(ctx), "--headless",
		"--nologo", "--nofirststartwizard", "--invisible", "--norestore", "--convert-to",
		"png", "--outdir", tempOutputPath, tempInputPath)

	// Redirect IO
	var stdErr bytes.Buffer
	cmd.Stderr = &stdErr

	if err := cmd.Run(); err != nil {
		l.l.WithContext(ctx).Warnf("Failed to invoke LibreOffice: %s", stdErr.String())
		return &Result{Path: tempOutputPath}, fmt.Errorf("failed to invoke LibreOffice: %w, raw output: %s", err, stdErr.String())
	}

	return &Result{
		Path: filepath.Join(
			tempOutputPath,
			strings.TrimSuffix(filepath.Base(tempInputPath), filepath.Ext(tempInputPath))+".png",
		),
		Continue: true,
		Cleanup:  []func(){func() { _ = os.RemoveAll(tempOutputPath) }},
	}, nil
}

func (l *LibreOfficeGenerator) Priority() int {
	return 50
}

func (l *LibreOfficeGenerator) Enabled(ctx context.Context) bool {
	return l.settings.LibreOfficeThumbGeneratorEnabled(ctx)
}
