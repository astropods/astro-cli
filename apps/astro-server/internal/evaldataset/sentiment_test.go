package evaldataset

import "testing"

func TestInfer(t *testing.T) {
	cases := []struct {
		in   string
		want Sentiment
	}{
		{"", SentimentNone},
		{"   ", SentimentNone},
		{"thanks!", SentimentPositive},
		{"Thank you so much", SentimentPositive},
		{"Perfect, that worked", SentimentPositive},
		{"right!", SentimentPositive},
		{"bright idea", SentimentNone},
		{"no, that's wrong", SentimentNegative},
		{"this is bad", SentimentNegative},
		{"Wrong answer", SentimentNegative},
		{"wait, what's the high?", SentimentNone},
		{"thanks for nothing", SentimentPositive},
		{"ugh, this output is terrible", SentimentNone},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := Infer(c.in); got != c.want {
				t.Fatalf("Infer(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestInferFromAny(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want Sentiment
	}{
		{"nil", nil, SentimentNone},
		{"string", "thanks", SentimentPositive},
		{"map with content", map[string]any{"content": "perfect"}, SentimentPositive},
		{"map with text", map[string]any{"text": "wrong"}, SentimentNegative},
		{"slice picks last", []any{"old", "thanks"}, SentimentPositive},
		{"nested", map[string]any{"message": map[string]any{"content": "no"}}, SentimentNegative},
		{"empty map", map[string]any{"foo": "bar"}, SentimentNone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := InferFromAny(c.in); got != c.want {
				t.Fatalf("InferFromAny(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
