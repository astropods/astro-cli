# Astro

A monorepo for Astro agents and graph visualization.

## Prerequisites

### Install Bun

```bash
curl -fsSL https://bun.com/install | bash
```

This installs `bun` to `~/.bun/bin/bun` by default. Add it to your PATH by adding the following to your shell configuration file (e.g., `~/.bashrc` or `~/.zshrc`):

```bash
export PATH="$HOME/.bun/bin:$PATH"
```

Then reload your shell or run:

```bash
source ~/.bashrc  # or source ~/.zshrc
```

## Setup

Install all dependencies from the root of the repository:

```bash
bun i
```

## Development

To start the development server:

```bash
cd apps/astro-client
bun run dev
```

Open your browser to the URL shown in the terminal (typically http://localhost:5173).

## Project Structure

```
├── apps/
│   └── astro-client/      # React frontend application
├── packages/
│   ├── astro-agents/      # Agent implementations
│   ├── astro-graph/       # Graph data structures
│   ├── astro-nodes/       # Node types
│   └── astro-types/       # Shared TypeScript types
```
