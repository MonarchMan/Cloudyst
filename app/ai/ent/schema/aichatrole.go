package schema

import (
	"entmodule"

	"entgo.io/ent"
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
			Comment("角色名称"),
		field.String("avatar").
			MaxLen(256).
			Comment("头像"),
		field.String("description").
			MaxLen(256).
			Comment("角色描述"),
		field.Int("sort").
			Comment("排序"),
		field.Int("user_id").
			Comment("用户ID"),
		field.Bool("public_status").
			Comment("是否公开"),
		field.String("category").
			MaxLen(32).
			Comment("角色类别"),
		field.String("system_message").
			MaxLen(1024).
			Comment("角色上下文"),
		field.Ints("knowledge_ids").
			Comment("关联知识库ID列表"),
		field.Ints("tool_ids").
			Comment("关联工具ID列表"),
		field.Strings("mcp_client_names").
			Comment("关联MCP客户端名称列表"),
		field.Enum("status").
			GoType(entmodule.Status("")).
			Comment("状态"),
	}
}

// Edges of the AiChatRole.
func (AiChatRole) Edges() []ent.Edge {
	return nil
}

func (AiChatRole) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}
