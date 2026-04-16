//go:build ignore

package main

import (
	"entmodule"
	"log"

	"entgo.io/ent/entc"
)

func main() {
	if err := entc.Generate("./schema", entmodule.GenConfig()); err != nil {
		log.Fatal("running ent codegen:", err)
	}
}
