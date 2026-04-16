package schema

import (
	pb "api/api/file/common/v1"
	mschema "entmodule/ent/schema"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"google.golang.org/protobuf/types/descriptorpb"
)

// StoragePolicy holds the schema definition for the StoragePolicy entity.
type StoragePolicy struct {
	ent.Schema
}

// Fields of the StoragePolicy.
func (StoragePolicy) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Annotations(entproto.Field(2)),
		field.String("type").
			Annotations(entproto.Field(3)),
		field.String("server").
			Optional().
			Annotations(entproto.Field(4)),
		field.String("bucket_name").
			Optional().
			Annotations(entproto.Field(5)),
		field.Bool("is_private").
			Optional().
			Annotations(entproto.Field(6)),
		field.Text("access_key").
			Optional().
			Annotations(entproto.Field(7)),
		field.Text("secret_key").
			Optional().
			Annotations(entproto.Field(8)),
		field.Int64("max_size").
			Optional().
			Annotations(entproto.Field(9)),
		field.String("dir_name_rule").
			Optional().
			Annotations(entproto.Field(10)),
		field.String("file_name_rule").
			Optional().
			Annotations(entproto.Field(11)),
		field.JSON("settings", &pb.PolicySetting{}).
			Default(&pb.PolicySetting{}).
			Optional().
			Annotations(entproto.Field(12, entproto.Type(descriptorpb.FieldDescriptorProto_TYPE_STRING))),
		field.Int("node_id").
			Optional().
			Annotations(entproto.Field(13)),
	}
}

// Edges of the StoragePolicy.
func (StoragePolicy) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("files", File.Type).
			Annotations(entproto.Field(81)),
		edge.To("entities", Entity.Type).
			Annotations(entproto.Field(82)),
		edge.From("node", Node.Type).
			Ref("storage_policy").
			Field("node_id").
			Unique().
			Annotations(entproto.Field(83)),
	}
}

func (StoragePolicy) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mschema.CommonMixin{},
	}
}

func (StoragePolicy) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
