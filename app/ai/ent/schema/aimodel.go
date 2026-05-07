package schema

import (
	"entmodule"

	"entgo.io/ent"
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
			Comment("模型名称"),
		field.String("type").
			MaxLen(64).
			Comment("模型类型"),
		field.String("platform").
			MaxLen(64).
			Comment("平台"),
		field.Int("sort").
			Comment("排序"),
		field.Enum("status").
			GoType(entmodule.Status("")).
			Comment("状态"),
		field.Float("temperature").
			Comment("温度"),
		field.Int("max_tokens").
			Comment("最大token数"),
		field.Int("max_context").
			Comment("最大上下文数"),
		field.Int("key_id").
			Comment("API Key ID"),
	}
}

// Edges of the AiModel.
func (AiModel) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("ai_api_key", AiApiKey.Type).
			Ref("ai_model").
			Field("key_id").
			Required().
			Unique(),
	}
}

func (AiModel) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}
