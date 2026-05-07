package schema

import (
	"entgo.io/ent"
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
			Comment("会话标题"),
		field.Bool("pinned").
			Comment("是否置顶会话"),
		field.Int("user_id").
			Comment("用户ID"),
		field.Int("role_id").
			Comment("角色ID"),
		field.String("system_message").
			MaxLen(1024).
			Default("").
			Comment("角色设定"),
		field.Int("model_id").
			Comment("模型ID"),
		field.String("model").
			MaxLen(32).
			Comment("模型标识"),
		field.Float("temperature").
			Comment("温度"),
		field.Int("max_tokens").
			Comment("单条回复的最大Token数"),
		field.Int("max_contexts").
			Comment("上下文的最大Messages数"),
	}
}

// Edges of the AiChatConversation.
func (AiChatConversation) Edges() []ent.Edge {
	return nil
}

func (AiChatConversation) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}
