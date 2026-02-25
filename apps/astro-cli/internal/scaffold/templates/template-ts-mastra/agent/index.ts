/**
 * {{.Name}} - {{.Description}}
 *
 * This agent uses Mastra's Agent class with the Astro adapter to connect
 * to the Astro messaging service via gRPC.
 *
 * Environment variables (automatically injected by 'astro dev'):
 *   GRPC_SERVER_ADDR - Messaging service address (default: localhost:9090)
{{- if .HasIntegration "anthropic"}}
 *   ANTHROPIC_API_KEY - Anthropic API key for Claude models
{{- end}}
{{- if .HasIntegration "openai"}}
 *   OPENAI_API_KEY - OpenAI API key for GPT models
{{- end}}
{{- if .HasKnowledge "qdrant"}}
 *   QDRANT_HOST - Qdrant vector database host
 *   QDRANT_PORT - Qdrant vector database port
{{- end}}
{{- if .HasKnowledge "redis"}}
 *   REDIS_HOST - Redis host
 *   REDIS_PORT - Redis port
{{- end}}
{{- if .HasKnowledge "neo4j"}}
 *   NEO4J_HOST - Neo4j graph database host
 *   NEO4J_PORT - Neo4j graph database port
{{- end}}
 */

import { Agent } from '@mastra/core/agent';
import { serve } from '@astropods/adapter-mastra';

const agent = new Agent({
  name: '{{.Name}}',
  instructions: 'You are {{.Name}}, a helpful AI assistant. {{.Description}}',
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
