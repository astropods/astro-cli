package spec

import _ "embed"

//go:generate go run ./cmd/generate-schema

//go:embed astropods.schema.json
var astroSchema []byte

// Schema returns the embedded JSON Schema for AstroSpec.
func Schema() []byte {
	return astroSchema
}
