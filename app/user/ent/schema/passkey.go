package schema

import (
	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/go-webauthn/webauthn/webauthn"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Passkey holds the schema definition for the Passkey entity.
type Passkey struct {
	ent.Schema
}

// Fields of the Passkey.
func (Passkey) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id").
			Annotations(entproto.Field(2)),
		field.String("credential_id").
			Annotations(entproto.Field(3)),
		field.String("name").
			Annotations(entproto.Field(4)),
		field.JSON("credential", &webauthn.Credential{}).
			Sensitive().
			Annotations(entproto.Field(5, entproto.Type(descriptorpb.FieldDescriptorProto_TYPE_STRING))),
		field.Time("used_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{
				dialect.MySQL: "datetime",
			}).
			Annotations(entproto.Field(6)),
	}
}

// Edges of the Passkey.
func (Passkey) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("users", User.Type).
			Field("user_id").
			Ref("passkey").
			Unique().
			Required().
			Annotations(entproto.Field(80)),
	}
}

func (Passkey) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}

func (Passkey) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "credential_id").Unique(),
	}
}

func (Passkey) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
	}
}
