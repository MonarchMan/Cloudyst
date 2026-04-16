package security

import (
	config "gateway/api/gateway/config/v1"
	v1 "gateway/api/gateway/middleware/security/v1"
	"gateway/middleware"
	"net/http"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// 添加init函数注册中间件
func init() {
	middleware.Register("security", MiddleWare)
}

func MiddleWare(c *config.Middleware) (middleware.Middleware, error) {
	return func(next http.RoundTripper) http.RoundTripper {
		return middleware.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			resp, err := next.RoundTrip(req)
			if err != nil {
				return nil, err
			}
			options := &v1.Security{
				CacheControl: proto.String("private, no-cache"),
			}
			if c.Options != nil {
				if err := anypb.UnmarshalTo(c.Options, options, proto.UnmarshalOptions{Merge: true}); err != nil {
					return nil, err
				}
			}
			resp.Header.Set("Cache-Control", *options.CacheControl)
			return resp, nil
		})
	}, nil
}
