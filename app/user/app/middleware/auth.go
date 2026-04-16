package middleware

import (
	commonpb "api/api/common/v1"
	"common/auth"
	"common/constants"
	"context"
	"strconv"
	"user/ent"
	"user/internal/data"
	"user/internal/data/types"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

type UserCtx struct{}

func UserFromContext(ctx context.Context) *ent.User {
	return ctx.Value(UserCtx{}).(*ent.User)
}

func CurrentUser(uc data.UserClient) middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			uid := 0
			if tr, ok := transport.FromServerContext(ctx); ok {
				if uidStr := tr.RequestHeader().Get(constants.UserIdKey); uidStr != "" {
					uid, _ = strconv.Atoi(uidStr)
				}
			}

			var err error
			ctx, err = SetUserCtx(ctx, uid, uc)
			if err != nil {
				return nil, err
			}
			return handler(ctx, req)
		}
	}
}

func SetUserCtx(ctx context.Context, uid int, uc data.UserClient) (context.Context, error) {
	//ctx = context.WithValue(ctx, data.LoadUserGroup{}, true)
	loginUser, err := uc.GetLoginUserByID(ctx, uid)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get login user: %w", err)
	}
	ctx = context.WithValue(ctx, UserCtx{}, loginUser)
	return ctx, nil
}

func LoginRequired() middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			if u := UserFromContext(ctx); u != nil && !data.IsAnonymousUser(u) {
				return next(ctx, req)
			}
			return nil, errors.Unauthorized("login required", "login required")
		}
	}
}

func CheckAdminPermission() middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			u := UserFromContext(ctx)
			if !u.Edges.Group.Permissions.Enabled(types.GroupPermissionIsAdmin) {
				return nil, errors.Forbidden("admin permission required", "admin permission required")
			}
			return next(ctx, req)
		}
	}
}

func SignRequired(instance auth.Auth) middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			if tr, ok := transport.FromServerContext(ctx); ok {
				if ht, ok := tr.(khttp.Transporter); ok {
					if err := auth.CheckURI(instance, ht.Request().URL); err != nil {
						return nil, commonpb.ErrorParamInvalid("Failed to verify signature: %s", err)
					}
				}
			}
			return next(ctx, req)
		}
	}
}
