package schema

import (
	mschema "entmodule/ent/schema"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// DirectLink holds the schema definition for the DirectLink entity.
type DirectLink struct {
	ent.Schema
}

// Fields of the DirectLink.
func (DirectLink) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Annotations(entproto.Field(2)),
		field.Int("downloads").
			Annotations(entproto.Field(3)),
		field.Int("file_id").
			Annotations(entproto.Field(4)),
		field.Int("speed").
			Annotations(entproto.Field(5)),
	}
}

// Edges of the DirectLink.
func (DirectLink) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("file", File.Type).
			Ref("direct_links").
			Field("file_id").
			Required().
			Unique().
			Annotations(entproto.Field(80)),
	}
}

func (DirectLink) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mschema.CommonMixin{},
	}
}

func (DirectLink) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
