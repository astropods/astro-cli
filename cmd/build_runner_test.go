package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDockerfileBaseImages(t *testing.T) {
	tests := []struct {
		name       string
		dockerfile string
		buildArgs  map[string]string
		want       []string
	}{
		{
			name:       "single FROM",
			dockerfile: "FROM python:3.12-slim\nRUN pip install flask\n",
			want:       []string{"python:3.12-slim"},
		},
		{
			name:       "multi-stage build",
			dockerfile: "FROM golang:1.22 AS builder\nRUN go build -o app\nFROM alpine:3.19\nCOPY --from=builder /app /app\n",
			want:       []string{"golang:1.22", "alpine:3.19"},
		},
		{
			name:       "FROM with platform flag",
			dockerfile: "FROM --platform=linux/amd64 node:20-alpine\nRUN npm install\n",
			want:       []string{"node:20-alpine"},
		},
		{
			name:       "scratch is excluded",
			dockerfile: "FROM golang:1.22 AS builder\nRUN go build\nFROM scratch\nCOPY --from=builder /app /app\n",
			want:       []string{"golang:1.22"},
		},
		{
			name:       "build stage alias as base is excluded",
			dockerfile: "FROM node:20 AS deps\nRUN npm ci\nFROM deps AS build\nRUN npm run build\nFROM alpine:3.19\nCOPY --from=build /app /app\n",
			want:       []string{"node:20", "alpine:3.19"},
		},
		{
			name:       "build arg substitution",
			dockerfile: "ARG BASE_IMAGE=python:3.12\nFROM ${BASE_IMAGE}\nRUN pip install flask\n",
			buildArgs:  map[string]string{"BASE_IMAGE": "python:3.11-slim"},
			want:       []string{"python:3.11-slim"},
		},
		{
			name:       "build arg substitution without braces",
			dockerfile: "FROM $BASE\n",
			buildArgs:  map[string]string{"BASE": "ubuntu:22.04"},
			want:       []string{"ubuntu:22.04"},
		},
		{
			name:       "unresolved build arg produces empty string and is excluded",
			dockerfile: "FROM ${UNKNOWN_IMAGE}\n",
			buildArgs:  nil,
			want:       nil,
		},
		{
			name:       "duplicate images are all returned",
			dockerfile: "FROM alpine:3.19 AS a\nRUN echo a\nFROM alpine:3.19 AS b\nRUN echo b\n",
			want:       []string{"alpine:3.19", "alpine:3.19"},
		},
		{
			name:       "case insensitive FROM",
			dockerfile: "from ubuntu:22.04\nRUN echo hello\n",
			want:       []string{"ubuntu:22.04"},
		},
		{
			name: "deep multi-stage chain only pulls external images",
			dockerfile: `FROM golang:1.22 AS base
RUN go mod download
FROM base AS builder
RUN go build -o app
FROM builder AS tester
RUN go test ./...
FROM alpine:3.19 AS runtime
COPY --from=builder /app /app
FROM runtime AS final
RUN apk add --no-cache ca-certificates
`,
			want: []string{"golang:1.22", "alpine:3.19"},
		},
		{
			name: "multi-stage with platform flags on each stage",
			dockerfile: `FROM --platform=linux/amd64 golang:1.22 AS builder
RUN go build -o app
FROM --platform=linux/amd64 alpine:3.19
COPY --from=builder /app /app
`,
			want: []string{"golang:1.22", "alpine:3.19"},
		},
		{
			name: "multi-stage with build arg base and scratch final",
			dockerfile: `FROM ${BUILD_IMAGE} AS builder
RUN make
FROM scratch
COPY --from=builder /app /app
`,
			buildArgs: map[string]string{"BUILD_IMAGE": "debian:bookworm"},
			want:      []string{"debian:bookworm"},
		},
		{
			name:       "empty dockerfile",
			dockerfile: "",
			want:       nil,
		},
		{
			name:       "comments and blank lines",
			dockerfile: "# This is a comment\n\nFROM nginx:latest\n# Another comment\n",
			want:       []string{"nginx:latest"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			dfPath := filepath.Join(dir, "Dockerfile")
			if err := os.WriteFile(dfPath, []byte(tt.dockerfile), 0o644); err != nil {
				t.Fatal(err)
			}

			got, err := parseDockerfileBaseImages(dfPath, tt.buildArgs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("image[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseDockerfileBaseImages_FileNotFound(t *testing.T) {
	_, err := parseDockerfileBaseImages("/nonexistent/Dockerfile", nil)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestPlatformImageTag(t *testing.T) {
	tests := []struct {
		baseName, tag, platform, want string
	}{
		{"myagent", "latest", "linux/amd64", "myagent-linux-amd64:latest"},
		{"myagent", "v1.0", "linux/arm64", "myagent-linux-arm64:v1.0"},
	}

	for _, tt := range tests {
		got := platformImageTag(tt.baseName, tt.tag, tt.platform)
		if got != tt.want {
			t.Errorf("platformImageTag(%q, %q, %q) = %q, want %q", tt.baseName, tt.tag, tt.platform, got, tt.want)
		}
	}
}

func TestParsePlatforms(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"linux/amd64", []string{"linux/amd64"}},
		{"linux/amd64,linux/arm64", []string{"linux/amd64", "linux/arm64"}},
	}

	for _, tt := range tests {
		got := parsePlatforms(tt.input)
		if len(got) != len(tt.want) {
			t.Fatalf("parsePlatforms(%q) = %v, want %v", tt.input, got, tt.want)
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("parsePlatforms(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}
