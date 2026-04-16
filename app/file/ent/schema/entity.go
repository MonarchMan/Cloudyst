package schema

import (
	mschema "entmodule/ent/schema"
	"file/internal/data/types"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/gofrs/uuid"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Entity holds the schema definition for the Entity entity.
type Entity struct {
	ent.Schema
}

// Fields of the Entity.
func (Entity) Fields() []ent.Field {
	return []ent.Field{
		field.Int("type").
			Annotations(entproto.Field(2)),
		field.Text("source").
			Annotations(entproto.Field(3)),
		field.Int64("size").
			Annotations(entproto.Field(4)),
		field.Int("reference_count").
			Default(1).
			Annotations(entproto.Field(5)),
		field.Int("storage_policy_entities").
			Annotations(entproto.Field(6)),
		field.Int("created_by").
			Optional().
			Annotations(entproto.Field(7)),
		field.UUID("upload_session_id", uuid.Must(uuid.NewV4())).
			Optional().
			Nillable().
			Annotations(entproto.Field(8)),
		field.JSON("props", &types.EntityProps{}).
			Optional().
			Annotations(entproto.Field(9, entproto.Type(descriptorpb.FieldDescriptorProto_TYPE_STRING))),
	}
}

// Edges of the Entity.
func (Entity) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("file", File.Type).
			Ref("entities").
			Annotations(entproto.Field(81)),
		edge.From("storage_policy", StoragePolicy.Type).
			Ref("entities").
			Field("storage_policy_entities").
			Unique().
			Required().
			Annotations(entproto.Field(82)),
	}
}

func (Entity) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mschema.CommonMixin{},
	}
}

func (Entity) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
