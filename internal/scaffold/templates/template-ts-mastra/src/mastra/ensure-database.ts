import { Client } from 'pg';

export type PostgresConnection = {
  host: string;
  port: number;
  database: string;
  user: string;
  password: string;
};

/**
 * Create the agent's database if it does not exist yet.
 *
 * Astropods injects `POSTGRES_DB=<sanitized project name>` but never creates that
 * database. Locally the volume is named `knowledge-<key>-data` with no project
 * prefix, so any two projects using the same knowledge key share one PGDATA
 * directory — and Postgres skips `initdb` on a non-empty one, ignoring `POSTGRES_DB`.
 * `PostgresStore.init()` then fails with `3D000 database "..." does not exist`.
 *
 * Connect to the `postgres` maintenance database and create it once. Failures are
 * logged rather than thrown: on a managed instance the database usually already
 * exists and the maintenance database may not be reachable, in which case
 * PostgresStore's own error is the more useful one.
 */
export async function ensureDatabaseExists(config: PostgresConnection): Promise<void> {
  const client = new Client({ ...config, database: 'postgres' });

  try {
    await client.connect();

    const { rowCount } = await client.query('select 1 from pg_database where datname = $1', [
      config.database,
    ]);
    if (rowCount) return;

    // A database name cannot be parameterized, so quote it the way quote_ident
    // would rather than interpolating POSTGRES_DB straight into the statement.
    await client.query(`create database "${config.database.replace(/"/g, '""')}"`);
    console.log(`Created database "${config.database}".`);
  } catch (error) {
    // 42P04 = duplicate_database: another instance won the race, which is fine.
    if ((error as { code?: string }).code === '42P04') return;

    console.warn(
      `Could not ensure database "${config.database}" exists ` +
        `(${(error as Error).message}). Continuing; storage init will report the real problem.`,
    );
  } finally {
    await client.end().catch(() => {});
  }
}
