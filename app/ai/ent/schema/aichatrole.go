package schema

import (
	"entmodule"
	mschema "entmodule/ent/schema"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// AiChatRole holds the schema definition for the AiChatRole entity.
type AiChatRole struct {
	ent.Schema
}

// Fields of the AiChatRole.
func (AiChatRole) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(128).
			Comment("角色名称").
			Annotations(entproto.Field(2)),
		field.String("avatar").
			MaxLen(256).
			Comment("头像").
			Annotations(entproto.Field(3)),
		field.String("description").
			MaxLen(256).
			Comment("角色描述").
			Annotations(entproto.Field(4)),
		field.Int("sort").
			Comment("排序").
			Annotations(entproto.Field(5)),
		field.Int("user_id").
			Comment("用户ID").
			Annotations(entproto.Field(6)),
		field.Bool("public_status").
			Comment("是否公开").
			Annotations(entproto.Field(7)),
		field.String("category").
			MaxLen(32).
			Comment("角色类别").
			Annotations(entproto.Field(8)),
		field.String("system_message").
			MaxLen(1024).
			Comment("角色上下文").
			Annotations(entproto.Field(9)),
		field.Ints("knowledge_ids").
			Comment("关联知识库ID列表").
			Annotations(entproto.Field(10)),
		field.Ints("tool_ids").
			Comment("关联工具ID列表").
			Annotations(entproto.Field(11)),
		field.Strings("mcp_client_names").
			Comment("关联MCP客户端名称列表").
			Annotations(entproto.Field(12)),
		field.Enum("status").
			GoType(entmodule.Status("")).
			Comment("状态").
			Annotations(entproto.Field(13), entproto.Enum(entmodule.StatusProtoValues)),
	}
}

// Edges of the AiChatRole.
func (AiChatRole) Edges() []ent.Edge {
	return nil
}

func (AiChatRole) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mschema.CommonMixin{},
	}
}

func (AiChatRole) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
