import { existsSync, readFileSync, readdirSync } from "node:fs";
import { join, resolve } from "node:path";
import { createSdkMcpServer, tool } from "@anthropic-ai/claude-agent-sdk";
import { z } from "zod";

const PKG_ROOT = resolve(import.meta.dirname, "../..");
const SOURCES_DIR = join(PKG_ROOT, "sources");

function listIds(): string[] {
  return readdirSync(SOURCES_DIR)
    .filter((f) => f.endsWith(".svg") && !f.endsWith(".dark.svg"))
    .map((f) => f.replace(/\.svg$/, ""))
    .sort();
}

function readPair(id: string): { lightSvg: string; darkSvg: string } | null {
  const lightPath = join(SOURCES_DIR, `${id}.svg`);
  const darkPath = join(SOURCES_DIR, `${id}.dark.svg`);
  if (!existsSync(lightPath) || !existsSync(darkPath)) return null;
  return {
    lightSvg: readFileSync(lightPath, "utf8"),
    darkSvg: readFileSync(darkPath, "utf8"),
  };
}

function textResult(text: string) {
  return { content: [{ type: "text" as const, text }] };
}

function errorResult(message: string) {
  return { content: [{ type: "text" as const, text: message }], isError: true };
}

const listIconsTool = tool(
  "list_icons",
  "List every icon id currently in this brand-icons package. Returns a JSON object: { ids: string[] }. Use this when you need to know what's in the library — for example before deciding whether a brand the user mentioned already exists.",
  {},
  async () => {
    const ids = listIds();
    return textResult(JSON.stringify({ ids }, null, 2));
  },
);

const readIconTool = tool(
  "read_icon",
  "Read the current light and dark source SVGs for an existing icon. Returns { id, lightSvg, darkSvg } or an error if the icon is not in the package. Use this when the user asks you to inspect, critique, or improve an existing icon — you must read what's there before proposing changes.",
  {
    id: z
      .string()
      .regex(/^[a-z0-9][a-z0-9-]*$/, "id must be kebab-case")
      .describe("Kebab-case icon id, e.g. 'sentry', 'google-drive'"),
  },
  async ({ id }) => {
    const pair = readPair(id);
    if (!pair) return errorResult(`Icon "${id}" not found in sources/.`);
    return textResult(JSON.stringify({ id, ...pair }, null, 2));
  },
);

export const iconsMcpServer = createSdkMcpServer({
  name: "icons",
  version: "0.1.0",
  tools: [listIconsTool, readIconTool],
});

// Exposed tool names for `allowedTools` (SDK MCP tool naming: mcp__<server>__<tool>).
// Read-only: the agent never modifies the library directly. All changes must
// surface as candidates that the human explicitly approves and saves.
export const ICONS_TOOL_NAMES = [
  "mcp__icons__list_icons",
  "mcp__icons__read_icon",
];
