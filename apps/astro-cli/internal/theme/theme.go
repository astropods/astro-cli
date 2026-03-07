package theme

import (
	"github.com/astropods/astro/apps/astro-cli/internal/buildinfo"
	"github.com/charmbracelet/lipgloss"
	"github.com/fatih/color"
)

// IsPreview reports whether the CLI was built as a preview release.
var IsPreview bool

// Primary is the primary accent color for lipgloss styles.
var Primary lipgloss.Color

// PrimaryANSI is the ANSI escape code for the primary accent color.
var PrimaryANSI string

// PrimaryFatihAttr is the fatih/color attribute for the primary accent color.
var PrimaryFatihAttr color.Attribute

func init() {
	IsPreview = buildinfo.BinaryName == "ast-preview"
	if IsPreview {
		Primary = lipgloss.Color("205")
		PrimaryANSI = "\033[38;5;205m"
		PrimaryFatihAttr = color.FgHiMagenta
	} else {
		Primary = lipgloss.Color("6")
		PrimaryANSI = "\033[36m"
		PrimaryFatihAttr = color.FgCyan
	}
}
