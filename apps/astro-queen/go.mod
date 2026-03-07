module github.com/astropods/astro/apps/astro-queen

go 1.24.2

require (
	github.com/1password/onepassword-sdk-go v0.4.0
	github.com/google/go-github/v69 v69.2.0
	github.com/astropods/astro/packages/astro-proto v0.0.0
	github.com/spf13/cobra v1.10.2
	golang.org/x/oauth2 v0.35.0
	google.golang.org/grpc v1.75.1
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/dylibso/observe-sdk/go v0.0.0-20240828172851-9145d8ad07e1 // indirect
	github.com/extism/go-sdk v1.7.1 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/ianlancetaylor/demangle v0.0.0-20251118225945-96ee0021ea0f // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/tetratelabs/wabin v0.0.0-20230304001439-f6f874872834 // indirect
	github.com/tetratelabs/wazero v1.11.0 // indirect
	go.opentelemetry.io/proto/otlp v1.9.0 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250825161204-c5933d9347a5 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/astropods/astro/packages/astro-proto => ../../packages/astro-proto
