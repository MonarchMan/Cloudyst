package dbfs

import (
	filepb "api/api/file/files/v1"
	"common/serializer"
	"context"
	"file/internal/biz/filemanager/fs"
	"file/internal/data"
	"file/internal/data/types"
	"fmt"
	"time"

	"github.com/samber/lo"
)

func (f *DBFS) PatchProps(ctx context.Context, uri *fs.URI, props *types.FileProps, delete bool) error {
	navigator, err := f.getNavigator(ctx, uri, NavigatorCapabilityModifyProps, NavigatorCapabilityLockFile)
	if err != nil {
		return err
	}

	target, err := f.getFileByPath(ctx, navigator, uri)
	if err != nil {
		return fmt.Errorf("failed to get target files: %w", err)
	}

	if target.OwnerID() != int(f.user.Id) {
		return fs.ErrOwnerOnly.WithCause(fmt.Errorf("only files ownerId can modify files props"))
	}

	// Lock target
	lr := &LockByPath{target.Uri(true), target, target.Type(), ""}
	ls, err := f.acquireByPath(ctx, -1, int(f.user.Id), true, fs.LockApp(fs.ApplicationUpdateMetadata), lr)
	defer func() { _ = f.Release(ctx, ls) }()
	if err != nil {
		return err
	}

	currentProps := target.Model.Props
	if currentProps == nil {
		currentProps = &types.FileProps{}
	}

	if props.View != nil {
		if delete {
			currentProps.View = nil
		} else {
			currentProps.View = props.View
		}
	}

	if _, err := f.fileClient.UpdateProps(ctx, target.Model, currentProps); err != nil {
		return serializer.NewError(serializer.CodeDBError, "failed to update files props", err)
	}

	return nil
}

func (f *DBFS) PatchMetadata(ctx context.Context, path []*fs.URI, metas ...*filepb.MetadataPatch) error {
	ae := serializer.NewAggregateError()
	targets := make([]*File, 0, len(path))
	for _, p := range path {
		navigator, err := f.getNavigator(ctx, p, NavigatorCapabilityUpdateMetadata, NavigatorCapabilityLockFile)
		if err != nil {
			ae.Add(p.String(), err)
			continue
		}

		target, err := f.getFileByPath(ctx, navigator, p)
		if err != nil {
			ae.Add(p.String(), fmt.Errorf("failed to get target files: %w", err))
			continue
		}

		// Require Update permission
		if _, ok := ctx.Value(ByPassOwnerCheckCtxKey{}).(bool); !ok && target.OwnerID() != int(f.user.Id) {
			return fs.ErrOwnerOnly.WithCause(fmt.Errorf("permission denied"))
		}

		if target.IsRootFolder() {
			ae.Add(p.String(), fs.ErrNotSupportedAction.WithCause(fmt.Errorf("cannot move root folder")))
			continue
		}

		targets = append(targets, target)
	}

	if len(targets) == 0 {
		return ae.Aggregate()
	}

	// Lock all targets
	lockTargets := lo.Map(targets, func(value *File, key int) *LockByPath {
		return &LockByPath{value.Uri(true), value, value.Type(), ""}
	})
	ls, err := f.acquireByPath(ctx, -1, int(f.user.Id), true, fs.LockApp(fs.ApplicationUpdateMetadata), lockTargets...)
	defer func() { _ = f.Release(ctx, ls) }()
	if err != nil {
		return err
	}

	metadataMap := make(map[string]string)
	privateMap := make(map[string]bool)
	deleted := make([]string, 0)
	updateModifiedAt := false
	for _, meta := range metas {
		if meta.Remove {
			deleted = append(deleted, meta.Key)
			continue
		}
		metadataMap[meta.Key] = meta.Value
		if meta.Private {
			privateMap[meta.Key] = meta.Private
		}
		if meta.UpdateModifiedAt {
			updateModifiedAt = true
		}
	}

	fc, tx, ctx, err := data.WithTx(ctx, f.fileClient)
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to start transaction", err)
	}

	for _, target := range targets {
		if err := fc.UpsertMetadata(ctx, target.Model, metadataMap, privateMap); err != nil {
			_ = data.Rollback(tx)
			return fmt.Errorf("failed to upsert metadata: %w", err)
		}

		if len(deleted) > 0 {
			if err := fc.RemoveMetadata(ctx, target.Model, deleted...); err != nil {
				_ = data.Rollback(tx)
				return fmt.Errorf("failed to remove metadata: %w", err)
			}
		}

		if updateModifiedAt {
			if err := fc.UpdateModifiedAt(ctx, target.Model, time.Now()); err != nil {
				_ = data.Rollback(tx)
				return fmt.Errorf("failed to update files modified at: %w", err)
			}
		}
	}

	if err := data.Commit(tx); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to commit metadata change", err)
	}

	return ae.Aggregate()
}
