package filters

import (
	pbuser "api/api/user/users/v1"
	cm "api/external/middlewares"
	"api/external/trans"
	"common/constants"
	"net/http"
	"strconv"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

func CurrentUser(client pbuser.UserClient) khttp.FilterFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			uid := 0
			if uidStr := req.Header.Get(constants.UserIdKey); uidStr != "" {
				uid, _ = strconv.Atoi(uidStr)
			}
			var err error
			ctx, err := cm.SetUserCtx(req.Context(), uid, client)
			if err != nil {
				return
			}
			req = req.WithContext(ctx)
			next.ServeHTTP(w, req)
		})
	}
}

func LoginRequired() khttp.FilterFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			if u := trans.FromContext(ctx); u != nil && u.Id != 0 {
				next.ServeHTTP(w, req)
			}
			w.WriteHeader(http.StatusUnauthorized)
		})
	}
}
