package handlers

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeEmails(t *testing.T) {
	cases := []struct {
		name    string
		in      []string
		want    []string
		wantErr bool
	}{
		{name: "nil", in: nil, want: []string{}},
		{
			name: "trims lowercases dedupes drops empty",
			in:   []string{"  Dev@X.com ", "dev@x.com", "", "OPS@x.com"},
			want: []string{"dev@x.com", "ops@x.com"},
		},
		{name: "invalid no domain dot", in: []string{"dev@localhost"}, wantErr: true},
		{name: "invalid no at", in: []string{"devx.com"}, wantErr: true},
		{name: "invalid spaces", in: []string{"a b@x.com"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeEmails(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNormalizeTokenName(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "trims", in: "  Engineering laptops \n", want: "Engineering laptops"},
		{name: "empty", in: "", wantErr: true},
		{name: "whitespace only", in: "   ", wantErr: true},
		{name: "at cap", in: strings.Repeat("a", maxTokenNameLen), want: strings.Repeat("a", maxTokenNameLen)},
		{name: "over cap", in: strings.Repeat("a", maxTokenNameLen+1), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeTokenName(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeEmailsRejectsOverCap(t *testing.T) {
	in := make([]string, maxExcludedEmails+1)
	for i := range in {
		in[i] = "u" + itoa(i) + "@x.com" // unique, so the cap (not dedupe) trips
	}
	if _, err := normalizeEmails(in); err == nil {
		t.Fatal("want cap error, got nil")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
