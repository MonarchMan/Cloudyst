package schema

import (
	pb "api/api/file/common/v1"
	mschema "entmodule/ent/schema"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Share holds the schema definition for the Share entity.
type Share struct {
	ent.Schema
}

// Fields of the Share.
func (Share) Fields() []ent.Field {
	return []ent.Field{
		field.String("password").
			Optional().
			Annotations(entproto.Field(2)),
		field.Int("views").
			Default(0).
			Annotations(entproto.Field(3)),
		field.Int("downloads").
			Default(0).
			Annotations(entproto.Field(4)),
		field.Time("expires").
			Nillable().
			Optional().
			SchemaType(map[string]string{
				dialect.MySQL: "datetime",
			}).
			Annotations(entproto.Field(5)),
		field.Int("remain_downloads").
			Nillable().
			Optional().
			Annotations(entproto.Field(6)),
		field.JSON("props", &pb.ShareProps{}).
			Optional().
			Annotations(entproto.Field(7, entproto.Type(descriptorpb.FieldDescriptorProto_TYPE_STRING))),
		field.Int("owner_id").
			Annotations(entproto.Field(8)),
		field.JSON("owner_info", &pb.UserInfo{}).
			Default(&pb.UserInfo{}).
			Optional().
			Annotations(entproto.Field(9, entproto.Type(descriptorpb.FieldDescriptorProto_TYPE_STRING))),
	}
}

// Edges of the Share.
func (Share) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("file", File.Type).
			Ref("shares").Unique().
			Annotations(entproto.Field(81)),
	}
}

func (Share) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mschema.CommonMixin{},
	}
}

func (Share) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
