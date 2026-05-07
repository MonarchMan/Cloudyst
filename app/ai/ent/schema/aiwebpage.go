package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// AiWebPage holds the schema definition for the AiWebPage entity.
type AiWebPage struct {
	ent.Schema
}

// Fields of the AiWebPage.
func (AiWebPage) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Comment("网站名称"),
		field.String("icon").
			Comment("网站图标"),
		field.String("title").
			Comment("网页标题"),
		field.String("url").
			Comment("网页URL"),
		field.String("snippet").
			Comment("网页描述"),
		field.Text("summary").
			Comment("网页摘要"),
		field.Int("message_id").
			Comment("消息ID"),
	}
}

// Edges of the AiWebPage.
func (AiWebPage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("ai_chat_message", AiChatMessage.Type).
			Ref("ai_web_page").
			Field("message_id").
			Required().
			Unique(),
	}
}

func (AiWebPage) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}
