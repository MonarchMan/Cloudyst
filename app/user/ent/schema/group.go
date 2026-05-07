package schema

import (
	"api/external/data/filedata"
	"api/external/data/userdata"
	"common/boolset"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Group holds the schema definition for the Group entity.
type Group struct {
	ent.Schema
}

// Fields of the Group.
func (Group) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
		field.Int64("max_storage").
			Optional(),
		field.Int("speed_limit").
			Optional(),
		field.Bytes("permissions").GoType(&boolset.BooleanSet{}),
		field.JSON("settings", &userdata.GroupSetting{}).
			Default(&userdata.GroupSetting{}).
			Optional(),
		field.Int("storage_policy_id").Optional(),
		field.JSON("storage_policy_info", &filedata.StoragePolicyInfo{}).
			Default(&filedata.StoragePolicyInfo{}).
			Optional(),
	}
}

// Edges of the Group.
func (Group) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("users", User.Type),
	}
}

func (Group) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}
