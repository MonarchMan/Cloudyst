package schema

import (
	mschema "entmodule/ent/schema"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema"
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
			Comment("会话ID").
			Annotations(entproto.Field(2)),
		field.Int("user_id").
			Comment("用户ID").
			Annotations(entproto.Field(3)),
		field.Int("role_id").
			Comment("角色ID").
			Annotations(entproto.Field(4)),
		field.Int("model_id").
			Comment("模型ID").
			Annotations(entproto.Field(5)),
		field.String("model").
			MaxLen(32).
			Comment("模型标识").
			Annotations(entproto.Field(6)),
		field.String("type").
			MaxLen(16).
			Comment("消息类型").
			Annotations(entproto.Field(7)),
		field.Int("reply_id").
			Comment("回复ID").
			Annotations(entproto.Field(8)),
		field.String("content").
			MaxLen(2048).
			Comment("消息内容").
			Annotations(entproto.Field(9)),
		field.String("reason_content").
			Default("").
			MaxLen(2048).
			Comment("推理内容").
			Annotations(entproto.Field(10)),
		field.Bool("use_context").
			Comment("是否携带上下文").
			Annotations(entproto.Field(11)),
		field.Ints("segment_ids").
			Comment("关联段落ID列表").
			Annotations(entproto.Field(12)),
		field.Strings("attachment_urls").
			Comment("关联文件URL列表").
			Annotations(entproto.Field(13)),
	}
}

// Edges of the AiChatMessage.
func (AiChatMessage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("ai_web_page", AiWebPage.Type).
			Annotations(entproto.Field(81)),
	}
}

func (AiChatMessage) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mschema.CommonMixin{},
	}
}

func (AiChatMessage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
