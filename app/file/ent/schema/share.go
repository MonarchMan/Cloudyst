package schema

import (
	"api/external/data/userdata"
	"file/internal/data/types"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Share holds the schema definition for the Share entity.
type Share struct {
	ent.Schema
}

// Fields of the Share.
func (Share) Fields() []ent.Field {
	return []ent.Field{
		field.String("password").
			Optional(),
		field.Int("views").
			Default(0),
		field.Int("downloads").
			Default(0),
		field.Time("expires").
			Nillable().
			Optional().
			SchemaType(map[string]string{
				dialect.MySQL: "datetime",
			}),
		field.Int("remain_downloads").
			Nillable().
			Optional(),
		field.JSON("props", &types.ShareProps{}).
			Optional(),
		field.Int("owner_id").
			Optional(),
		field.JSON("owner_info", &userdata.UserInfo{}).
			Default(&userdata.UserInfo{}).
			Optional(),
	}
}

// Edges of the Share.
func (Share) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("file", File.Type).
			Ref("shares").Unique(),
	}
}

func (Share) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}
