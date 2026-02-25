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
import { serve } from '@astropods/adapter-mastra';

const agent = new Agent({
  id: '{{.Name}}',
  name: '{{.Name | humanName}}',
  instructions: 'You are {{.Name | humanName}}, a helpful AI assistant. {{.Description | jsStr}}',
{{- if and (ne .ModelProvider "") (ne .Model "")}}
  model: '{{.ModelProvider}}/{{.Model}}',
{{- else if .HasIntegration "anthropic"}}
  model: 'anthropic/claude-sonnet-4-5',
{{- else if .HasIntegration "openai"}}
  model: 'openai/gpt-4o',
{{- else}}
  model: 'anthropic/claude-sonnet-4-5',
{{- end}}
});

serve(agent);
