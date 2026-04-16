package middlewares

import (
	pbuser "api/api/user/users/v1"
	"api/external/trans"
	"common/boolset"
	"common/constants"
	"context"
	"strconv"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
)

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

func SetUserCtx(ctx context.Context, uid int, client pbuser.UserClient) (context.Context, error) {
	req := &pbuser.RawUserRequest{Id: int32(uid)}
	user, err := client.GetUserInfo(ctx, req)
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
			if !(&permission).Enabled(int(constants.GroupPermissionIsAdmin)) {
				return nil, errors.Forbidden("admin permission required", "admin permission required")
			}
			return next(ctx, req)
		}
	}
}

func LoginRequired() middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			if u := trans.FromContext(ctx); u != nil && u.Id != 0 {
				return next(ctx, req)
			}
			return nil, errors.Unauthorized("login required", "login required")
		}
	}
}
