package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// AiChatMessage holds the schema definition for the AiChatMessage entity.
type AiChatMessage struct {
	ent.Schema
}

// Fields of the AiChatMessage.
func (AiChatMessage) Fields() []ent.Field {
	return []ent.Field{
		field.Int("conversation_id").
			Comment("会话ID"),
		field.Int("user_id").
			Comment("用户ID"),
		field.Int("role_id").
			Comment("角色ID"),
		field.Int("model_id").
			Comment("模型ID"),
		field.String("model").
			MaxLen(32).
			Comment("模型标识"),
		field.String("type").
			MaxLen(16).
			Comment("消息类型"),
		field.Int("reply_id").
			Comment("回复ID"),
		field.String("content").
			MaxLen(2048).
			Comment("消息内容"),
		field.String("reason_content").
			Default("").
			MaxLen(2048).
			Comment("推理内容"),
		field.Bool("use_context").
			Comment("是否携带上下文"),
		field.Ints("segment_ids").
			Comment("关联段落ID列表"),
		field.Strings("attachment_urls").
			Comment("关联文件URL列表"),
	}
}

// Edges of the AiChatMessage.
func (AiChatMessage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("ai_web_page", AiWebPage.Type),
	}
}

func (AiChatMessage) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}
