// Command generate-schema reflects spec.AstroSpec into a JSON Schema
// and writes it to astro.schema.json in the package root.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	"github.com/invopop/jsonschema"
	spec "github.com/postman/astro/packages/astro-spec"
)

func main() {
	r := &jsonschema.Reflector{
		DoNotReference: true,
	}
	schema := r.Reflect(&spec.AstroSpec{})
	schema.ID = "https://astro.postman.com/schema/astro.json"
	schema.Title = "Astro Spec"
	schema.Description = "Schema for astro.yml agent specification"

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		panic(err)
	}

	// Write to package root (two levels up from cmd/generate-schema/)
	_, thisFile, _, _ := runtime.Caller(0)
	pkgRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	outPath := filepath.Join(pkgRoot, "astro.schema.json")

	if err := os.WriteFile(outPath, append(data, '\n'), 0644); err != nil {
		panic(err)
	}
}
