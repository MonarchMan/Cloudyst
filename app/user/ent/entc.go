//go:build ignore

package main

import (
	"log"

	"entgo.io/contrib/entproto"
	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

func main() {
	ex, err := entproto.NewExtension()
	if err != nil {
		log.Fatalf("creating entproto extension: %v", err)
	}
	if err := entc.Generate("./schema", &gen.Config{
		Features: []gen.Feature{
			gen.FeatureIntercept,
			gen.FeatureSnapshot,
			gen.FeatureUpsert,
			gen.FeatureExecQuery,
		},
		Templates: []*gen.Template{
			gen.MustParse(gen.NewTemplate("edge_helper").ParseFiles("templates/edgehelper.tmpl")),
			gen.MustParse(gen.NewTemplate("mutation_helper").ParseFiles("templates/mutationhelper.tmpl")),
			gen.MustParse(gen.NewTemplate("create_helper").ParseFiles("templates/createhelper.tmpl")),
		},
	}, entc.Extensions(ex)); err != nil {
		log.Fatal("running ent codegen:", err)
	}
}
