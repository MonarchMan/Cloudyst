package schema

import (
	"queue"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// Task holds the schema definition for the Task entity.
type Task struct {
	ent.Schema
}

// Fields of the Task.
func (Task) Fields() []ent.Field {
	return []ent.Field{
		field.String("type"),
		field.Enum("status").
			GoType(queue.TaskStatus("")).
			Default(string(queue.StatusQueued)),
		field.JSON("public_state", &queue.TaskPublicState{}),
		field.Text("private_state").
			Optional(),
		field.String("trace_id").
			Optional().
			Immutable(),
		field.Int("user_id").
			Optional(),
	}
}

// Edges of the Task.
func (Task) Edges() []ent.Edge {
	return nil
}

func (Task) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}

func (Task) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "ai_task",
		},
	}
}
