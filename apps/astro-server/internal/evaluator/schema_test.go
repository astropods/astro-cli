package evaluator

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func schemaProperties(t *testing.T, output Output) map[string]any {
	t.Helper()
	format := responseFormat(output)
	jsonSchema, ok := format["json_schema"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "json_schema", format["type"])
	assert.Equal(t, responseSchemaName, jsonSchema["name"])
	assert.Equal(t, true, jsonSchema["strict"])

	schema, ok := jsonSchema["schema"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "object", schema["type"])
	assert.Equal(t, false, schema["additionalProperties"])
	assert.Equal(t, []string{"value", "confidence", "explanation"}, schema["required"])

	properties, ok := schema["properties"].(map[string]any)
	require.True(t, ok)
	return properties
}

func valueSchemaFor(t *testing.T, output Output) map[string]any {
	t.Helper()
	value, ok := schemaProperties(t, output)["value"].(map[string]any)
	require.True(t, ok)
	return value
}

func TestResponseFormatValueSchemaPerOutputType(t *testing.T) {
	assert.Equal(t, map[string]any{"type": "boolean"}, valueSchemaFor(t, Output{Type: OutputBoolean}))
	assert.Equal(t, map[string]any{"type": "number"}, valueSchemaFor(t, Output{Type: OutputNumber}))
	assert.Equal(t, map[string]any{"type": "string"}, valueSchemaFor(t, Output{Type: OutputString}))
}

func TestResponseFormatEnumPreservesOptionOrder(t *testing.T) {
	options := []string{"positive", "neutral", "negative", "unclear"}
	value := valueSchemaFor(t, Output{Type: OutputEnum, Options: options})

	assert.Equal(t, "string", value["type"])
	assert.Equal(t, options, value["enum"])
}

func TestResponseFormatEnumCopiesOptions(t *testing.T) {
	options := []string{"positive", "negative"}
	value := valueSchemaFor(t, Output{Type: OutputEnum, Options: options})

	options[0] = "mutated"
	assert.Equal(t, []string{"positive", "negative"}, value["enum"])
}

func TestResponseFormatOmitsRangeAndLengthKeywords(t *testing.T) {
	cases := map[string]Output{
		"number bounded": {Type: OutputNumber, Minimum: float64Ptr(0), Maximum: float64Ptr(1)},
		"string limited": {Type: OutputString, MaxLength: intPtr(500)},
	}

	for name, output := range cases {
		t.Run(name, func(t *testing.T) {
			value := valueSchemaFor(t, output)
			for _, keyword := range []string{"minimum", "maximum", "maxLength", "max_length"} {
				assert.NotContains(t, value, keyword)
			}
		})
	}
}

func TestResponseFormatConfidenceAndExplanationAreFixed(t *testing.T) {
	properties := schemaProperties(t, Output{Type: OutputBoolean})

	assert.Equal(t, map[string]any{"type": "number"}, properties["confidence"])

	explanation, ok := properties["explanation"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "string", explanation["type"])
	assert.NotEmpty(t, explanation["description"])
}

func TestResponseFormatMarshalsToJSON(t *testing.T) {
	raw, err := json.Marshal(responseFormat(Output{Type: OutputEnum, Options: []string{"a", "b"}}))
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"enum":["a","b"]`)
}
