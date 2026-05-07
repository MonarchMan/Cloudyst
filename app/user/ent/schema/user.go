package schema

import (
	"api/external/data/userdata"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("email").
			MaxLen(100).
			Unique(),
		field.String("nick").
			MaxLen(100),
		field.String("password").
			Optional().
			Sensitive(),
		field.Enum("status").
			Values("active", "inactive", "manual_banned", "sys_banned").
			Default("active"),
		field.Int64("storage").
			Default(0),
		field.String("two_factor_secret").
			Sensitive().
			Optional(),
		field.String("avatar").
			Optional(),
		field.JSON("settings", &userdata.UserSetting{}).
			Default(&userdata.UserSetting{}).
			Optional(),
		field.Int("group_users"),
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
