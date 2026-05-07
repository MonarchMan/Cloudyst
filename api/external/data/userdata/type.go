package userdata

import (
	"api/external/data/filedata"
	"common/boolset"
	"time"
)

type UserInfo struct {
	ID        string    `json:"id"`
	Nickname  string    `json:"nickname"`
	Avatar    string    `json:"avatar"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

type User struct {
	// ID of the ent.
	ID int `json:"id,omitempty"`
	// CreatedAt holds the value of the "created_at" field.
	CreatedAt time.Time `json:"created_at,omitempty"`
	// UpdatedAt holds the value of the "updated_at" field.
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	// DeletedAt holds the value of the "deleted_at" field.
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	// Email holds the value of the "email" field.
	Email string `json:"email,omitempty"`
	// Nick holds the value of the "nick" field.
	Nick string `json:"nick,omitempty"`
	// Password holds the value of the "password" field.
	Password string `json:"-"`
	// Status holds the value of the "status" field.
	Status string `json:"status,omitempty"`
	// Storage holds the value of the "storage" field.
	Storage int64 `json:"storage,omitempty"`
	// TwoFactorSecret holds the value of the "two_factor_secret" field.
	TwoFactorSecret string `json:"-"`
	// Avatar holds the value of the "avatar" field.
	Avatar string `json:"avatar,omitempty"`
	// Settings holds the value of the "settings" field.
	Settings *UserSetting `json:"settings,omitempty"`
	// GroupUsers holds the value of the "group_users" field.
	GroupUsers int `json:"group_users,omitempty"`
	// Group holds the value of the group edge.
	Group *Group `json:"group,omitempty"`
}

type Group struct {
	// ID of the ent.
	ID int `json:"id,omitempty"`
	// CreatedAt holds the value of the "created_at" field.
	CreatedAt time.Time `json:"created_at,omitempty"`
	// UpdatedAt holds the value of the "updated_at" field.
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	// DeletedAt holds the value of the "deleted_at" field.
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	// Name holds the value of the "name" field.
	Name string `json:"name,omitempty"`
	// MaxStorage holds the value of the "max_storage" field.
	MaxStorage int64 `json:"max_storage,omitempty"`
	// SpeedLimit holds the value of the "speed_limit" field.
	SpeedLimit int `json:"speed_limit,omitempty"`
	// Permissions holds the value of the "permissions" field.
	Permissions *boolset.BooleanSet `json:"permissions,omitempty"`
	// Settings holds the value of the "settings" field.
	Settings *GroupSetting `json:"settings,omitempty"`
	// StoragePolicyID holds the value of the "storage_policy_id" field.
	StoragePolicyID int `json:"storage_policy_id,omitempty"`
	// StoragePolicyInfo holds the value of the "storage_policy_info" field.
	StoragePolicyInfo *filedata.StoragePolicyInfo `json:"storage_policy_info,omitempty"`
}

type UserSetting struct {
	ProfileOff          bool                             `json:"profile_off,omitempty"`
	PreferredTheme      string                           `json:"preferred_theme,omitempty"`
	VersionRetention    bool                             `json:"version_retention,omitempty"`
	VersionRetentionExt []string                         `json:"version_retention_ext,omitempty"`
	VersionRetentionMax int                              `json:"version_retention_max,omitempty"`
	Pined               []*PinedFile                     `json:"pined,omitempty"`
	Language            string                           `json:"email_language,omitempty"`
	DisableViewSync     bool                             `json:"disable_view_sync,omitempty"`
	FsViewMap           map[string]filedata.ExplorerView `json:"fs_view_map,omitempty"`
	ShareLinksInProfile string                           `json:"share_links_in_profile,omitempty"`
}

type GroupSetting struct {
	CompressSize          int64                  `json:"compress_size,omitempty"` // 可压缩大小
	DecompressSize        int64                  `json:"decompress_size,omitempty"`
	RemoteDownloadOptions map[string]interface{} `json:"remote_download_options,omitempty"` // 离线下载用户组配置
	SourceBatchSize       int                    `json:"source_batch,omitempty"`
	Aria2BatchSize        int                    `json:"aria2_batch,omitempty"`
	MaxWalkedFiles        int                    `json:"max_walked_files,omitempty"`
	TrashRetention        int                    `json:"trash_retention,omitempty"`
	RedirectedSource      bool                   `json:"redirected_source,omitempty"`
}

type PinedFile struct {
	Uri  string `json:"uri"`
	Name string `json:"name,omitempty"`
}
