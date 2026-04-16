package schema

import (
	"entmodule"
	mschema "entmodule/ent/schema"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// AiModel holds the schema definition for the AiModel entity.
type AiModel struct {
	ent.Schema
}

// Fields of the AiModel.
func (AiModel) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(64).
			Comment("模型名称").
			Annotations(entproto.Field(2)),
		field.String("type").
			MaxLen(64).
			Comment("模型类型").
			Annotations(entproto.Field(3)),
		field.String("platform").
			MaxLen(64).
			Comment("平台").
			Annotations(entproto.Field(4)),
		field.Int("sort").
			Comment("排序").
			Annotations(entproto.Field(5)),
		field.Enum("status").
			GoType(entmodule.Status("")).
			Comment("状态").
			Annotations(entproto.Field(6), entproto.Enum(entmodule.StatusProtoValues)),
		field.Float("temperature").
			Comment("温度").
			Annotations(entproto.Field(7)),
		field.Int("max_tokens").
			Comment("最大token数").
			Annotations(entproto.Field(8)),
		field.Int("max_context").
			Comment("最大上下文数").
			Annotations(entproto.Field(9)),
		field.Int("key_id").
			Comment("API Key ID").
			Annotations(entproto.Field(10)),
	}
}

// Edges of the AiModel.
func (AiModel) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("ai_api_key", AiApiKey.Type).
			Ref("ai_model").
			Field("key_id").
			Required().
			Unique().
			Annotations(entproto.Field(81)),
	}
}

func (AiModel) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mschema.CommonMixin{},
	}
}

func (AiModel) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
