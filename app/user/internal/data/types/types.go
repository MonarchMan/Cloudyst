package types

import (
	pb "api/api/user/common/v1"
)

type DBType string

var (
	SQLiteDB   DBType = "sqlite"
	SQLite3DB  DBType = "sqlite3"
	MySqlDB    DBType = "mysql"
	MsSqlDB    DBType = "mssql"
	PostgresDB DBType = "postgres"
	MariaDB    DBType = "mariadb"
)

type (
	UserSetting struct {
		ProfileOff          bool                     `json:"profile_off,omitempty"`
		PreferredTheme      string                   `json:"preferred_theme,omitempty"`
		VersionRetention    bool                     `json:"version_retention,omitempty"`
		VersionRetentionExt []string                 `json:"version_retention_ext,omitempty"`
		VersionRetentionMax int                      `json:"version_retention_max,omitempty"`
		Pined               []*pb.PinedFile          `json:"pined,omitempty"`
		Language            string                   `json:"email_language,omitempty"`
		DisableViewSync     bool                     `json:"disable_view_sync,omitempty"`
		FsViewMap           map[string]ExplorerView  `json:"fs_view_map,omitempty"`
		ShareLinksInProfile ShareLinksInProfileLevel `json:"share_links_in_profile,omitempty"`
	}

	ShareLinksInProfileLevel string

	// GroupSetting 用户组其他配置
	GroupSetting struct {
		CompressSize          int64                  `json:"compress_size,omitempty"` // 可压缩大小
		DecompressSize        int64                  `json:"decompress_size,omitempty"`
		RemoteDownloadOptions map[string]interface{} `json:"remote_download_options,omitempty"` // 离线下载用户组配置
		SourceBatchSize       int                    `json:"source_batch,omitempty"`
		Aria2BatchSize        int                    `json:"aria2_batch,omitempty"`
		MaxWalkedFiles        int                    `json:"max_walked_files,omitempty"`
		TrashRetention        int                    `json:"trash_retention,omitempty"`
		RedirectedSource      bool                   `json:"redirected_source,omitempty"`
	}

	ExplorerView struct {
		PageSize       int              `json:"page_size" binding:"min=50"`
		Order          string           `json:"order,omitempty" binding:"max=255"`
		OrderDirection string           `json:"order_direction,omitempty" binding:"eq=asc|eq=desc"`
		View           string           `json:"view,omitempty" binding:"eq=list|eq=grid|eq=gallery"`
		Thumbnail      bool             `json:"thumbnail,omitempty"`
		GalleryWidth   int              `json:"gallery_width,omitempty" binding:"min=50,max=500"`
		Columns        []ListViewColumn `json:"columns,omitempty" binding:"max=1000"`
	}

	ListViewColumn struct {
		Type  int             `json:"type" binding:"min=0"`
		Width *int            `json:"width,omitempty"`
		Props *ColumTypeProps `json:"props,omitempty"`
	}

	ColumTypeProps struct {
		MetadataKey   string `json:"metadata_key,omitempty" binding:"max=255"`
		CustomPropsID string `json:"custom_props_id,omitempty" binding:"max=255"`
	}

	StoragePolicyInfo struct {
		ID        int    `json:"id" binding:"min=0"`
		Name      string `json:"name" binding:"max=255"`
		Type      string `json:"type" binding:"max=255"`
		IsPrivate bool   `json:"is_private,omitempty"`
	}
)

type (
	CustomPropsType string
	CustomProps     struct {
		ID      string          `json:"id"`
		Name    string          `json:"name"`
		Type    CustomPropsType `json:"type"`
		Max     int             `json:"max,omitempty"`
		Min     int             `json:"min,omitempty"`
		Default string          `json:"default,omitempty"`
		Options []string        `json:"options,omitempty"`
		Icon    string          `json:"icon,omitempty"`
	}
)

const (
	CustomPropsTypeText        = "text"
	CustomPropsTypeNumber      = "number"
	CustomPropsTypeBoolean     = "boolean"
	CustomPropsTypeSelect      = "select"
	CustomPropsTypeMultiSelect = "multi_select"
	CustomPropsTypeLink        = "link"
	CustomPropsTypeRating      = "rating"

	AuthnSessionKey = "authn_session_"
	UserResetPrefix = "user_reset_"
	GravatarAvatar  = "gravatar"
	FileAvatar      = "files"
	SearchLimit     = 10
)

const (
	GroupPermissionIsAdmin = iota
	GroupPermissionIsAnonymous
	GroupPermissionShare
	GroupPermissionWebDAV
	GroupPermissionArchiveDownload
	GroupPermissionArchiveTask
	GroupPermissionWebDAVProxy
	GroupPermissionShareDownload
	GroupPermission_CommunityPlaceholder1
	GroupPermissionRemoteDownload
	GroupPermission_CommunityPlaceholder2
	GroupPermissionRedirectedSource // not used
	GroupPermissionAdvanceDelete
	GroupPermission_CommunityPlaceholder3
	GroupPermission_CommunityPlaceholder4
	GroupPermissionSetExplicitUser_placeholder
	GroupPermissionIgnoreFileOwnership // not used
	GroupPermissionUniqueRedirectDirectLink
)

const (
	ViewerActionView = "view"
	ViewerActionEdit = "edit"

	ViewerTypeBuiltin = "builtin"
	ViewerTypeWopi    = "wopi"
	ViewerTypeCustom  = "custom"
)

type (
	FileTypeIconSetting struct {
		Exts      []string `json:"exts"`
		Icon      string   `json:"icon,omitempty"`
		Color     string   `json:"color,omitempty"`
		ColorDark string   `json:"color_dark,omitempty"`
		Img       string   `json:"img,omitempty"`
	}
)
