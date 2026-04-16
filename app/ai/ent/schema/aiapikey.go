package schema

import (
	"entmodule"
	mschema "entmodule/ent/schema"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
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
			Comment("名称").
			Annotations(entproto.Field(2)),
		field.String("api_key").
			MaxLen(255).
			Comment("API 密钥").
			Annotations(entproto.Field(3)),
		field.String("platform").
			MaxLen(255).
			Comment("平台").
			Annotations(entproto.Field(4)),
		field.String("url").
			MaxLen(255).
			Default("").
			Comment("custom api url").
			Annotations(entproto.Field(5)),
		field.Enum("status").
			GoType(entmodule.Status("")).
			Comment("状态").
			Annotations(entproto.Field(6), entproto.Enum(entmodule.StatusProtoValues)),
	}
}

// Edges of the AiApiKey.
func (AiApiKey) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("ai_model", AiModel.Type).
			Annotations(entproto.Field(81)),
	}
}

func (AiApiKey) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mschema.CommonMixin{},
	}
}

func (AiApiKey) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "AiApiKey"},
		entproto.Message(),
	}
}
