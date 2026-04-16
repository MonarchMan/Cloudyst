package middleware

import (
	commonpb "api/api/common/v1"
	filepb "api/api/file/common/v1"
	userpb "api/api/user/common/v1"
	pbuser "api/api/user/users/v1"
	"api/external/trans"
	"common/auth"
	"common/boolset"
	"common/cache"
	"common/constants"
	"common/request"
	"common/serializer"
	"context"
	"file/app/response"
	"file/internal/biz/filemanager/driver/oss"
	"file/internal/biz/filemanager/driver/upyun"
	"file/internal/biz/filemanager/fs"
	"file/internal/biz/filemanager/manager"
	"file/internal/conf"
	"file/internal/data"
	"file/internal/data/rpc"
	"file/internal/data/types"
	"file/internal/pkg/utils"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/qiniu/go-sdk/v7/auth/qbox"
)

func UseUploadSessionMid(policyType string, kv cache.Driver, client pbuser.UserClient) middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			r, err := utils.RequestFromContext(ctx)
			if err != nil {
				return nil, errors.InternalServer(err.Error(), "internal error")
			}
			if err := uploacCallbackCheck(r, policyType, kv, client); err != nil {
				return nil, errors.Unauthorized(err.Error(), "Unauthorized")
			}
			return next(ctx, req)
		}
	}
}

func UseUploadSession(policyType string, kv cache.Driver, client pbuser.UserClient) khttp.FilterFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if err := uploacCallbackCheck(req, policyType, kv, client); err != nil {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

func uploacCallbackCheck(req *http.Request, policyType string, kv cache.Driver, client pbuser.UserClient) error {
	path := strings.TrimPrefix(req.URL.Path, "/")
	parts := strings.Split(path, "/")
	// 验证 Callback Key
	sessionID := parts[1]
	if sessionID == "" {
		return commonpb.ErrorParamInvalid("sessionID cannot be empty")
	}

	callbackSessionRaw, exist := kv.Get(manager.UploadSessionCachePrefix + sessionID)
	if !exist {
		return serializer.NewError(serializer.CodeUploadSessionExpired, "Upload session does not exist or expired", nil)
	}

	callbackSession := callbackSessionRaw.(fs.UploadSession)
	ctx := context.WithValue(req.Context(), manager.UploadSessionCtx, &callbackSession)
	req = req.WithContext(ctx)
	if callbackSession.Policy.Type != policyType {
		return filepb.ErrorPolicyNotAllowed("Policy type not allowed")
	}

	if _, err := SetUserCtx(ctx, callbackSession.UID, client); err != nil {
		return err
	}

	return nil
}

func RemoteCallbackAuth() middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			r, err := utils.RequestFromContext(ctx)
			if err != nil {
				return nil, errors.InternalServer(err.Error(), "internal error")
			}
			session := r.Context().Value(manager.UploadSessionCtx).(*fs.UploadSession)
			if session.Policy.Edges.Node == nil {
				return nil, userpb.ErrorCredentialInvalid("Node not found")
			}
			authInstance := auth.HMACAuth{SecretKey: []byte(session.Policy.Edges.Node.SlaveKey)}
			if err := auth.CheckRequest(authInstance, r); err != nil {
				return nil, userpb.ErrorCredentialInvalid("Signature verification failed: %s", err)
			}
			return next(ctx, req)
		}
	}
}

func OssCallbackAuthFilter(kv cache.Driver, c *conf.Bootstrap) khttp.FilterFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			client := request.NewClient(c.Server.Sys.Mode, request.WithContext(req.Context()), request.WithLogger(log.GetLogger()))
			if err := oss.VerifyCallbackSignature(req, kv, client); err != nil {
				log.Debugf("Failed to verify callback request: %s", err)
				http.Error(w, "Failed to verify callback request", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

func UpyunCallbackAuth() khttp.FilterFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			uploadSession := req.Context().Value(manager.UploadSessionCtx).(*fs.UploadSession)
			if err := upyun.ValidateCallback(req, uploadSession); err != nil {
				log.Errorf("Failed to verify callback request: %v", err)
				http.Error(w, "Failed to verify callback request", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

func QiniuCallbackAuth() khttp.FilterFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			uploadSession := req.Context().Value(manager.UploadSessionCtx).(*fs.UploadSession)

			// 验证回调是否来自qiniu
			mac := qbox.NewMac(uploadSession.Policy.AccessKey, uploadSession.Policy.SecretKey)
			ok, err := mac.VerifyCallback(req)
			if err != nil {
				log.Errorf("Failed to verify callback request: %v", err)
				http.Error(w, "Failed to verify callback request", http.StatusUnauthorized)
				return
			}

			if !ok {
				http.Error(w, "Invalid signature", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

func WebDavAuth(client pbuser.UserClient) khttp.FilterFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			username, password, ok := req.BasicAuth()
			if !ok {
				if req.Method == http.MethodOptions {
					next.ServeHTTP(w, req)
				}
				w.Header()["WWW-Authenticate"] = []string{`Basic realm="cloudreve"`}
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			ctx := req.Context()
			expectedUser, err := client.GetActiveUserByDavAccount(ctx, &pbuser.GetActiveUserByDavAccountRequest{
				Username: username,
				Password: password,
			})
			if err != nil {
				if username == "" {
					if u, err := client.GetUserByEmail(ctx, &pbuser.GetUserByEmailRequest{
						Email: username,
					}); err == nil {
						req = req.WithContext(context.WithValue(ctx, trans.UserCtx{}, u))
					}
				}
				log.Debugf("WebDAVAuth: failed to get user %q with provided credential: %s", username, err)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			//Validate dav account
			accounts := expectedUser.DavAccounts
			if accounts == nil || len(accounts) == 0 {
				log.Debugf("WebDAVAuth: failed to get user dav accounts %q with provided credential: %s", username, err)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// 用户组已启用WebDAV？
			group := expectedUser.Group
			if group == nil {
				log.Debugf("WebDAVAuth: user group not found: %s", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			permissions := boolset.BooleanSet(group.Permissions)
			if !(&permissions).Enabled(types.GroupPermissionWebDAV) {
				log.Debugf("WebDAVAuth: user %q does not have WebDAV permission.", expectedUser.Email)
				w.WriteHeader(http.StatusForbidden)
				return
			}

			// 检查是否只读
			options := boolset.BooleanSet(expectedUser.DavAccounts[0].Options)
			if (&options).Enabled(constants.DavAccountReadOnly) {
				switch req.Method {
				case http.MethodDelete, http.MethodPut, "MKCOL", "COPY", "MOVE", "LOCK", "UNLOCK":
					w.WriteHeader(http.StatusForbidden)
					return
				}
			}

			req = req.WithContext(context.WithValue(ctx, trans.UserCtx{}, expectedUser))
			next.ServeHTTP(w, req)
		})
	}
}

func SignRequired(instance auth.Auth) khttp.FilterFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			var err error
			switch req.Method {
			case http.MethodPut, http.MethodPost, http.MethodPatch:
				err = auth.CheckRequest(instance, req)
			default:
				err = auth.CheckURI(instance, req.URL)
			}

			if err != nil {
				response.ErrorEncoder(w, req, err)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

func CurrentUser(client pbuser.UserClient) middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			uid := 0
			if tr, ok := transport.FromServerContext(ctx); ok {
				if uidStr := tr.RequestHeader().Get(constants.UserIdKey); uidStr != "" {
					uid, _ = strconv.Atoi(uidStr)
				}
			}

			var err error
			ctx, err = SetUserCtx(ctx, uid, client)
			if err != nil {
				return nil, err
			}
			return handler(ctx, req)
		}
	}
}

func LoginRequired() middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			if u := trans.FromContext(ctx); u != nil && !data.IsAnonymousUser(u) {
				return next(ctx, req)
			}
			return nil, errors.Unauthorized("login required", "login required")
		}
	}
}

func SetUserCtx(ctx context.Context, uid int, client pbuser.UserClient) (context.Context, error) {
	user, err := rpc.GetUserInfo(ctx, uid, client)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, trans.UserCtx{}, user)
	return ctx, nil
}

func CheckAdminPermission() middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			u := trans.FromContext(ctx)
			permission := boolset.BooleanSet(u.Group.Permissions)
			if !(&permission).Enabled(types.GroupPermissionIsAdmin) {
				return nil, errors.Forbidden("admin permission required", "admin permission required")
			}
			return next(ctx, req)
		}
	}
}

func MatchRoutes(url string, routes []string) bool {
	for _, u := range routes {
		if strings.HasPrefix(url, u) {
			return true
		}
	}
	return false
}
