/**
 * {{.Name}} — Astropods entry point.
 *
 * `mastra dev` / `mastra start` boot the Mastra server, which serves Studio, the
 * HTTP API, and the background workers. On Astropods there is no Mastra server:
 * the platform talks to the agent over the messaging gRPC sidecar, so this file
 * starts the pieces the server would have started and then hands the agent to
 * the adapter.
 *
 * Environment variables injected by the platform:
{{- range .AgentEnvVars}}
{{- if .Description}}
 *   {{.Key}} - {{.Description}}
{{- else}}
 *   {{.Key}}
{{- end}}
{{- end}}
 */
import { mastra } from './mastra';
import { startMessaging } from './messaging';

const agent = mastra.getAgent('{{.Name}}');

// Mastra's cron machinery (SchedulerWorker) is only injected by startWorkers(),
// which the Mastra server normally calls. Without this, `start_schedule` writes
// a schedule row and returns an id, but the cron loop never boots and the
// schedule never fires.
await mastra.startWorkers();

// Owns the single messaging stream, so scheduled runs can push their output back
// into the conversation. See src/messaging.ts for why serve() is not used.
await startMessaging(agent, mastra.getLogger());
