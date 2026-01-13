# Astro CLI

A powerful command-line interface for the Astro platform, combining the structure of [Cobra](https://github.com/spf13/cobra) with the beautiful interactive TUI capabilities of [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Features

- **Command-line interface** powered by Cobra for structured commands
- **Interactive TUI mode** with Bubble Tea for beautiful terminal interactions
- **Version information** display with build details

## Installation

### Build from source

```bash
cd apps/astro-cli
go build -o astro .
```

### Install globally

```bash
cd apps/astro-cli
go install
```

## Usage

### Commands

#### Interactive Mode (TUI)

Launch the beautiful terminal user interface:

```bash
./astro interactive
```

Or simply run without arguments to show help:

```bash
./astro
```

Navigation in interactive mode:
- `↑/↓` or `j/k` - Navigate menu
- `Enter` or `Space` - Select option
- `Esc` - Go back to main menu
- `q` - Quit

#### Version Information

Display version, commit, and build date:

```bash
./astro version
```

### Global Flags

- `-v, --verbose` - Enable verbose output for more detailed information
- `-h, --help` - Show help for any command

## Development

### Prerequisites

- Go 1.24 or higher

### Dependencies

- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Terminal styling

### Project Structure

```
astro-cli/
├── main.go              # Entry point
├── cmd/
│   ├── root.go         # Root command definition
│   ├── interactive.go  # Interactive TUI mode
│   └── version.go      # Version command
├── ui/
│   └── model.go        # Bubble Tea TUI model
├── go.mod
├── go.sum
└── README.md
```

## Command Examples

```bash
# Show help
./astro --help

# Show version
./astro version

# Launch interactive mode
./astro interactive
```

## Building

Use the included Makefile for common tasks:

```bash
# Build the binary
make build

# Install globally
make install

# Clean build artifacts
make clean

# Run tests
make test

# Format code
make fmt
```

## Architecture

The Astro CLI combines two powerful Go libraries:

1. **Cobra**: Provides the command structure, argument parsing, and help generation for the CLI commands
2. **Bubble Tea**: Powers the interactive TUI mode with elegant terminal rendering and user interactions

This hybrid approach gives users the flexibility to:
- Use traditional command-line arguments for scripting and automation
- Use the interactive TUI for exploratory workflows and better UX
