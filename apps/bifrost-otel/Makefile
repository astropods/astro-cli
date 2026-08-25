OCB_VERSION ?= 0.157.0
BIN         := ./bin/bifrost-otel

.PHONY: install-ocb build run test tidy

install-ocb:
	go install go.opentelemetry.io/collector/cmd/builder@v$(OCB_VERSION)

build:
	builder --config builder-config.yaml

run: build
	$(BIN) --config config/collector-config.yaml

tidy:
	cd internal/exporter/bifrostotel && go mod tidy

test:
	cd internal/exporter/bifrostotel && go test ./...
