package evaljudge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
)

const maxExplanationRunes = 240

func responseFormat() map[string]any {
	dimensions := criterionDimensionStrings()
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "eval_dataset_judgment_prediction",
			"strict": true,
			"schema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"verdict_score": map[string]any{"type": "number"},
					"confidence":    map[string]any{"type": "integer"},
					"explanation": map[string]any{
						"type":        "string",
						"description": "One complete sentence of at most 220 characters; aim for 120 to 180 characters.",
					},
					"criteria": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"properties": map[string]any{
								"dimension_key":   map[string]any{"type": "string", "enum": dimensions},
								"dimension_value": map[string]any{"type": "number"},
							},
							"required": []string{"dimension_key", "dimension_value"},
						},
					},
				},
				"required": []string{"verdict_score", "confidence", "explanation", "criteria"},
			},
		},
	}
}

type modelPrediction struct {
	VerdictScore *float64          `json:"verdict_score"`
	Confidence   *int              `json:"confidence"`
	Explanation  *string           `json:"explanation"`
	Criteria     *[]modelCriterion `json:"criteria"`
}

type modelCriterion struct {
	DimensionKey   *string  `json:"dimension_key"`
	DimensionValue *float64 `json:"dimension_value"`
}

type validatedPrediction struct {
	verdictScore    float64
	confidence      int
	explanation     string
	criterionValues map[judgmentstore.CriterionDimension]float64
}

func parsePrediction(response *aigateway.ChatCompletionResponse) (judgmentstore.Prediction, error) {
	if response == nil {
		return judgmentstore.Prediction{}, invalidOutput("missing response")
	}
	if len(response.Choices) != 1 {
		return judgmentstore.Prediction{}, invalidOutput("expected exactly one choice, got %d", len(response.Choices))
	}
	choice := response.Choices[0]
	if choice.Message.Role != "assistant" {
		return judgmentstore.Prediction{}, invalidOutput("choice role is %q, want assistant", choice.Message.Role)
	}
	if choice.FinishReason != "stop" {
		return judgmentstore.Prediction{}, invalidOutput("choice finish_reason is %q, want %q", choice.FinishReason, "stop")
	}
	content := strings.TrimSpace(choice.Message.Content)
	if content == "" {
		return judgmentstore.Prediction{}, invalidOutput("choice content is empty")
	}

	model, err := decodePrediction(content)
	if err != nil {
		return judgmentstore.Prediction{}, err
	}
	validated, err := validatePrediction(model)
	if err != nil {
		return judgmentstore.Prediction{}, err
	}
	return toStoredPrediction(validated), nil
}

func decodePrediction(content string) (modelPrediction, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(content))
	decoder.DisallowUnknownFields()
	var model modelPrediction
	if err := decoder.Decode(&model); err != nil {
		return modelPrediction{}, invalidOutput("decode: %v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return modelPrediction{}, invalidOutput("multiple JSON values")
		}
		return modelPrediction{}, invalidOutput("trailing content: %v", err)
	}
	return model, nil
}

func validatePrediction(model modelPrediction) (validatedPrediction, error) {
	if model.VerdictScore == nil || model.Confidence == nil || model.Explanation == nil || model.Criteria == nil {
		return validatedPrediction{}, invalidOutput("missing required field")
	}
	if math.IsNaN(*model.VerdictScore) || math.IsInf(*model.VerdictScore, 0) || *model.VerdictScore < -1 || *model.VerdictScore > 1 {
		return validatedPrediction{}, invalidOutput("verdict_score %v is outside [-1, 1]", *model.VerdictScore)
	}
	if *model.Confidence < 0 || *model.Confidence > 100 {
		return validatedPrediction{}, invalidOutput("confidence %d is outside [0, 100]", *model.Confidence)
	}
	explanation := strings.TrimSpace(*model.Explanation)
	if explanation == "" {
		return validatedPrediction{}, invalidOutput("explanation is empty")
	}
	if len(*model.Criteria) != len(judgmentstore.CriterionDimensions) {
		return validatedPrediction{}, invalidOutput("expected %d criteria, got %d", len(judgmentstore.CriterionDimensions), len(*model.Criteria))
	}

	values := make(map[judgmentstore.CriterionDimension]float64, len(*model.Criteria))
	for i, criterion := range *model.Criteria {
		if criterion.DimensionKey == nil || criterion.DimensionValue == nil {
			return validatedPrediction{}, invalidOutput("criterion %d is missing a required field", i)
		}
		dimension := judgmentstore.CriterionDimension(*criterion.DimensionKey)
		if !dimension.Valid() {
			return validatedPrediction{}, invalidOutput("invalid criterion dimension %q", dimension)
		}
		if _, exists := values[dimension]; exists {
			return validatedPrediction{}, invalidOutput("duplicate criterion dimension %q", dimension)
		}
		value := *criterion.DimensionValue
		if math.IsNaN(value) || math.IsInf(value, 0) || value < -1 || value > 1 {
			return validatedPrediction{}, invalidOutput("criterion %q value %v is outside [-1, 1]", dimension, value)
		}
		values[dimension] = value
	}

	for _, dimension := range judgmentstore.CriterionDimensions {
		if _, ok := values[dimension]; !ok {
			return validatedPrediction{}, invalidOutput("missing criterion dimension %q", dimension)
		}
	}
	return validatedPrediction{
		verdictScore:    *model.VerdictScore,
		confidence:      *model.Confidence,
		explanation:     explanation,
		criterionValues: values,
	}, nil
}

func toStoredPrediction(validated validatedPrediction) judgmentstore.Prediction {
	criteria := make([]judgmentstore.PredictionCriterion, 0, len(judgmentstore.CriterionDimensions))
	for _, dimension := range judgmentstore.CriterionDimensions {
		criteria = append(criteria, judgmentstore.PredictionCriterion{
			Dimension: dimension,
			Value:     validated.criterionValues[dimension],
		})
	}
	return judgmentstore.Prediction{
		VerdictScore: validated.verdictScore,
		Confidence:   validated.confidence,
		Explanation:  truncatePrefixRunes(validated.explanation, maxExplanationRunes),
		JudgeVersion: EvalDatasetJudgeVersion,
		Criteria:     criteria,
	}
}

func truncatePrefixRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	const marker = "..."
	markerRunes := []rune(marker)
	if limit <= len(markerRunes) {
		return string(markerRunes[:limit])
	}
	return string(runes[:limit-len(markerRunes)]) + marker
}

func invalidOutput(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidOutput, fmt.Sprintf(format, args...))
}
