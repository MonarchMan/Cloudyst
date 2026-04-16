package schema

import (
	"ai/internal/biz/types"
	"entmodule"
	mschema "entmodule/ent/schema"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema"
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
		field.Int("tokens").
			Comment("文档token数").
			Annotations(entproto.Field(6)),
		field.Int("segment_max_tokens").
			Comment("分片最大Token数"),
		field.Int("retrieval_count").
			Comment("召回次数"),
		field.Enum("process").
			GoType(types.DocumentStatus("")).
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
			Unique().
			Annotations(entproto.Field(81)),
		edge.To("ai_knowledge_segment", AiKnowledgeSegment.Type).
			Annotations(entproto.Field(82)),
	}
}

func (AiKnowledgeDocument) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mschema.CommonMixin{},
	}
}

func (AiKnowledgeDocument) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
