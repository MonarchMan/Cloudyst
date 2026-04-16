package schema

import (
	"ai/internal/biz/types"
	mschema "entmodule/ent/schema"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"google.golang.org/protobuf/types/descriptorpb"
)

// AiImage holds the schema definition for the AiImage entity.
type AiImage struct {
	ent.Schema
}

// Fields of the AiImage.
func (AiImage) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id").
			Comment("用户ID").
			Annotations(entproto.Field(2)),
		field.String("platform").
			MaxLen(64).
			Comment("平台").
			Annotations(entproto.Field(3)),
		field.Int("model_id").
			Comment("模型ID").
			Annotations(entproto.Field(4)),
		field.String("model").
			MaxLen(32).
			Comment("模型标识").
			Annotations(entproto.Field(5)),
		field.String("prompt").
			MaxLen(2048).
			Comment("提示").
			Annotations(entproto.Field(6)),
		field.Int("width").
			Comment("宽度").
			Annotations(entproto.Field(7)),
		field.Int("height").
			Comment("高度").
			Annotations(entproto.Field(8)),
		field.JSON("options", map[string]any{}).
			Comment("绘制参数").
			Annotations(entproto.Field(9, entproto.Type(descriptorpb.FieldDescriptorProto_TYPE_STRING))),
		field.Enum("status").
			GoType(types.ImageStatus("")).
			Comment("状态").
			Annotations(entproto.Field(10), entproto.Enum(types.ImageStatusProtoValues)),
		field.String("pic_url").
			MaxLen(2048).
			Comment("图片URL").
			Annotations(entproto.Field(11)),
		field.String("task_id").
			Comment("任务编号").
			Annotations(entproto.Field(12)),
		field.String("buttons").
			MaxLen(2048).
			Comment("mj 按钮").
			Annotations(entproto.Field(13)),
	}
}

// Edges of the AiImage.
func (AiImage) Edges() []ent.Edge {
	return nil
}

func (AiImage) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mschema.CommonMixin{},
	}
}

func (AiImage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
