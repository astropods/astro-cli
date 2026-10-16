/**
 * {{.Name}} - {{.Description}}
 *
 * This agent uses Mastra's Agent class with the Astro adapter to connect
 * to the Astro messaging service via gRPC.
 *
 * Environment variables (automatically injected by 'astro dev'):
{{- range .AgentEnvVars}}
{{- if .Description}}
 *   {{.Key}} - {{.Description}}
{{- else}}
 *   {{.Key}}
{{- end}}
{{- end}}
 */

import { Agent } from '@mastra/core/agent';
import { Mastra } from '@mastra/core/mastra';
import { Memory } from '@mastra/memory';
import { LibSQLStore } from '@mastra/libsql';
import { Observability } from '@mastra/observability';
import { OtelExporter } from '@mastra/otel-exporter';
import { serve, type ModelChoice } from '@astropods/adapter-mastra';
{{- if .AIGateway}}
import { createOpenAI } from '@ai-sdk/openai';

// Astro AI Gateway: managed model access over the OpenAI-compatible API.
// No provider key needed — the platform injects URL + credential at runtime.
const gateway = createOpenAI({
  apiKey: process.env.ASTRO_GATEWAY_API_KEY,
  baseURL: `${process.env.ASTRO_GATEWAY_URL}/v1`,
});
{{- end}}

const memory = new Memory({
  storage: new LibSQLStore({
    id: 'memory',
    url: ':memory:',
  }),
});

function resolveOtlpTracesEndpoint(): string {
  const raw = process.env.OTEL_EXPORTER_OTLP_ENDPOINT || 'http://localhost:4318';
  try {
    const url = new URL(raw);
    if (!url.pathname || url.pathname === '/') {
      url.pathname = '/v1/traces';
    }
    return url.toString();
  } catch {
    return `${raw.replace(/\/+$/, '')}/v1/traces`;
  }
}

const observability = new Observability({
  configs: {
    otel: {
      serviceName: '{{.Name}}',
      exporters: [
        new OtelExporter({
          provider: {
            custom: {
              endpoint: resolveOtlpTracesEndpoint(),
              protocol: 'http/protobuf',
            },
          },
        }),
      ],
    },
  },
});

const agent = new Agent({
  id: '{{.Name}}',
  name: '{{.Name | humanName}}',
  instructions: 'You are {{.Name | humanName}}, a helpful AI assistant. {{.Description | jsStr}}',
{{- if .AIGateway}}
  // The model picked in chat (selection: live), else the one chosen at deploy (MODEL_DEFAULT).
  model: ({ requestContext }) =>
    gateway(requestContext.get<'model', ModelChoice | undefined>('model')?.model ?? process.env.MODEL_DEFAULT ?? 'claude-sonnet-4-6'),
{{- else}}
  // The model picked in chat (selection: live), else the one named here.
{{- if and (.HasIntegration "openai") (not (.HasIntegration "anthropic"))}}
  model: ({ requestContext }) =>
    `openai/${requestContext.get<'model', ModelChoice | undefined>('model')?.model ?? 'gpt-4o'}`,
{{- else}}
  model: ({ requestContext }) =>
    `anthropic/${requestContext.get<'model', ModelChoice | undefined>('model')?.model ?? 'claude-sonnet-4-5'}`,
{{- end}}
{{- end}}
  memory,
  // Ensure traces include stable Astro metadata by default.
  // The collector endpoint (OTEL_EXPORTER_OTLP_ENDPOINT) is injected in deployed
  // environments only — `ast project start` runs no collector, so local tracing
  // is a silent no-op.
  defaultOptions: {
    tracingOptions: {
      tags: ['astro', 'agent:{{.Name}}'],
      metadata: {
        agent_id: '{{.Name}}',
      },
    },
  },
});

// Instantiate Mastra so it registers agents/observability plugins at startup.
// `serve(agent)` handles request serving; this constructor call wires runtime integration.
new Mastra({
  agents: {
    '{{.Name}}': agent,
  },
  observability,
});

serve(agent);
