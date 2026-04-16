package schema

import (
	pb "api/api/user/common/v1"
	"common/boolset"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"google.golang.org/protobuf/types/descriptorpb"
)

// DavAccount holds the schema definition for the DavAccount entity.
type DavAccount struct {
	ent.Schema
}

// Fields of the DavAccount.
func (DavAccount) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			StorageKey("id").
			Annotations(entproto.Field(1)),
		field.String("name").
			Annotations(entproto.Field(2)),
		field.Text("uri").
			Annotations(entproto.Field(3)),
		field.String("password").
			Sensitive().
			Annotations(entproto.Field(4)),
		field.Bytes("options").GoType(&boolset.BooleanSet{}).
			Annotations(entproto.Field(5)),
		field.JSON("props", &pb.DavAccountProps{}).
			Optional().
			Annotations(entproto.Field(6, entproto.Type(descriptorpb.FieldDescriptorProto_TYPE_STRING))),
		field.Int("owner_id").
			Annotations(entproto.Field(7)),
	}
}

// Edges of the DavAccount.
func (DavAccount) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", User.Type).
			Ref("dav_accounts").
			Field("owner_id").
			Unique().
			Required().
			Annotations(entproto.Field(80)),
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

func (DavAccount) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
