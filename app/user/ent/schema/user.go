package schema

import (
	pb "api/api/user/common/v1"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"google.golang.org/protobuf/types/descriptorpb"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("email").
			MaxLen(100).
			Unique().
			Annotations(entproto.Field(2)),
		field.String("nick").
			MaxLen(100).
			Annotations(entproto.Field(3)),
		field.String("password").
			Optional().
			Sensitive().
			Annotations(entproto.Field(4)),
		field.Enum("status").
			Values("active", "inactive", "manual_banned", "sys_banned").
			Default("active").
			Annotations(entproto.Field(5), entproto.Enum(map[string]int32{
				"active":        0,
				"inactive":      1,
				"manual_banned": 2,
				"sys_banned":    3,
			})),
		field.Int64("storage").
			Default(0).
			Annotations(entproto.Field(6)),
		field.String("two_factor_secret").
			Sensitive().
			Optional().
			Annotations(entproto.Field(7)),
		field.String("avatar").
			Optional().
			Annotations(entproto.Field(8)),
		field.JSON("settings", &pb.UserSetting{}).
			Default(&pb.UserSetting{}).
			Optional().
			Annotations(entproto.Field(9, entproto.Type(descriptorpb.FieldDescriptorProto_TYPE_STRING))),
		field.Int("group_users").
			Annotations(entproto.Field(10)),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("group", Group.Type).
			Ref("users").
			Field("group_users").
			Unique().
			Required().
			Annotations(entproto.Field(80)),
		edge.To("passkey", Passkey.Type).
			Annotations(entproto.Field(81)),
		edge.To("dav_accounts", DavAccount.Type).
			Annotations(entproto.Field(82)),
	}
}

func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}

func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
