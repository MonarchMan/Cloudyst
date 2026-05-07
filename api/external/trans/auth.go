package trans

import (
	userpb "api/api/user/common/v1"
	"api/external/data/userdata"
	"context"
)

type (
	UserCtx struct{}
)

func FromContext(ctx context.Context) *userdata.User {
	return ctx.Value(UserCtx{}).(*userdata.User)
}

func WithValue(ctx context.Context, user *userdata.User) context.Context {
	return context.WithValue(ctx, UserCtx{}, user)
}

func WithProtoValue(ctx context.Context, protoUser *userpb.User) context.Context {
	user := userdata.UserFromProto(protoUser)
	return WithValue(ctx, user)
}
