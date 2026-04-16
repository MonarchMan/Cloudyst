package middleware

import (
	pbuser "api/api/user/users/v1"
	ftypes "api/external/data/file"
	"api/external/trans"
	"common/cache"
	"common/hashid"
	"context"
	"file/internal/biz/filemanager/manager"
	"file/internal/biz/setting"
	"file/internal/data/rpc"
	"file/internal/pkg/utils"
	"file/internal/pkg/wopi"
	"net/http"
	"strings"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

func ViewerSessionValidator(hasher hashid.Encoder, kv cache.Driver, uc pbuser.UserClient, settings setting.Provider) khttp.FilterFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// parse id
			path := strings.TrimPrefix(r.URL.Path, "/file/wopi/")
			parts := strings.Split(path, "/")
			fileId := parts[0]
			id, err := hasher.Decode(fileId, hashid.FileID)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Header().Set(wopi.ServerErrorHeader, "Failed to parse file id")
			}
			utils.WithValue(r, hashid.ObjectIDCtx{}, id)

			// validate session
			token := strings.Split(r.URL.Query().Get(wopi.AccessTokenQuery), ".")
			if len(token) != 2 {
				w.WriteHeader(http.StatusForbidden)
				w.Header().Set(wopi.ServerErrorHeader, "malformed access token")
			}
			sessionRaw, exist := kv.Get(manager.ViewerSessionCachePrefix + token[0])
			if !exist {
				w.WriteHeader(http.StatusForbidden)
				w.Header().Set(wopi.ServerErrorHeader, "invalid access token")
			}

			session := sessionRaw.(manager.ViewerSessionCache)
			user, err := rpc.GetUserInfo(r.Context(), session.UserID, uc)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Header().Set(wopi.ServerErrorHeader, "user not found")
			}
			ctx := context.WithValue(r.Context(), trans.UserCtx{}, user)

			if id != session.FileID {
				w.WriteHeader(http.StatusForbidden)
				w.Header().Set(wopi.ServerErrorHeader, "invalid file")
			}

			viewers := settings.FileViewers(ctx)
			var v *ftypes.Viewer
			for _, group := range viewers {
				for _, viewer := range group.Viewers {
					if viewer.ID == session.ViewerID && !viewer.Disabled {
						v = &viewer
						break
					}
				}

				if v != nil {
					break
				}
			}

			if v == nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Header().Set(wopi.ServerErrorHeader, "viewer not found")
			}

			ctx = context.WithValue(ctx, manager.ViewerCtx{}, v)
			ctx = context.WithValue(ctx, manager.ViewerSessionCacheCtx{}, &session)
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}
