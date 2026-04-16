package schema

import (
	pb "api/api/file/common/v1"
	mschema "entmodule/ent/schema"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Task holds the schema definition for the Task entity.
type Task struct {
	ent.Schema
}

// Fields of the Task.
func (Task) Fields() []ent.Field {
	return []ent.Field{
		field.String("type").
			Annotations(entproto.Field(2)),
		field.Enum("status").
			Values("queued", "processing", "suspending", "error", "canceled", "completed").
			Default("queued").
			Annotations(entproto.Field(3), entproto.Enum(map[string]int32{
				"queued":     0,
				"processing": 1,
				"suspending": 2,
				"error":      3,
				"canceled":   4,
				"completed":  5,
			})),
		field.JSON("public_state", &pb.TaskPublicState{}).
			Annotations(entproto.Field(4, entproto.Type(descriptorpb.FieldDescriptorProto_TYPE_STRING))),
		field.Text("private_state").
			Optional().
			Annotations(entproto.Field(5)),
		field.String("trace_id").
			Optional().
			Immutable().
			Annotations(entproto.Field(6)),
		field.Int("user_id").
			Optional().
			Annotations(entproto.Field(7)),
	}
}

// Edges of the Task.
func (Task) Edges() []ent.Edge {
	return nil
}

func (Task) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mschema.CommonMixin{},
	}
}

func (Task) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
