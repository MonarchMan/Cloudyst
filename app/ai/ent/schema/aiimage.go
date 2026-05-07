package schema

import (
	"ai/internal/biz/types"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// AiImage holds the schema definition for the AiImage entity.
type AiImage struct {
	ent.Schema
}

// Fields of the AiImage.
func (AiImage) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id").
			Comment("用户ID"),
		field.String("platform").
			MaxLen(64).
			Comment("平台"),
		field.Int("model_id").
			Comment("模型ID"),
		field.String("model").
			MaxLen(32).
			Comment("模型标识"),
		field.String("prompt").
			MaxLen(2048).
			Comment("提示"),
		field.Int("width").
			Comment("宽度"),
		field.Int("height").
			Comment("高度"),
		field.JSON("options", map[string]any{}).
			Comment("绘制参数"),
		field.Enum("status").
			GoType(types.ImageStatus("")).
			Comment("状态"),
		field.String("pic_url").
			MaxLen(2048).
			Comment("图片URL"),
		field.String("task_id").
			Comment("任务编号"),
		field.String("buttons").
			MaxLen(2048).
			Comment("mj 按钮"),
	}
}

// Edges of the AiImage.
func (AiImage) Edges() []ent.Edge {
	return nil
}

func (AiImage) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}
