package schema

import (
	"entmodule"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// AiApiKey holds the schema definition for the AiApiKey entity.
type AiApiKey struct {
	ent.Schema
}

// Fields of the AiApiKey.
func (AiApiKey) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(255).
			Comment("名称"),
		field.String("api_key").
			MaxLen(255).
			Comment("API 密钥"),
		field.String("platform").
			MaxLen(255).
			Comment("平台"),
		field.String("url").
			MaxLen(255).
			Default("").
			Comment("custom api url"),
		field.Enum("status").
			GoType(entmodule.Status("")).
			Comment("状态"),
	}
}

// Edges of the AiApiKey.
func (AiApiKey) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("ai_model", AiModel.Type),
	}
}

func (AiApiKey) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}
