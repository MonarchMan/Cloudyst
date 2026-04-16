package schema

import (
	mschema "entmodule/ent/schema"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Metadata holds the schema definition for the Metadata entity.
type Metadata struct {
	ent.Schema
}

// Fields of the Metadata.
func (Metadata) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Annotations(entproto.Field(2)),
		field.Text("value").
			Annotations(entproto.Field(3)),
		field.Int("file_id").
			Annotations(entproto.Field(4)),
		field.Bool("is_public").
			Default(false).
			Annotations(entproto.Field(5)),
	}
}

// Edges of the Metadata.
func (Metadata) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("file", File.Type).
			Ref("metadata").
			Field("file_id").
			Required().
			Unique().
			Annotations(entproto.Field(81)),
	}
}

func (Metadata) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("file_id", "name").
			Unique(),
	}
}

func (Metadata) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mschema.CommonMixin{},
	}
}

func (Metadata) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
