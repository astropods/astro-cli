package evaluator

const responseSchemaName = "eval_dataset_evaluator_result"

// responseFormat builds the strict response schema for one evaluator's declared
// output. Range and length bounds are deliberately absent: structured output does
// not support them, and ValidateValue has to enforce the stored value anyway.
func responseFormat(output Output) map[string]any {
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   responseSchemaName,
			"strict": true,
			"schema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"value":      valueSchema(output),
					"confidence": map[string]any{"type": "number"},
					"explanation": map[string]any{
						"type":        "string",
						"description": "One complete sentence of at most 220 characters; aim for 120 to 180 characters.",
					},
				},
				"required": []string{"value", "confidence", "explanation"},
			},
		},
	}
}

func valueSchema(output Output) map[string]any {
	switch output.Type {
	case OutputBoolean:
		return map[string]any{"type": "boolean"}
	case OutputEnum:
		return map[string]any{"type": "string", "enum": append([]string(nil), output.Options...)}
	case OutputNumber:
		return map[string]any{"type": "number"}
	default:
		return map[string]any{"type": "string"}
	}
}
