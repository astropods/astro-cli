module github.com/postman/astro/apps/astro-queen

go 1.24.2

require (
	github.com/google/go-github/v69 v69.2.0
	github.com/postman/astro/packages/astro-proto v0.0.0
	github.com/spf13/cobra v1.10.2
	golang.org/x/oauth2 v0.35.0
	google.golang.org/grpc v1.75.1
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/net v0.41.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.26.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250707201910-8d1bb00bc6a7 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)

replace github.com/postman/astro/packages/astro-proto => ../../packages/astro-proto
