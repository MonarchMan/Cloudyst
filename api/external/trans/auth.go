package trans

import (
	userpb "api/api/user/common/v1"
	"context"
)

type (
	UserCtx struct{}
)

func FromContext(ctx context.Context) *userpb.User {
	return ctx.Value(UserCtx{}).(*userpb.User)
}
