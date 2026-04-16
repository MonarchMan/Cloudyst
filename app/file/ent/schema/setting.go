package schema

import (
	mschema "entmodule/ent/schema"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// Setting holds the schema definition for the Setting entity.
type Setting struct {
	ent.Schema
}

// Fields of the Setting.
func (Setting) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Unique().
			Annotations(entproto.Field(2)),
		field.Text("value").
			Optional().
			Annotations(entproto.Field(3)),
	}
}

// Edges of the Setting.
func (Setting) Edges() []ent.Edge {
	return nil
}

func (Setting) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mschema.CommonMixin{},
	}
}

func (Setting) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
