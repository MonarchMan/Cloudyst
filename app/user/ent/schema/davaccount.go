package schema

import (
	pb "api/api/user/common/v1"
	"common/boolset"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DavAccount holds the schema definition for the DavAccount entity.
type DavAccount struct {
	ent.Schema
}

// Fields of the DavAccount.
func (DavAccount) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			StorageKey("id"),
		field.String("name"),
		field.Text("uri"),
		field.String("password").
			Sensitive(),
		field.Bytes("options").GoType(&boolset.BooleanSet{}),
		field.JSON("props", &pb.DavAccountProps{}).
			Optional(),
		field.Int("owner_id"),
	}
}

// Edges of the DavAccount.
func (DavAccount) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", User.Type).
			Ref("dav_accounts").
			Field("owner_id").
			Unique().
			Required(),
	}
}

// Indexes of the DavAccount.
func (DavAccount) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("owner_id", "password").
			Unique(),
	}
}

func (DavAccount) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}
