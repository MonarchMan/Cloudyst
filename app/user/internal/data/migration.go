package data

import (
	pb "api/api/user/common/v1"
	ftypes "api/external/data/file"
	"common/boolset"
	"common/cache"
	"common/constants"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"user/ent"
	"user/ent/group"
	"user/ent/setting"
	"user/internal/data/types"

	"github.com/Masterminds/semver/v3"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/samber/lo"
)

// needMigration exams if required schema version is satisfied.
func needMigration(client *ent.Client, ctx context.Context, requiredDbVersion string) bool {
	c, _ := client.Setting.Query().Where(setting.NameEQ(DBVersionPrefix + requiredDbVersion)).Count(ctx)
	return c == 0
}

func migrate(l *log.Helper, client *ent.Client, ctx context.Context, kv cache.Driver, requiredDbVersion string) error {
	l.Info("Start initializing database schema...")
	l.Info("Creating basic table schema...")
	if err := client.Schema.Create(ctx); err != nil {
		return fmt.Errorf("Failed creating schema resources: %w", err)
	}

	migrateDefaultSettings(l, client, ctx, kv)

	if err := migrateSysGroups(l, client, ctx); err != nil {
		return fmt.Errorf("failed migrating default storage policy: %w", err)
	}

	if err := applyPatches(l, client, ctx, requiredDbVersion); err != nil {
		return fmt.Errorf("failed applying schema patches: %w", err)
	}

	client.Setting.Create().SetName(DBVersionPrefix + requiredDbVersion).SetValue("installed").Save(ctx)
	return nil
}

func migrateDefaultSettings(l *log.Helper, client *ent.Client, ctx context.Context, kv cache.Driver) {
	// clean kv cache
	if err := kv.DeleteAll(); err != nil {
		l.Warn("Failed to remove all KV entries while schema migration: %s", err)
	}

	// List existing settings into a map
	existingSettings := make(map[string]struct{})
	settings, err := client.Setting.Query().All(ctx)
	if err != nil {
		l.Warn("Failed to query existing settings: %s", err)
	}

	for _, s := range settings {
		existingSettings[s.Name] = struct{}{}
	}

	l.Info("Insert default settings...")
	for k, v := range DefaultSettings {
		if _, ok := existingSettings[k]; ok {
			l.Debugf("Skip inserting setting %s, already exists.", k)
			continue
		}

		if override, ok := os.LookupEnv(EnvDefaultOverwritePrefix + k); ok {
			l.Infof("Override default setting %q with env value %q", k, override)
			v = override
		}

		client.Setting.Create().SetName(k).SetValue(v).SaveX(ctx)
	}
}

func migrateSysGroups(l *log.Helper, client *ent.Client, ctx context.Context) error {
	if err := migrateAdminGroup(l, client, ctx); err != nil {
		return err
	}

	if err := migrateUserGroup(l, client, ctx); err != nil {
		return err
	}

	if err := migrateAnonymousGroup(l, client, ctx); err != nil {
		return err
	}

	return nil
}

func migrateAdminGroup(l *log.Helper, client *ent.Client, ctx context.Context) error {
	if _, err := client.Group.Query().Where(group.ID(1)).First(ctx); err == nil {
		l.Info("Default admin group (ID=1) already exists, skip migrating.")
		return nil
	}

	l.Info("Insert default admin group...")
	permissions := &boolset.BooleanSet{}
	boolset.Sets(map[int]bool{
		types.GroupPermissionIsAdmin:             true,
		types.GroupPermissionShare:               true,
		types.GroupPermissionWebDAV:              true,
		types.GroupPermissionWebDAVProxy:         true,
		types.GroupPermissionArchiveDownload:     true,
		types.GroupPermissionArchiveTask:         true,
		types.GroupPermissionShareDownload:       true,
		types.GroupPermissionRemoteDownload:      true,
		types.GroupPermissionRedirectedSource:    true,
		types.GroupPermissionAdvanceDelete:       true,
		types.GroupPermissionIgnoreFileOwnership: true,
		// TODO: review default permission
	}, permissions)
	if _, err := client.Group.Create().
		SetName("Admin").
		SetStoragePolicyID(1).
		SetMaxStorage(1 * constants.TB). // 1 TB default storage
		SetPermissions(permissions).
		SetStoragePolicyInfo(&pb.StoragePolicyInfo{
			Id:        1,
			Name:      "Default",
			Type:      "Default",
			IsPrivate: false,
		}).
		SetSettings(&pb.GroupSetting{
			SourceBatchSize:  1000,
			Aria2BatchSize:   50,
			MaxWalkedFiles:   100000,
			TrashRetention:   7 * 24 * 3600,
			RedirectedSource: true,
		}).
		Save(ctx); err != nil {
		return fmt.Errorf("failed to create default admin group: %w", err)
	}

	return nil
}

func migrateUserGroup(l *log.Helper, client *ent.Client, ctx context.Context) error {
	if _, err := client.Group.Query().Where(group.ID(2)).First(ctx); err == nil {
		l.Info("Default users group (ID=2) already exists, skip migrating.")
		return nil
	}

	l.Info("Insert default users group...")
	permissions := &boolset.BooleanSet{}
	boolset.Sets(map[int]bool{
		types.GroupPermissionShare:            true,
		types.GroupPermissionShareDownload:    true,
		types.GroupPermissionRedirectedSource: true,
	}, permissions)
	if _, err := client.Group.Create().
		SetName("User").
		SetStoragePolicyID(1).
		SetMaxStorage(1 * constants.GB). // 1 GB default storage
		SetPermissions(permissions).
		SetStoragePolicyInfo(&pb.StoragePolicyInfo{
			Id:        1,
			Name:      "Default",
			Type:      "Default",
			IsPrivate: false,
		}).
		SetSettings(&pb.GroupSetting{
			SourceBatchSize:  10,
			Aria2BatchSize:   1,
			MaxWalkedFiles:   100000,
			TrashRetention:   7 * 24 * 3600,
			RedirectedSource: true,
		}).
		Save(ctx); err != nil {
		return fmt.Errorf("failed to create default users group: %w", err)
	}

	return nil
}

func migrateAnonymousGroup(l *log.Helper, client *ent.Client, ctx context.Context) error {
	if _, err := client.Group.Query().Where(group.ID(AnonymousGroupID)).First(ctx); err == nil {
		l.Info("Default anonymous group (ID=3) already exists, skip migrating.")
		return nil
	}

	l.Info("Insert default anonymous group...")
	permissions := &boolset.BooleanSet{}
	boolset.Sets(map[int]bool{
		types.GroupPermissionIsAnonymous:   true,
		types.GroupPermissionShareDownload: true,
	}, permissions)
	if _, err := client.Group.Create().
		SetName("Anonymous").
		SetPermissions(permissions).
		SetStoragePolicyInfo(nil).
		SetSettings(&pb.GroupSetting{
			MaxWalkedFiles:   100000,
			RedirectedSource: true,
		}).
		Save(ctx); err != nil {
		return fmt.Errorf("failed to create default anonymous group: %w", err)
	}

	return nil
}

type (
	PatchFunc func(l *log.Helper, client *ent.Client, ctx context.Context) error
	Patch     struct {
		Name       string
		EndVersion string
		Func       PatchFunc
	}
)

var patches = []Patch{
	{
		Name:       "apply_default_archive_viewer",
		EndVersion: "4.7.0",
		Func: func(l *log.Helper, client *ent.Client, ctx context.Context) error {
			fileViewersSetting, err := client.Setting.Query().Where(setting.Name("file_viewers")).First(ctx)
			if err != nil {
				return fmt.Errorf("failed to query file_viewers setting: %w", err)
			}

			var fileViewers []ftypes.ViewerGroup
			if err := json.Unmarshal([]byte(fileViewersSetting.Value), &fileViewers); err != nil {
				return fmt.Errorf("failed to unmarshal file_viewers setting: %w", err)
			}

			fileViewerExisted := false
			for _, viewer := range fileViewers[0].Viewers {
				if viewer.ID == "archive" {
					fileViewerExisted = true
					break
				}
			}

			// 2.2 If not existed, add it
			if !fileViewerExisted {
				// Found existing archive viewer default setting
				var defaultArchiveViewer ftypes.Viewer
				for _, viewer := range defaultFileViewers[0].Viewers {
					if viewer.ID == "archive" {
						defaultArchiveViewer = viewer
						break
					}
				}

				fileViewers[0].Viewers = append(fileViewers[0].Viewers, defaultArchiveViewer)
				newFileViewersSetting, err := json.Marshal(fileViewers)
				if err != nil {
					return fmt.Errorf("failed to marshal file_viewers setting: %w", err)
				}

				if _, err := client.Setting.UpdateOne(fileViewersSetting).SetValue(string(newFileViewersSetting)).Save(ctx); err != nil {
					return fmt.Errorf("failed to update file_viewers setting: %w", err)
				}
			}

			return nil
		},
	},
	{
		Name:       "apply_default_excalidraw_viewer",
		EndVersion: "4.1.0",
		Func: func(l *log.Helper, client *ent.Client, ctx context.Context) error {
			// 1. Apply excalidraw file icons
			// 1.1 Check if it's already applied
			iconSetting, err := client.Setting.Query().Where(setting.Name("explorer_icons")).First(ctx)
			if err != nil {
				return fmt.Errorf("failed to query explorer_icons setting: %w", err)
			}

			var icons []types.FileTypeIconSetting
			if err := json.Unmarshal([]byte(iconSetting.Value), &icons); err != nil {
				return fmt.Errorf("failed to unmarshal explorer_icons setting: %w", err)
			}

			iconExisted := false
			for _, icon := range icons {
				if lo.Contains(icon.Exts, "excalidraw") {
					iconExisted = true
					break
				}
			}

			// 1.2 If not existed, add it
			if !iconExisted {
				// Found existing excalidraw icon default setting
				var defaultExcalidrawIcon types.FileTypeIconSetting
				for _, icon := range defaultIcons {
					if lo.Contains(icon.Exts, "excalidraw") {
						defaultExcalidrawIcon = icon
						break
					}
				}

				icons = append(icons, defaultExcalidrawIcon)
				newIconSetting, err := json.Marshal(icons)
				if err != nil {
					return fmt.Errorf("failed to marshal explorer_icons setting: %w", err)
				}

				if _, err := client.Setting.UpdateOne(iconSetting).SetValue(string(newIconSetting)).Save(ctx); err != nil {
					return fmt.Errorf("failed to update explorer_icons setting: %w", err)
				}
			}

			// 2. Apply default file viewers
			// 2.1 Check if it's already applied
			fileViewersSetting, err := client.Setting.Query().Where(setting.Name("file_viewers")).First(ctx)
			if err != nil {
				return fmt.Errorf("failed to query file_viewers setting: %w", err)
			}

			var fileViewers []ftypes.ViewerGroup
			if err := json.Unmarshal([]byte(fileViewersSetting.Value), &fileViewers); err != nil {
				return fmt.Errorf("failed to unmarshal file_viewers setting: %w", err)
			}

			fileViewerExisted := false
			for _, viewer := range fileViewers[0].Viewers {
				if viewer.ID == "excalidraw" {
					fileViewerExisted = true
					break
				}
			}

			// 2.2 If not existed, add it
			if !fileViewerExisted {
				// Found existing excalidraw viewer default setting
				var defaultExcalidrawViewer ftypes.Viewer
				for _, viewer := range defaultFileViewers[0].Viewers {
					if viewer.ID == "excalidraw" {
						defaultExcalidrawViewer = viewer
						break
					}
				}

				fileViewers[0].Viewers = append(fileViewers[0].Viewers, defaultExcalidrawViewer)
				newFileViewersSetting, err := json.Marshal(fileViewers)
				if err != nil {
					return fmt.Errorf("failed to marshal file_viewers setting: %w", err)
				}

				if _, err := client.Setting.UpdateOne(fileViewersSetting).SetValue(string(newFileViewersSetting)).Save(ctx); err != nil {
					return fmt.Errorf("failed to update file_viewers setting: %w", err)
				}
			}

			return nil
		},
	},
	{
		Name:       "apply_email_title_magic_var",
		EndVersion: "4.7.0",
		Func: func(l *log.Helper, client *ent.Client, ctx context.Context) error {
			// 1. Activate Template
			mailActivationTemplateSetting, err := client.Setting.Query().Where(setting.Name("mail_activation_template")).First(ctx)
			if err != nil {
				return fmt.Errorf("failed to query mail_activation_template setting: %w", err)
			}

			var mailActivationTemplate []struct {
				Title    string `json:"title"`
				Body     string `json:"body"`
				Language string `json:"language"`
			}
			if err := json.Unmarshal([]byte(mailActivationTemplateSetting.Value), &mailActivationTemplate); err != nil {
				return fmt.Errorf("failed to unmarshal mail_activation_template setting: %w", err)
			}

			for i, t := range mailActivationTemplate {
				mailActivationTemplate[i].Title = fmt.Sprintf("[{{ .CommonContext.SiteBasic.Name }}] %s", t.Title)
			}

			newMailActivationTemplate, err := json.Marshal(mailActivationTemplate)
			if err != nil {
				return fmt.Errorf("failed to marshal mail_activation_template setting: %w", err)
			}

			if _, err := client.Setting.UpdateOne(mailActivationTemplateSetting).SetValue(string(newMailActivationTemplate)).Save(ctx); err != nil {
				return fmt.Errorf("failed to update mail_activation_template setting: %w", err)
			}

			// 2. Reset Password Template
			mailResetTemplateSetting, err := client.Setting.Query().Where(setting.Name("mail_reset_template")).First(ctx)
			if err != nil {
				return fmt.Errorf("failed to query mail_reset_template setting: %w", err)
			}

			var mailResetTemplate []struct {
				Title    string `json:"title"`
				Body     string `json:"body"`
				Language string `json:"language"`
			}
			if err := json.Unmarshal([]byte(mailResetTemplateSetting.Value), &mailResetTemplate); err != nil {
				return fmt.Errorf("failed to unmarshal mail_reset_template setting: %w", err)
			}

			for i, t := range mailResetTemplate {
				mailResetTemplate[i].Title = fmt.Sprintf("[{{ .CommonContext.SiteBasic.Name }}] %s", t.Title)
			}

			newMailResetTemplate, err := json.Marshal(mailResetTemplate)
			if err != nil {
				return fmt.Errorf("failed to marshal mail_reset_template setting: %w", err)
			}

			if _, err := client.Setting.UpdateOne(mailResetTemplateSetting).SetValue(string(newMailResetTemplate)).Save(ctx); err != nil {
				return fmt.Errorf("failed to update mail_reset_template setting: %w", err)
			}

			return nil
		},
	},
}

func applyPatches(l *log.Helper, client *ent.Client, ctx context.Context, requiredDbVersion string) error {
	allVersionMarks, err := client.Setting.Query().Where(setting.NameHasPrefix(DBVersionPrefix)).All(ctx)
	if err != nil {
		return err
	}

	requiredDbVersion = strings.TrimSuffix(requiredDbVersion, "-pro")

	// Find the latest applied version
	var latestAppliedVersion *semver.Version
	for _, v := range allVersionMarks {
		v.Name = strings.TrimSuffix(v.Name, "-pro")
		version, err := semver.NewVersion(strings.TrimPrefix(v.Name, DBVersionPrefix))
		if err != nil {
			l.Warn("Failed to parse past version %s: %s", v.Name, err)
			continue
		}
		if latestAppliedVersion == nil || version.Compare(latestAppliedVersion) > 0 {
			latestAppliedVersion = version
		}
	}

	requiredVersion, err := semver.NewVersion(requiredDbVersion)
	if err != nil {
		return fmt.Errorf("failed to parse required version %s: %w", requiredDbVersion, err)
	}

	if latestAppliedVersion == nil || requiredVersion.Compare(requiredVersion) > 0 {
		latestAppliedVersion = requiredVersion
	}

	for _, patch := range patches {
		if latestAppliedVersion.Compare(semver.MustParse(patch.EndVersion)) < 0 {
			l.Info("Applying schema patch %s...", patch.Name)
			if err := patch.Func(l, client, ctx); err != nil {
				return err
			}
		}
	}

	return nil
}
