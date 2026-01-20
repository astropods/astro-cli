# Astro Agents

Utilities for defining and running AI agents. Includes tools, prompts, resources, and agent execution logic.

## Prerequisites

- [Bun](https://bun.sh/) runtime installed
- An [OpenAI API key](https://platform.openai.com/api-keys)

## Setup

### 1. Install Dependencies

From the **repository root**, install all dependencies:

```bash
bun install
```

This installs dependencies for the entire monorepo, including this package.

### 2. Configure Environment

Run the interactive setup script to create your `.env` file:

```bash
bun run playground:setup
```

This will prompt you for your OpenAI API key and create the required `playground/.env` file.

> **Note:** You can also manually copy `playground/.env.example` to `playground/.env` and fill in the values.

## Running the Playground

The playground is an interactive UI for testing agents. It requires two processes running in separate terminals:

### Terminal 1: Start the API Server

```bash
bun run playground:server
```

This starts the backend API server on `http://localhost:3001`.

### Terminal 2: Start the Frontend

```bash
bun run playground
```

This starts the Vite dev server for the playground UI (typically on `http://localhost:5173`).

## Available Scripts

| Script                      | Description                       |
| --------------------------- | --------------------------------- |
| `bun run dev`               | Watch mode for the agents package |
| `bun run playground`        | Start the playground frontend     |
| `bun run playground:setup`  | Interactive environment setup     |
| `bun run playground:server` | Start the playground API server   |

## Project Structure

```
astro-agents/
├── src/
│   ├── agent.ts          # AstroAgent class definition
│   ├── agents/           # Example agent implementations
│   └── index.ts          # Package exports
└── playground/
    ├── server.ts         # API server for testing agents
    ├── setup-env.ts      # Environment setup CLI
    ├── .env.example      # Environment template
    └── src/              # React frontend
```
