package constants

const (
	APIPrefix      = "/api/v1"
	APIPrefixSlave = "/api/v1/file/slave"
	CrHeaderPrefix = "X-Cr-"
	CloudScheme    = "cloudyst"
	BackendVersion = "0.0.1"
	FileAPIPrefix  = "/api/v1/file"
	UserAPIPrefix  = "/api/v1/user"
	AiAPIPrefix    = "/api/v1/ai"

	AiServicePrefix = "ai"
	DBVersionPrefix = "db_version_"
)

const ConfigPath = "../../configs"

// FileSystemType is the type of files system.
type (
	FileSystemType string
)

const (
	FileSystemMy           = FileSystemType("my")
	FileSystemShare        = FileSystemType("share")
	FileSystemTrash        = FileSystemType("trash")
	FileSystemSharedWithMe = FileSystemType("shared_with_me")
	FileSystemUnknown      = FileSystemType("unknown")
)

// Node
const (
	MasterMode string = "master"
	SlaveMode  string = "slave"
)

// Size Constants
const (
	MB = 1 << 20
	GB = 1 << 30
	TB = 1 << 40
	PB = 1 << 50
)

// Status
const (
	StatusActive       string = "active"
	StatusInactive     string = "inactive"
	StatusManualBanned string = "manual_banned"
	StatusSysBanned    string = "sys_banned"
)

// Request Header
const (
	AuthorizationHeader = "Authorization"
	TokenHeaderPrefix   = "Bearer "
	RevokeTokenPrefix   = "jwt_revoke_"
	TokenHeaderPrefixCr = "Bearer Cr "
	UserIdKey           = "X-User-Id"
)

const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

const (
	DavPrefix = "/dav"
)

// http://www.webdav.org/specs/rfc4918.html#status.code.extensions.to.http11
const (
	StatusMulti               = 207
	StatusUnprocessableEntity = 422
	StatusLocked              = 423
	StatusFailedDependency    = 424
	StatusInsufficientStorage = 507
)

const (
	DavAccountReadOnly int = iota
	DavAccountProxy
	DavAccountDisableSysFiles
)

var SupportedExts = []string{"jpg", "jpeg", "png", "gif"}

const SystemKnowledgeID = 1
