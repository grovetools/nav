package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/invopop/jsonschema"
	"github.com/grovetools/nav/internal/manager"
)

func main() {
	r := &jsonschema.Reflector{
		AllowAdditionalProperties: true,
		ExpandedStruct:            true,
		FieldNameTag:              "yaml",
	}

	schema := r.Reflect(&manager.TmuxConfig{})
	schema.Title = "Grove Nav Configuration"
	schema.Description = "Schema for the 'nav' extension in grove.toml. For backwards compatibility, 'tmux' is also supported."

	// Make all fields optional - Grove configs should not require any fields
	schema.Required = nil

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		log.Fatalf("Error marshaling schema: %v", err)
	}

	// Write to the package root
	if err := os.WriteFile("nav.schema.json", data, 0644); err != nil {
		log.Fatalf("Error writing schema file: %v", err)
	}

	log.Printf("Successfully generated nav schema at nav.schema.json")
}
