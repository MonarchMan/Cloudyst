package setting

import "time"

type PWASetting struct {
	SmallIcon       string
	MediumIcon      string
	LargeIcon       string
	Display         string
	ThemeColor      string
	BackgroundColor string
}

type SiteBasic struct {
	Name        string
	Title       string
	ID          string
	Description string
	Script      string
}

const (
	CaptchaNormal    = string("normal")
	CaptchaReCaptcha = string("recaptcha")
	CaptchaTcaptcha  = string("tcaptcha")
	CaptchaTurnstile = string("turnstile")
	CaptchaCap       = string("cap")
)

type ReCaptcha struct {
	Key    string
	Secret string
}

type TcCaptcha struct {
	AppID        string
	AppSecretKey string
	SecretID     string
	SecretKey    string
}

type Turnstile struct {
	Key    string
	Secret string
}

type Cap struct {
	InstanceURL string
	SiteKey     string
	SecretKey   string
	AssetServer string
}

type SMTP struct {
	FromName        string
	From            string
	Host            string
	ReplyTo         string
	User            string
	Password        string
	ForceEncryption bool
	Port            int
	Keepalive       int
}

type TokenAuth struct {
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

type DBFS struct {
	UseCursorPagination        bool
	MaxPageSize                int
	MaxRecursiveSearchedFolder int
	UseSSEForSearch            bool
}

type (
	QueueType    string
	QueueSetting struct {
		WorkerNum          int
		MaxExecution       time.Duration
		BackoffFactor      float64
		BackoffMaxDuration time.Duration
		MaxRetry           int
		RetryDelay         time.Duration
	}
)

type ThumbEncode struct {
	Quality int
	Format  string
}

var (
	QueueTypeMediaMeta      = QueueType("media_meta")
	QueueTypeIOIntense      = QueueType("io_intense")
	QueueTypeThumb          = QueueType("thumb")
	QueueTypeEntityRecycle  = QueueType("recycle")
	QueueTypeSlave          = QueueType("slave")
	QueueTypeRemoteDownload = QueueType("remote_download")
)

type CronType string

var (
	CronTypeEntityCollect    = CronType("entity_collect")
	CronTypeTrashBinCollect  = CronType("trash_bin_collect")
	CronTypeOauthCredRefresh = CronType("oauth_cred_refresh")
)

type Theme struct {
	Themes       string
	DefaultTheme string
}

type Logo struct {
	Normal string
	Light  string
}

type LegalDocuments struct {
	PrivacyPolicy  string
	TermsOfService string
}

type CaptchaMode int

const (
	CaptchaModeDigit = CaptchaMode(iota)
	CaptchaModeAudio
	CaptchaModeMath
	CaptchaModeNumberString
	CaptchaModeChinese
)

type Captcha struct {
	Height             int
	Width              int
	Mode               CaptchaMode
	ComplexOfNoiseText int
	MaxSkew            float64
	Fonts              []string
	BgColor            bool
	IsShowNoiseText    bool
	Source             string
	ShowLineOptions    int
	Length             int
	Language           string
}

type ExplorerFrontendSettings struct {
	Icons string
}

const (
	MapProviderOpenStreetMap = "openstreetmap"
	MapProviderGoogle        = "google"
)

const (
	MapGoogleTileTypeRegular   = "regular"
	MapGoogleTileTypeSatellite = "satellite"
	MapGoogleTileTypeTerrain   = "terrain"
)

type MapSetting struct {
	Provider       string
	GoogleTileType string
	MapboxAK       string
}

// Viewer related

type (
	SearchCategory string
)

const (
	CategoryUnknown  = SearchCategory("unknown")
	CategoryImage    = SearchCategory("image")
	CategoryVideo    = SearchCategory("video")
	CategoryAudio    = SearchCategory("audio")
	CategoryDocument = SearchCategory("document")
)

type AppSetting struct {
	Promotion bool
}

type EmailTemplate struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Language string `json:"language"`
}

type Avatar struct {
	Gravatar string `json:"gravatar"`
	Path     string `json:"path"`
}

type AvatarProcess struct {
	Path        string `json:"path"`
	MaxFileSize int64  `json:"max_file_size"`
	MaxWidth    int    `json:"max_width"`
}

type CustomNavItem struct {
	Icon string `json:"icon"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type CustomHTML struct {
	HeadlessFooter string `json:"headless_footer,omitempty"`
	HeadlessBody   string `json:"headless_bottom,omitempty"`
	SidebarBottom  string `json:"sidebar_bottom,omitempty"`
}
