package schema

import (
	"entmodule"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// AiKnowledgeSegment holds the schema definition for the AiKnowledgeSegment entity.
type AiKnowledgeSegment struct {
	ent.Schema
}

// Fields of the AiKnowledgeSegment.
func (AiKnowledgeSegment) Fields() []ent.Field {
	return []ent.Field{
		field.Int("document_id").
			Comment("文档ID"),
		field.Int("knowledge_id").
			Comment("知识库ID"),
		field.Int("content_length").
			Comment("分段内容长度"),
		field.Int("tokens").
			Comment("分段token数"),
		field.Int("chunk_index").
			Default(0).
			Comment("文档内切片顺序"),
		field.String("section_path").
			Optional().
			Default("").
			Comment("章节路径"),
		field.Int("start_offset").
			Default(0).
			Comment("切片在原文中的起始字符偏移"),
		field.Int("end_offset").
			Default(0).
			Comment("切片在原文中的结束字符偏移"),
		field.JSON("metadata", map[string]any{}).
			Optional().
			Comment("切片级增强元信息"),
		field.String("vector_id").
			MaxLen(100).
			Default("").
			Comment("向量库的分片向量ID"),
		field.Int("retrieval_count").
			Comment("召回次数"),
		field.Enum("status").
			GoType(entmodule.Status("")).
			Comment("状态"),
	}
}

// Edges of the AiKnowledgeSegment.
func (AiKnowledgeSegment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("ai_knowledge_document", AiKnowledgeDocument.Type).
			Ref("ai_knowledge_segment").
			Field("document_id").
			Required().
			Unique(),
	}
}

func (AiKnowledgeSegment) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}
