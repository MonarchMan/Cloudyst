package device

import (
	config "gateway/api/gateway/config/v1"
	"gateway/middleware"
	"net/http"
	"regexp"

	"github.com/go-kratos/kratos/v2/errors"
)

func init() {
	middleware.Register("device", MobileOnlyMiddleware)
}

// 简单的移动端 User-Agent 正则
// 包含 Android, iPhone, iPad, Mobile 等关键字
var mobileRegex = regexp.MustCompile(`(?i)(Android|iPhone|iPod|iPad|Mobile|Windows Phone|BlackBerry)`)

// MobileOnlyMiddleware 仅允许移动端设备访问
func MobileOnlyMiddleware(c *config.Middleware) (middleware.Middleware, error) {
	return func(next http.RoundTripper) http.RoundTripper {
		return middleware.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			ua := req.Header.Get("User-Agent")

			// 检查是否包含移动端特征
			if !mobileRegex.MatchString(ua) {
				// 拒绝访问
				return nil, errors.Forbidden("ACCESS_DENIED", "该功能仅支持移动端设备访问")
			}

			// 如果通过，放行
			return next.RoundTrip(req)
		})
	}, nil
}
