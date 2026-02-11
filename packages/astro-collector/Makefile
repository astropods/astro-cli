.PHONY: build install-ocb clean

OCB_VERSION := 0.145.0

# Install the OpenTelemetry Collector Builder
install-ocb:
	go install go.opentelemetry.io/collector/cmd/builder@v$(OCB_VERSION)

# Build the collector using ocb
build: install-ocb
	builder --config=builder-config.yaml --output-path=./bin

clean:
	rm -rf ./bin
