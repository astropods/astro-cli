package buildinfo

// BinaryName is set at build time via ldflags. Defaults to "ast" (production).
// Both the cmd and theme packages derive their binary name from this value.
var BinaryName = "ast"
