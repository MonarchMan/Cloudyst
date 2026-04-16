package schema

import (
	pb "api/api/user/common/v1"
	"common/boolset"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
)

// Group holds the schema definition for the Group entity.
type Group struct {
	ent.Schema
}

// Fields of the Group.
func (Group) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Annotations(entproto.Field(2)),
		field.Int64("max_storage").
			Optional().
			Annotations(entproto.Field(3)),
		field.Int("speed_limit").
			Optional().
			Annotations(entproto.Field(4)),
		field.Bytes("permissions").GoType(&boolset.BooleanSet{}).
			Annotations(entproto.Field(5)),
		field.JSON("settings", &pb.GroupSetting{}).
			Default(&pb.GroupSetting{}).
			Optional().
			Annotations(entproto.Field(6, entproto.Type(descriptor.FieldDescriptorProto_TYPE_STRING))),
		field.Int("storage_policy_id").Optional().
			Annotations(entproto.Field(7)),
		field.JSON("storage_policy_info", &pb.StoragePolicyInfo{}).
			Default(&pb.StoragePolicyInfo{}).
			Optional().
			Annotations(entproto.Field(8, entproto.Type(descriptor.FieldDescriptorProto_TYPE_STRING))),
	}
}

// Edges of the Group.
func (Group) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("users", User.Type).
			Annotations(entproto.Field(80)),
	}
}

func (Group) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}

func (Group) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
