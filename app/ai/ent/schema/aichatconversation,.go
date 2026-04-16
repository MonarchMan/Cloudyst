package schema

import (
	mschema "entmodule/ent/schema"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// AiChatConversation holds the schema definition for the AiChatConversation entity.
type AiChatConversation struct {
	ent.Schema
}

// Fields of the AiChatConversation.
func (AiChatConversation) Fields() []ent.Field {
	return []ent.Field{
		field.String("title").
			MaxLen(256).
			Comment("会话标题").
			Annotations(entproto.Field(2)),
		field.Bool("pinned").
			Comment("是否置顶会话").
			Annotations(entproto.Field(3)),
		field.Int("user_id").
			Comment("用户ID").
			Annotations(entproto.Field(4)),
		field.Int("role_id").
			Comment("角色ID").
			Annotations(entproto.Field(5)),
		field.String("system_message").
			MaxLen(1024).
			Default("").
			Comment("角色设定").
			Annotations(entproto.Field(6)),
		field.Int("model_id").
			Comment("模型ID").
			Annotations(entproto.Field(7)),
		field.String("model").
			MaxLen(32).
			Comment("模型标识").
			Annotations(entproto.Field(8)),
		field.Float("temperature").
			Comment("温度").
			Annotations(entproto.Field(9)),
		field.Int("max_tokens").
			Comment("单条回复的最大Token数").
			Annotations(entproto.Field(10)),
		field.Int("max_contexts").
			Comment("上下文的最大Messages数").
			Annotations(entproto.Field(11)),
	}
}

// Edges of the AiChatConversation.
func (AiChatConversation) Edges() []ent.Edge {
	return nil
}

func (AiChatConversation) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mschema.CommonMixin{},
	}
}

func (AiChatConversation) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
