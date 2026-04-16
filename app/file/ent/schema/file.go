package schema

import (
	pb "api/api/file/common/v1"
	"context"
	"file/ent/hook"
	"file/internal/data/types"
	"time"

	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"google.golang.org/protobuf/types/descriptorpb"
)

// File holds the schema definition for the File entity.
type File struct {
	ent.Schema
}

// Fields of the File.
func (File) Fields() []ent.Field {
	return []ent.Field{
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{
				dialect.MySQL: "datetime",
			}).
			Annotations(entproto.Field(2)),
		field.Time("updated_at").
			Default(time.Now).
			SchemaType(map[string]string{
				dialect.MySQL: "datetime",
			}).
			Annotations(entproto.Field(3)),
		field.Int("type").
			Annotations(entproto.Field(4)),
		field.String("name").
			Annotations(entproto.Field(5)),
		field.Int("owner_id").
			Comment("files's owner id").
			Annotations(entproto.Field(6)),
		field.JSON("owner_info", &pb.UserInfo{}).
			Default(&pb.UserInfo{}).
			Optional().
			Annotations(entproto.Field(7, entproto.Type(descriptorpb.FieldDescriptorProto_TYPE_STRING))),
		field.Int64("size").
			Default(0).
			Annotations(entproto.Field(8)),
		field.Int("primary_entity").
			Optional().
			Annotations(entproto.Field(9)),
		field.Int("file_parent_id").
			Optional().
			Annotations(entproto.Field(10)),
		field.Bool("is_symbolic").
			Default(false).
			Annotations(entproto.Field(11)),
		field.JSON("props", &types.FileProps{}).
			Optional().
			Annotations(entproto.Field(12, entproto.Type(descriptorpb.FieldDescriptorProto_TYPE_STRING))),
		field.Int("storage_policy_files").
			Optional().
			Annotations(entproto.Field(13)),
	}
}

// Edges of the File.
func (File) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("storage_policies", StoragePolicy.Type).
			Ref("files").
			Field("storage_policy_files").
			Unique().
			Annotations(entproto.Field(81)),
		edge.To("children", File.Type).
			Annotations(entproto.Field(82)).
			From("parent").
			Field("file_parent_id").
			Unique().
			Annotations(entproto.Field(83)),
		edge.To("metadata", Metadata.Type).
			Annotations(entproto.Field(84)),
		edge.To("entities", Entity.Type).
			Annotations(entproto.Field(85)),
		edge.To("shares", Share.Type).
			Annotations(entproto.Field(86)),
		edge.To("direct_links", DirectLink.Type).
			Annotations(entproto.Field(87)),
	}
}

// Indexes of the File.
func (File) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("file_parent_id", "name").
			Unique(),
		index.Fields("file_parent_id", "type", "updated_at"),
		index.Fields("file_parent_id", "type", "size"),
	}
}

func (f File) Hooks() []ent.Hook {
	return []ent.Hook{
		hook.On(func(next ent.Mutator) ent.Mutator {
			return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
				if s, ok := m.(interface{ SetUpdatedAt(time.Time) }); ok {
					_, set := m.Field("updated_at")
					if !set {
						s.SetUpdatedAt(time.Now())
					}
				}
				v, err := next.Mutate(ctx, m)
				return v, err
			})
		}, ent.OpUpdate|ent.OpUpdateOne),
	}
}

func (File) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entproto.Message(),
		entproto.Field(1),
	}
}
