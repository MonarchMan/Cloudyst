package schema

import (
	mschema "entmodule/ent/schema"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema"
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
			Comment("网站名称").
			Annotations(entproto.Field(2)),
		field.String("icon").
			Comment("网站图标").
			Annotations(entproto.Field(3)),
		field.String("title").
			Comment("网页标题").
			Annotations(entproto.Field(4)),
		field.String("url").
			Comment("网页URL").
			Annotations(entproto.Field(5)),
		field.String("snippet").
			Comment("网页描述").
			Annotations(entproto.Field(6)),
		field.Text("summary").
			Comment("网页摘要").
			Annotations(entproto.Field(7)),
		field.Int("message_id").
			Comment("消息ID").
			Annotations(entproto.Field(8)),
	}
}

// Edges of the AiWebPage.
func (AiWebPage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("ai_chat_message", AiChatMessage.Type).
			Ref("ai_web_page").
			Field("message_id").
			Required().
			Unique().
			Annotations(entproto.Field(81)),
	}
}

func (AiWebPage) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mschema.CommonMixin{},
	}
}

func (AiWebPage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
