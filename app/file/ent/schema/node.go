package schema

import (
	pb "api/api/file/common/v1"
	"common/boolset"
	mschema "entmodule/ent/schema"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Node holds the schema definition for the Node entity.
type Node struct {
	ent.Schema
}

// Fields of the Node.
func (Node) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("status").
			Values("active", "suspended").
			Annotations(entproto.Field(2), entproto.Enum(map[string]int32{
				"active":    1,
				"suspended": 2,
			})),
		field.String("name").
			Annotations(entproto.Field(3)),
		field.Enum("type").
			Values("master", "slave").
			Annotations(entproto.Field(4), entproto.Enum(map[string]int32{
				"master": 1,
				"slave":  2,
			})),
		field.String("server").
			Optional().
			Annotations(entproto.Field(5)),
		field.String("slave_key").
			Optional().
			Annotations(entproto.Field(6)),
		field.Bytes("capabilities").
			GoType(&boolset.BooleanSet{}).
			Annotations(entproto.Field(7)),
		field.JSON("settings", &pb.NodeSetting{}).
			Default(&pb.NodeSetting{}).
			Optional().
			Annotations(entproto.Field(8, entproto.Type(descriptorpb.FieldDescriptorProto_TYPE_STRING))),
		field.Int("weight").
			Default(0).
			Annotations(entproto.Field(9)),
	}
}

// Edges of the Node.
func (Node) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("storage_policy", StoragePolicy.Type).
			Annotations(entproto.Field(81)),
	}
}

func (Node) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mschema.CommonMixin{},
	}
}

func (Node) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
