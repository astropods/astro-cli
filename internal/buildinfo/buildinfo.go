package buildinfo

import "fmt"

// All build-time variables are declared here and set via ldflags:
//
//	go build -ldflags "-X github.com/astropods/astro-cli/internal/buildinfo.<Var>=<value>"

// Build type constants derived from BinaryName.
const (
	BuildTypeProd    = "prod"
	BuildTypePreview = "preview"
	BuildTypeDev     = "dev"
)

// BuildType is set at init time from BinaryName and is the authoritative signal
// for build-specific behavior (command visibility, theming, etc.).
var BuildType string

// AppDirName is the directory name used to namespace config and state for this
// binary (e.g. ".ast-dev"). Used as both the home config dir (~/.ast-dev) and
// the project-local state dir (.ast-dev/ in the working directory).
var AppDirName string

func init() {
	switch BinaryName {
	case "ast":
		BuildType = BuildTypeProd
	case "ast-preview":
		BuildType = BuildTypePreview
	default:
		BuildType = BuildTypeDev
	}
	AppDirName = "." + BinaryName
}

// Validate returns an error if BinaryName is not one of the recognised values.
// Called at startup to catch misconfigured builds early.
func Validate() error {
	switch BinaryName {
	case "ast", "ast-preview", "ast-dev":
		return nil
	default:
		return fmt.Errorf("unrecognised binary name %q: must be one of ast, ast-preview, ast-dev", BinaryName)
	}
}

var (
	// BinaryName defaults to "ast-dev"; set to "ast" for production or "ast-preview" for preview builds.
	BinaryName = "ast-dev"

	// Version is the release version string (e.g. "1.2.3"); defaults to "dev".
	Version = BuildTypeDev

	// Commit is the short git SHA baked in at build time.
	Commit = ""

	// DownloadBaseURL is the base URL for CLI self-upgrade downloads.
	DownloadBaseURL = ""

	// WorkOSClientID is the public OAuth client ID for the device flow.
	WorkOSClientID = "client_01K1VMRDRQ94MV98D9ANFVT7H2"

	// DefaultServerURL is the platform API base URL.
	DefaultServerURL = "http://localhost:8080"

	// DefaultRegistryURL overrides the registry URL derived from DefaultServerURL.
	// When empty the registry URL is derived via auth.RegistryURLFromServerURL.
	DefaultRegistryURL = ""

	// AmplitudeAPIKey enables telemetry when non-empty.
	AmplitudeAPIKey = ""
)
