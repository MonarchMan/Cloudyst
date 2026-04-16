package schema

import (
	"entmodule"
	mschema "entmodule/ent/schema"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema"
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
			Comment("工具名称").
			Annotations(entproto.Field(2)),
		field.String("description").
			Comment("工具描述").
			Annotations(entproto.Field(3)),
		field.String("type").
			Comment("工具类型").
			Annotations(entproto.Field(4)),
		field.String("parameters").
			Comment("工具参数").
			Annotations(entproto.Field(5)),
		field.Enum("status").
			GoType(entmodule.Status("")).
			Comment("状态").
			Annotations(entproto.Field(6), entproto.Enum(entmodule.StatusProtoValues)),
	}
}

// Edges of the AiTool.
func (AiTool) Edges() []ent.Edge {
	return nil
}

func (AiTool) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mschema.CommonMixin{},
	}
}

func (AiTool) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
