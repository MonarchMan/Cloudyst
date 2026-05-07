package schema

import (
	"entmodule"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// AiTool holds the schema definition for the AiTool entity.
type AiTool struct {
	ent.Schema
}

// Fields of the AiTool.
func (AiTool) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Comment("工具名称"),
		field.String("description").
			Comment("工具描述"),
		field.String("type").
			Comment("工具类型"),
		field.String("parameters").
			Comment("工具参数"),
		field.Enum("status").
			GoType(entmodule.Status("")).
			Comment("状态"),
	}
}

// Edges of the AiTool.
func (AiTool) Edges() []ent.Edge {
	return nil
}

func (AiTool) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}
