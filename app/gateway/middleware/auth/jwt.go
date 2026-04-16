package auth

import (
	"common/auth"
	"common/constants"
	"common/hashid"
	"common/util"
	"net/http"
	"strconv"
	"strings"

	config "gateway/api/gateway/config/v1"
	v1 "gateway/api/gateway/middleware/auth/v1"
	"gateway/middleware"

	"github.com/go-kratos/kratos/v2/log"
	kjwt "github.com/go-kratos/kratos/v2/middleware/auth/jwt"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

var (
	defaultSecret   = util.RandStringRunes(256)
	defaultHashSalt = util.RandStringRunes(64)
)

func init() {
	middleware.Register("auth", New)
}

// New
func New(c *config.Middleware) (middleware.Middleware, error) {
	options := &v1.Auth{
		Secret:   defaultSecret,
		HashSalt: defaultHashSalt,
	}
	if c.Options != nil {
		if err := anypb.UnmarshalTo(c.Options, options, proto.UnmarshalOptions{Merge: true}); err != nil {
			return nil, err
		}
	}
	hasher, err := hashid.New(options.HashSalt)
	if err != nil {
		return nil, err
	}
	whiteList := []string{"/f/", "/s/", "/wopi/", "/dav/"}
	return func(next http.RoundTripper) http.RoundTripper {
		return middleware.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if middleware.IsInWhitelist(req.URL.Path, whiteList...) {
				return next.RoundTrip(req)
			}

			headerVal := req.Header.Get(constants.AuthorizationHeader)
			if strings.HasPrefix(headerVal, constants.TokenHeaderPrefixCr) {
				return nil, kjwt.ErrTokenInvalid
			}

			tokenString := strings.TrimPrefix(headerVal, constants.TokenHeaderPrefix)
			//  宽松验证，严格授权
			if tokenString == "" {
				return next.RoundTrip(req)
			}

			token, err := jwt.ParseWithClaims(tokenString, &auth.Claims{}, func(token *jwt.Token) (any, error) {
				return []byte(options.Secret), nil
			})

			if err != nil {
				log.Warnf("Failed to parse jwt token: %s", err)
				return next.RoundTrip(req)
			}

			claims, ok := token.Claims.(*auth.Claims)
			if ok && claims.TokenType != constants.TokenTypeAccess {
				return nil, kjwt.ErrTokenInvalid
			}

			uid, err := hasher.Decode(claims.Subject, hashid.UserID)
			if err != nil {
				return nil, kjwt.ErrTokenParseFail
			}

			req.Header.Set(constants.UserIdKey, strconv.Itoa(uid))
			return next.RoundTrip(req)
		})
	}, nil
}
