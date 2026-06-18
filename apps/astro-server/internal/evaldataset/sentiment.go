package evaldataset

import (
	"regexp"
	"strings"
)

type Sentiment string

const (
	SentimentPositive Sentiment = "positive"
	SentimentNegative Sentiment = "negative"
	SentimentNone     Sentiment = ""
)

var positiveKeywords = []string{
	"thanks", "thank you", "thx", "perfect", "great", "awesome",
	"helpful", "interesting", "nice", "exactly", "correct", "right", "yes",
}

var negativeKeywords = []string{
	"no", "wrong", "bad", "incorrect", "unhelpful", "useless", "stop", "nope",
}

var (
	positiveRe = buildKeywordBoundaryRegex(positiveKeywords)
	negativeRe = buildKeywordBoundaryRegex(negativeKeywords)
)

func buildKeywordBoundaryRegex(words []string) *regexp.Regexp {
	escaped := make([]string, len(words))
	for i, w := range words {
		escaped[i] = regexp.QuoteMeta(w)
	}

	// \b matches "right!" but avoids matching "right" inside "bright".
	return regexp.MustCompile(`(?i)\b(?:` + strings.Join(escaped, "|") + `)\b`)
}

// Infer classifies a user-message string as positive, negative, or none using
// keyword matches. It checks both keyword sets and returns positive when both
// match. Empty or whitespace-only input returns SentimentNone.
func Infer(input string) Sentiment {
	s := strings.TrimSpace(input)
	if s == "" {
		return SentimentNone
	}
	hasPos := positiveRe.MatchString(s)
	hasNeg := negativeRe.MatchString(s)
	switch {
	case hasPos:
		return SentimentPositive
	case hasNeg:
		return SentimentNegative
	default:
		return SentimentNone
	}
}

// InferFromAny extracts a representative string from a free-form JSON value and
// classifies it with Infer. Strings pass through; maps are searched for
// content-like fields; slices are searched from last to first.
func InferFromAny(input any) Sentiment {
	return Infer(stringifyInput(input))
}

func stringifyInput(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case map[string]any:
		for _, key := range []string{"content", "text", "message", "input"} {
			if val, ok := x[key]; ok {
				if s := stringifyInput(val); s != "" {
					return s
				}
			}
		}
		return ""
	case []any:
		// Common shape: messages array with the last entry being the newest.
		for i := len(x) - 1; i >= 0; i-- {
			if s := stringifyInput(x[i]); s != "" {
				return s
			}
		}
		return ""
	default:
		return ""
	}
}
