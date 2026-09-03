import { Mastra } from '@mastra/core/mastra';
import { PostgresStore } from '@mastra/pg';
import { MastraStorageExporter, Observability, SensitiveDataFilter } from '@mastra/observability';
import { OtelExporter } from '@mastra/otel-exporter';
import { agent } from './agents/agent';
import { ensureDatabaseExists } from './ensure-database';
import {
  listSchedulesTool,
  startScheduleTool,
  stopScheduleTool,
} from './tools/schedule-tools';

/**
 * The platform's collector. `serve()` from the Mastra adapter wires this up on its
 * own, but src/astro.ts drives the bridge directly (see src/messaging.ts), so the
 * exporter is configured here instead. Mastra's OtelExporter passes the endpoint
 * straight to the OTel SDK, which does not append a signal path.
 */
const otlpEndpoint = process.env.OTEL_EXPORTER_OTLP_ENDPOINT;
const otelExporters = otlpEndpoint
  ? [
      new OtelExporter({
        provider: {
          custom: {
            endpoint: `${otlpEndpoint.replace(/\/+$/, '')}/v1/traces`,
            protocol: 'http/protobuf',
          },
        },
      }),
    ]
  : [];

/**
 * Postgres is the one built-in knowledge provider that injects five separate
 * variables rather than a single URL, so the connection is assembled by hand.
 *
 * Fail loudly on a missing variable: provider-mode injection silently not reaching
 * the container is a known platform failure mode, and it is far easier to diagnose
 * here than as an opaque connection error on the first tool call.
 */
function requireEnv(name: string): string {
  const value = process.env[name];
  if (!value) {
    throw new Error(
      `${name} is not set. Agent memory, tasks, schedules, and traces all live in the ` +
        'Postgres instance Astropods provisions from the `knowledge` block in ' +
        'astropods.yml, so start this agent with `ast dev`. To run it outside the ' +
        'stack, set POSTGRES_HOST, POSTGRES_DB, POSTGRES_USER (and ' +
        'POSTGRES_PORT/POSTGRES_PASSWORD if needed) yourself.',
    );
  }
  return value;
}

const postgres = {
  host: requireEnv('POSTGRES_HOST'),
  port: process.env.POSTGRES_PORT ? Number(process.env.POSTGRES_PORT) : 5432,
  database: requireEnv('POSTGRES_DB'),
  user: requireEnv('POSTGRES_USER'),
  // Container-mode Postgres can run with trust auth and no password.
  password: process.env.POSTGRES_PASSWORD ?? '',
};

// The platform injects a database name but does not create the database.
await ensureDatabaseExists(postgres);

export const mastra = new Mastra({
  agents: { '{{.Name}}': agent },
  tools: { startScheduleTool, listSchedulesTool, stopScheduleTool },
  storage: new PostgresStore({ id: 'mastra-storage', ...postgres }),
  observability: new Observability({
    configs: {
      default: {
        serviceName: '{{.Name}}',
        // Traces go to the same Postgres, plus the platform collector when the
        // stack provides one.
        exporters: [new MastraStorageExporter(), ...otelExporters],
        spanOutputProcessors: [new SensitiveDataFilter()],
      },
    },
  }),
  schedules: {
    /**
     * Delivery of scheduled output happens in the deliver-scheduled-output processor,
     * not here: for a threaded schedule this hook fires when the signal is accepted,
     * before the woken run has produced any text, and `result` is absent entirely.
     * Kept for visibility into fires that never became a run.
     */
    onFinish: ({ mastra, outcome, schedule }) => {
      if (outcome !== 'succeeded') {
        mastra.getLogger().warn('Schedule fired but did not run', {
          scheduleId: schedule.id,
          outcome,
        });
      }
    },
    onError: ({ mastra, schedule, phase, error }) => {
      mastra.getLogger().error('Schedule failed', {
        scheduleId: schedule.id,
        phase,
        error: error.message,
      });
    },
  },
});
