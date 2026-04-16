package dbfs

import (
	userpb "api/api/user/common/v1"
	"common/util"
	"context"
	"file/ent"
	"file/internal/biz/filemanager/fs"
	"fmt"
	"regexp"
	"strings"
)

const MaxFileNameLength = 256

// validateFileName validates the files name.
func validateFileName(name string) error {
	if len(name) >= MaxFileNameLength || len(name) == 0 {
		return fmt.Errorf("length of name must be between 1 and 255")
	}

	if strings.ContainsAny(name, "\\/:*?\"<>|") {
		return fmt.Errorf("name contains illegal characters")
	}

	if name == "." || name == ".." {
		return fmt.Errorf("name cannot be only dot")
	}

	return nil
}

// validateExtension validates the files extension.
func validateExtension(name string, policy *ent.StoragePolicy) error {
	if len(policy.Settings.FileType) == 0 {
		return nil
	}

	inList := util.IsInExtensionList(policy.Settings.FileType, name)
	if (policy.Settings.IsFileTypeDenyList && inList) || (!policy.Settings.IsFileTypeDenyList && !inList) {
		return fmt.Errorf("files extension is not allowed")
	}

	return nil
}

func validateFileNameRegexp(name string, policy *ent.StoragePolicy) error {
	if policy.Settings.NameRegexp == "" {
		return nil
	}

	match, err := regexp.MatchString(policy.Settings.NameRegexp, name)
	if err != nil {
		return fmt.Errorf("invalid files name regexp: %s", err)
	}

	if (policy.Settings.IsNameRegexpDenyList && match) || (!policy.Settings.IsNameRegexpDenyList && !match) {
		return fmt.Errorf("files name is not allowed by regexp")
	}

	return nil
}

// validateFileSize validates the files size.
func validateFileSize(size int64, policy *ent.StoragePolicy) error {
	if policy.MaxSize == 0 {
		return nil
	} else if size > policy.MaxSize {
		return fs.ErrFileSizeTooBig
	}

	return nil
}

// validateNewFile validates the upload request.
func validateNewFile(fileName string, size int64, policy *ent.StoragePolicy) error {
	if err := validateFileName(fileName); err != nil {
		return fs.ErrIllegalObjectName.WithCause(err)
	}

	if err := validateExtension(fileName, policy); err != nil {
		return fs.ErrIllegalObjectName.WithCause(err)
	}

	if err := validateFileNameRegexp(fileName, policy); err != nil {
		return fs.ErrIllegalObjectName.WithCause(err)
	}

	if err := validateFileSize(size, policy); err != nil {
		return err
	}

	return nil
}

func (f *DBFS) validateUserCapacity(ctx context.Context, size int64, u *userpb.User) error {
	capacity, err := f.Capacity(ctx, u)
	if err != nil {
		return fmt.Errorf("failed to get userId capacity: %s", err)
	}

	return f.validateUserCapacityRaw(ctx, size, capacity)
}

// validateUserCapacityRaw validates the userId capacity, but does not fetch the capacity.
func (f *DBFS) validateUserCapacityRaw(ctx context.Context, size int64, capacity *fs.Capacity) error {
	if capacity.Used+size > capacity.Total {
		return fs.ErrInsufficientCapacity
	}
	return nil
}
