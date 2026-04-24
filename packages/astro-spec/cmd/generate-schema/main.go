// Command generate-schema reflects spec.AstroSpec into a JSON Schema
// and writes it to package.schema.json in the package root.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	spec "github.com/astropods/astro/packages/astro-spec"
	"github.com/invopop/jsonschema"
)

func main() {
	r := &jsonschema.Reflector{
		DoNotReference: true,
	}
	schema := r.Reflect(&spec.AstroSpec{})
	// FIXME: This should be set at build time via ldflags
	schema.ID = "https://astropods.com/schema/package.json"
	schema.Title = "Astro AI Spec"
	schema.Description = "Schema for Astro AI agent specification"

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		panic(err)
	}

	// Write to package root (two levels up from cmd/generate-schema/)
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller failed")
	}
	pkgRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	outPath := filepath.Join(pkgRoot, "astropods.schema.json")

	if err := os.WriteFile(outPath, append(data, '\n'), 0600); err != nil {
		panic(err)
	}
}
