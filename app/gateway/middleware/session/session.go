package session

import (
	"common/cache"
	"common/util"
	config "gateway/api/gateway/config/v1"
	v1 "gateway/api/gateway/middleware/session/v1"
	"gateway/middleware"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/gorilla/sessions"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// SessionName 会话名称常量
const SessionName = "cloudyst-session"

// var GlobalStore Store
var kvConfig atomic.Pointer[config.Redis]

func init() {
	//kvConfig.Store(&config.Redis{})
}

func Init(c *config.Redis) {
	l := log.GetLogger()
	var kv cache.Driver
	if c == nil || c.Addr == "" {
		kv = cache.NewMemoStore(cache.DefaultCacheFile, l)
	}
	kv = cache.NewRedisStore(l, 10, int(c.Db), c.Network, c.Addr, c.User, c.Password, c.UseTls, c.TlsSkipVerify)
	middleware.Register("session", New(kv))
}

func SetKcConfig(c *config.Redis) {
	kvConfig.Store(c)
}

func New(kv cache.Driver) middleware.Factory {
	return func(c *config.Middleware) (middleware.Middleware, error) {
		options := &v1.Session{}
		if c.Options != nil {
			if err := anypb.UnmarshalTo(c.Options, options, proto.UnmarshalOptions{Merge: true}); err != nil {
				return nil, err
			}
		}
		if options.Secret == "" {
			options.Secret = util.RandStringRunes(64)
		}

		// 初始化会话存储
		globalStore := NewStore(kv, []byte(options.Secret))

		if options.Path == "" {
			options.Path = "/"
		}
		opts := &sessions.Options{
			HttpOnly: options.HttpOnly,
			MaxAge:   60 * 86400,
			Path:     options.Path,
			Secure:   false,
		}
		// 配置会话选项
		switch strings.ToLower(options.SameSite) {
		case "default":
			opts.SameSite = http.SameSiteDefaultMode
		case "none":
			opts.SameSite = http.SameSiteNoneMode
		case "strict":
			opts.SameSite = http.SameSiteStrictMode
		case "lax":
			opts.SameSite = http.SameSiteLaxMode
		default:
			opts.SameSite = http.SameSiteDefaultMode
		}

		globalStore.SetOptions(opts)
		whiteList := []string{"/f/", "/s/", "/wopi/", "/dav/"}
		return func(next http.RoundTripper) http.RoundTripper {
			return middleware.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				if middleware.IsInWhitelist(req.URL.Path, whiteList...) {
					return next.RoundTrip(req)
				}

				session, err := globalStore.Get(req, SessionName)
				if err != nil {
					return nil, err
				}
				// 提取并注入 Header 传给后端
				//req.Header.Set("x-md-global-user-id", fmt.Sprintf("%v", userIDVal))
				resp, err := next.RoundTrip(req)
				if err != nil {
					return resp, err
				}
				if err := globalStore.SaveWithHeader(req, resp.Header, session); err != nil {
					log.Errorf("session save error: %v", err)
				}
				return resp, err
			})
		}, nil
	}
}
