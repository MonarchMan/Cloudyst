package schema

import (
	"entmodule"
	mschema "entmodule/ent/schema"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// AiKnowledge holds the schema definition for the AiKnowledge entity.
type AiKnowledge struct {
	ent.Schema
}

// Fields of the AiKnowledge.
func (AiKnowledge) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(255).
			Comment("知识库名称"),
		field.Text("description").
			Comment("知识库描述"),
		field.Int("user_id").
			Comment("用户ID"),
		field.Int("embedding_model_id").
			Comment("嵌入模型ID"),
		field.String("embedding_model").
			MaxLen(32).
			Comment("嵌入模型标识"),
		field.Int("top_k").
			Comment("topK"),
		field.Float("similarity_threshold").
			Comment("相似度阈值"),
		field.Bool("is_public").
			Comment("是否公开"),
		field.Bool("is_master").
			Comment("是否主知识库(用于建立全文搜索索引)"),
		field.Enum("status").
			GoType(entmodule.Status("")).
			Comment("状态"),
	}
}

// Edges of the AiKnowledge.
func (AiKnowledge) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("ai_knowledge_document", AiKnowledgeDocument.Type),
	}
}

func (AiKnowledge) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mschema.CommonMixin{},
	}
}
