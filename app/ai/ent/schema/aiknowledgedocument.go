package schema

import (
	"ai/internal/biz/types"
	"entmodule"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// AiKnowledgeDocument holds the schema definition for the AiKnowledgeDocument entity.
type AiKnowledgeDocument struct {
	ent.Schema
}

// Fields of the AiKnowledgeDocument.
func (AiKnowledgeDocument) Fields() []ent.Field {
	return []ent.Field{
		field.Int("knowledge_id").
			Comment("知识库ID"),
		field.String("name").
			MaxLen(255).
			Comment("文档名称"),
		field.String("url").
			Comment("文档URL"),
		field.String("version").
			Comment("文档版本(文件系统里指 entity id)"),
		field.Int("content_length").
			Comment("文档内容长度"),
		field.Int64("size").
			Comment("文档大小，单位字节"),
		field.Int("tokens").
			Comment("文档token数"),
		field.Int("chunks").
			Default(0).
			Comment("文档切片总数"),
		field.String("parse_type").
			Optional().
			Default("").
			Comment("解析类型，如 markdown/pdf/html/docx/txt"),
		field.String("content_hash").
			Optional().
			Default("").
			Comment("文档内容hash，用于判断是否需要重建索引"),
		field.JSON("metadata", map[string]any{}).
			Optional().
			Comment("文档级增强元信息，如标题、摘要、术语、语言、质量评分"),
		field.Int("segment_max_tokens").
			Comment("分片最大Token数"),
		field.Int("retrieval_count").
			Comment("召回次数"),
		field.Enum("progress").
			GoType(types.DocumentProgress("")).
			Comment("文档处理进度"),
		field.Enum("status").
			GoType(entmodule.Status("")).
			Comment("状态"),
	}
}

// Edges of the AiKnowledgeDocument.
func (AiKnowledgeDocument) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("ai_knowledge", AiKnowledge.Type).
			Ref("ai_knowledge_document").
			Field("knowledge_id").
			Required().
			Unique(),
		edge.To("ai_knowledge_segment", AiKnowledgeSegment.Type),
	}
}

func (AiKnowledgeDocument) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}
