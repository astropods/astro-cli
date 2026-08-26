import { pathToFileURL } from 'node:url';
import { Agent } from '@mastra/core/agent';
import { TaskSignalProvider } from '@mastra/core/signals';
{{- if .AIGateway}}
import { askUserTool, webFetchTool } from '@mastra/core/tools';
{{- else}}
import { askUserTool, webFetchTool, webSearchTool } from '@mastra/core/tools';
{{- end}}
import { LocalFilesystem, LocalSandbox, WORKSPACE_TOOLS, Workspace } from '@mastra/core/workspace';
import { Memory } from '@mastra/memory';
{{- if .AIGateway}}
import { createOpenAI } from '@ai-sdk/openai';
{{- end}}
import { deliverScheduledOutput } from '../deliver-scheduled-output';
import {
  listSchedulesTool,
  startScheduleTool,
  stopScheduleTool,
} from '../tools/schedule-tools';

/**
 * On Astropods this points at a writable scratch directory (`/tmp/workspace`)
 * because the container filesystem is read-only. Its contents do not survive a
 * container restart.
 */
{{- if .AIGateway}}

// Astro AI Gateway: managed model access over an OpenAI-compatible API. No provider
// key — the platform injects the base URL and credential at runtime.
const gateway = createOpenAI({
  apiKey: process.env.ASTRO_GATEWAY_API_KEY,
  baseURL: `${process.env.ASTRO_GATEWAY_URL}/v1`,
});
{{- end}}

/**
 * What this agent is for, from `ast create`. Kept in its own single-quoted literal so
 * a quote or backtick in the description cannot break the instructions below.
 */
const purpose = '{{.Description | jsStr}}';

const workspacePath = process.env.WORKSPACE_PATH || 'workspace';

/**
 * `file://` links only mean something when the user is on the same machine as
 * the agent. Over Astropods messaging they point into a container the user
 * cannot browse, so the agent needs different closing instructions.
 */
const fileDeliveryInstructions =
  process.env.ASTROPODS_CONTAINER === '1'
    ? 'Files you create live in a workspace the user cannot browse. Give the workspace-relative path, and paste the contents into the conversation when they are short enough to be useful; avoid file:// URLs, localhost, and static-file servers.'
    : `For local file changes, end with a plain-text URL using ${pathToFileURL(`${workspacePath}/`).href}; avoid Markdown links, localhost, /workspace, relative paths, and static-file servers.`;

const workspace = new Workspace({
  id: 'agent-workspace',
  name: 'Agent Workspace',
  filesystem: new LocalFilesystem({
    basePath: workspacePath,
  }),
  sandbox: new LocalSandbox({
    workingDirectory: workspacePath,
  }),
  tools: {
    [WORKSPACE_TOOLS.FILESYSTEM.WRITE_FILE]: {
      requireReadBeforeWrite: true,
    },
    [WORKSPACE_TOOLS.FILESYSTEM.EDIT_FILE]: {
      requireReadBeforeWrite: true,
    },
    [WORKSPACE_TOOLS.FILESYSTEM.DELETE]: {
      requireApproval: true,
    },
  },
});

export const agent = new Agent({
  id: '{{.Name}}',
  name: '{{.Name | humanName}}',
  description: '{{.Description | jsStr}}',
  metadata: {
{{- if .AIGateway}}
    // Gateway models have no native web search (see the tools block), so these stay
    // within what this agent can actually do: the workspace, the task list, and
    // web_fetch against a URL the user names.
    suggestedPrompts: [
      'Build a Japanese sakura festival landing page.',
      'Draft a plan for a small project and track it as tasks.',
      'Summarise https://docs.astropods.com/llms.txt for me.',
    ],
{{- else}}
    suggestedPrompts: [
      "What's the weather in Austin this weekend?",
      "What's the SPCX stock price right now?",
      'Build a Japanese sakura festival landing page.',
    ],
{{- end}}
  },
  instructions: `You are {{.Name | humanName}}. ${purpose}

{{- if .AIGateway}}
You are also a starter agent for exploring what Mastra can do. Help the user try useful capabilities, build small projects, track multi-step work, and shape this harness into a starting point for future work.

Suggested prompts: Create a Japanese Sakura festival page; Draft a plan and track it as tasks; Summarise a page whose URL I give you.

You have no web search. You can read a page the user names with web_fetch, so ask for a URL rather than guessing at current facts, and say plainly when something needs live data you cannot reach. web_fetch returns raw HTML, so prefer a text endpoint when one exists — the Astropods docs serve clean Markdown for any page by appending .md, and https://docs.astropods.com/llms.txt is a compact overview.
{{- else}}
You are also a starter agent for exploring what Mastra can do. Help the user try useful capabilities, build small projects, answer current questions, and shape this harness into a starting point for future work.

Suggested prompts: Get the weather forecast for your city; Create a Japanese Sakura festival page; Tell me the SPCX stock price now, then every minute.
{{- end}}

When the user greets you or does not have a specific task, invite them to try the suggested prompts.

Ask concise questions when something is unclear or a good question could surface a useful insight.

${fileDeliveryInstructions}
`,
{{- if .AIGateway}}
  // Chosen at deploy time from the gateway's model list and injected as
  // MODEL_DEFAULT, so switching models needs no code change.
  model: gateway(process.env.MODEL_DEFAULT ?? 'claude-sonnet-4-6'),
{{- else if .HasIntegration "openai"}}
  model: 'openai/gpt-5.6-terra',
{{- else}}
  model: 'anthropic/claude-sonnet-4-5',
{{- end}}
  defaultOptions: {
    maxSteps: 100,
    autoResumeSuspendedTools: true,
    // Stable tags on every span, so traces are filterable without extra
    // instrumentation. The collector endpoint is injected in deployed
    // environments only; locally this is a no-op.
    tracingOptions: {
      tags: ['astro', 'agent:{{.Name}}'],
      metadata: {
        agent_id: '{{.Name}}',
      },
    },
  },
  memory: new Memory({
    options: {
      generateTitle: true,
      // A cheaper model compacts long histories.
{{- if .AIGateway}}
      observationalMemory: {
        model: gateway(process.env.MODEL_DEFAULT ?? 'claude-sonnet-4-6'),
      },
{{- else if .HasIntegration "openai"}}
      observationalMemory: {
        model: 'openai/gpt-5-mini',
      },
{{- else}}
      observationalMemory: {
        model: 'anthropic/claude-haiku-4-5',
      },
{{- end}}
    },
  }),
  workspace,
  // Delivers the output of schedule-woken runs back into the conversation; a no-op
  // for ordinary turns, which the messaging bridge already streams.
  outputProcessors: [deliverScheduledOutput],
  tools: {
    ask_user: askUserTool,
    start_schedule: startScheduleTool,
    list_schedules: listSchedulesTool,
    stop_schedule: stopScheduleTool,
    web_fetch: webFetchTool,
{{- if not .AIGateway}}
    // Resolved at run time to the active provider's native search
    // (openai.web_search, anthropic.web_search_20250305, …), so it costs nothing
    // here and executes on the provider's side.
    web_search: webSearchTool,
{{- else}}
    // No web_search: it is a provider-defined tool, and Mastra infers the provider
    // from the model. A gateway model is an OpenAI-compatible client reporting
    // "openai.responses", which does not normalize to a supported provider, so
    // including it throws WEB_SEARCH_UNSUPPORTED_PROVIDER as soon as tools resolve.
    // web_fetch still works — it runs in this process, not the provider's.
{{- end}}
  },
  signals: [new TaskSignalProvider()],
});
