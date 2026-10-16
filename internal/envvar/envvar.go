// Package envvar names the environment variables the CLI reads to configure itself.
package envvar

const (
	AccessToken    = "ASTRO_ACCESS_TOKEN"
	RefreshToken   = "ASTRO_REFRESH_TOKEN"
	Account        = "ASTRO_ACCOUNT"
	NonInteractive = "ASTRO_NONINTERACTIVE"
	NoTelemetry    = "ASTRO_NO_TELEMETRY"
	// Via names the caller that started this process, such as "mcp" for a child of ast mcp.
	Via = "ASTRO_VIA"
)
