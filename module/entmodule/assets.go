package entmodule

import (
	"embed"

	"entgo.io/ent/entc/gen"
)

//go:embed ent/templates/*.tmpl
var TemplatesFS embed.FS

func GenConfig() *gen.Config {
	return &gen.Config{
		Features: []gen.Feature{
			gen.FeatureIntercept,
			gen.FeatureSnapshot,
			gen.FeatureUpsert,
			gen.FeatureExecQuery,
		},
		Templates: []*gen.Template{
			gen.MustParse(gen.NewTemplate("edge_helper").ParseFS(TemplatesFS, "ent/templates/edgehelper.tmpl")),
			gen.MustParse(gen.NewTemplate("mutation_helper").ParseFS(TemplatesFS, "ent/templates/mutationhelper.tmpl")),
			gen.MustParse(gen.NewTemplate("create_helper").ParseFS(TemplatesFS, "ent/templates/createhelper.tmpl")),
		},
	}
}
