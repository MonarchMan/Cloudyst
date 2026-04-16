//go:build ignore

package main

import (
	"entmodule"
	"log"

	"entgo.io/contrib/entproto"
	"entgo.io/ent/entc"
)

func main() {
	ex, err := entproto.NewExtension()
	if err != nil {
		log.Fatalf("creating entproto extension: %v", err)
	}
	if err := entc.Generate("./schema", entmodule.GenConfig(), entc.Extensions(ex)); err != nil {
		log.Fatal("running ent codegen:", err)
	}
}
