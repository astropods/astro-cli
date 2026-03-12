-- atlas:txmode none

CREATE SCHEMA IF NOT EXISTS river;

-- River main migration 002 [up]
CREATE TYPE river.river_job_state AS ENUM(
  'available',
  'cancelled',
  'completed',
  'discarded',
  'retryable',
  'running',
  'scheduled'
);

CREATE TABLE river.river_job(
  -- 8 bytes
  id bigserial PRIMARY KEY,

  -- 8 bytes (4 bytes + 2 bytes + 2 bytes)
  --
  -- `state` is kept near the top of the table for operator convenience -- when
  -- looking at jobs with `SELECT *` it'll appear first after ID. The other two
  -- fields aren't as important but are kept adjacent to `state` for alignment
  -- to get an 8-byte block.
  state river.river_job_state NOT NULL DEFAULT 'available',
  attempt smallint NOT NULL DEFAULT 0,
  max_attempts smallint NOT NULL,

  -- 8 bytes each (no alignment needed)
  attempted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT NOW(),
  finalized_at timestamptz,
  scheduled_at timestamptz NOT NULL DEFAULT NOW(),

  -- 2 bytes (some wasted padding probably)
  priority smallint NOT NULL DEFAULT 1,

  -- types stored out-of-band
  args jsonb,
  attempted_by text[],
  errors jsonb[],
  kind text NOT NULL,
  metadata jsonb NOT NULL DEFAULT '{}',
  queue text NOT NULL DEFAULT 'default',
  tags varchar(255)[],

  CONSTRAINT finalized_or_finalized_at_null CHECK ((state IN ('cancelled', 'completed', 'discarded') AND finalized_at IS NOT NULL) OR finalized_at IS NULL),
  CONSTRAINT max_attempts_is_positive CHECK (max_attempts > 0),
  CONSTRAINT priority_in_range CHECK (priority >= 1 AND priority <= 4),
  CONSTRAINT queue_length CHECK (char_length(queue) > 0 AND char_length(queue) < 128),
  CONSTRAINT kind_length CHECK (char_length(kind) > 0 AND char_length(kind) < 128)
);

CREATE INDEX river_job_kind ON river.river_job USING btree(kind);

CREATE INDEX river_job_state_and_finalized_at_index ON river.river_job USING btree(state, finalized_at) WHERE finalized_at IS NOT NULL;

CREATE INDEX river_job_prioritized_fetching_index ON river.river_job USING btree(state, queue, priority, scheduled_at, id);

CREATE INDEX river_job_args_index ON river.river_job USING GIN(args);

CREATE INDEX river_job_metadata_index ON river.river_job USING GIN(metadata);

CREATE OR REPLACE FUNCTION river.river_job_notify()
  RETURNS TRIGGER
  AS $$
DECLARE
  payload json;
BEGIN
  IF NEW.state = 'available' THEN
    -- Notify will coalesce duplicate notifications within a transaction, so
    -- keep these payloads generalized:
    payload = json_build_object('queue', NEW.queue);
    PERFORM
      pg_notify('river_insert', payload::text);
  END IF;
  RETURN NULL;
END;
$$
LANGUAGE plpgsql;

CREATE TRIGGER river_notify
  AFTER INSERT ON river.river_job
  FOR EACH ROW
  EXECUTE PROCEDURE river.river_job_notify();

CREATE UNLOGGED TABLE river.river_leader(
    -- 8 bytes each (no alignment needed)
    elected_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,

    -- types stored out-of-band
    leader_id text NOT NULL,
    name text PRIMARY KEY,

    CONSTRAINT name_length CHECK (char_length(name) > 0 AND char_length(name) < 128),
    CONSTRAINT leader_id_length CHECK (char_length(leader_id) > 0 AND char_length(leader_id) < 128)
);

-- River main migration 003 [up]
ALTER TABLE river.river_job ALTER COLUMN tags SET DEFAULT '{}';
UPDATE river.river_job SET tags = '{}' WHERE tags IS NULL;
ALTER TABLE river.river_job ALTER COLUMN tags SET NOT NULL;

-- River main migration 004 [up]
ALTER TABLE river.river_job ALTER COLUMN args SET DEFAULT '{}';
UPDATE river.river_job SET args = '{}' WHERE args IS NULL;
ALTER TABLE river.river_job ALTER COLUMN args SET NOT NULL;
ALTER TABLE river.river_job ALTER COLUMN args DROP DEFAULT;

ALTER TABLE river.river_job ALTER COLUMN metadata SET DEFAULT '{}';
UPDATE river.river_job SET metadata = '{}' WHERE metadata IS NULL;
ALTER TABLE river.river_job ALTER COLUMN metadata SET NOT NULL;

-- The 'pending' job state will be used for upcoming functionality:
ALTER TYPE river.river_job_state ADD VALUE IF NOT EXISTS 'pending' AFTER 'discarded';

ALTER TABLE river.river_job DROP CONSTRAINT finalized_or_finalized_at_null;
ALTER TABLE river.river_job ADD CONSTRAINT finalized_or_finalized_at_null CHECK (
    (finalized_at IS NULL AND state NOT IN ('cancelled', 'completed', 'discarded')) OR
    (finalized_at IS NOT NULL AND state IN ('cancelled', 'completed', 'discarded'))
);

DROP TRIGGER river_notify ON river.river_job;
DROP FUNCTION river.river_job_notify;

CREATE TABLE river.river_queue (
    name text PRIMARY KEY NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    metadata jsonb NOT NULL DEFAULT '{}' ::jsonb,
    paused_at timestamptz,
    updated_at timestamptz NOT NULL
);

ALTER TABLE river.river_leader
    ALTER COLUMN name SET DEFAULT 'default',
    DROP CONSTRAINT name_length,
    ADD CONSTRAINT name_length CHECK (name = 'default');

-- River main migration 005 [up]
DO
$body$
BEGIN
    IF (SELECT to_regclass('river.river_migration') IS NOT NULL) THEN
        ALTER TABLE river.river_migration
            RENAME TO river_migration_old;

        CREATE TABLE river.river_migration(
            line TEXT NOT NULL,
            version bigint NOT NULL,
            created_at timestamptz NOT NULL DEFAULT NOW(),
            CONSTRAINT line_length CHECK (char_length(line) > 0 AND char_length(line) < 128),
            CONSTRAINT version_gte_1 CHECK (version >= 1),
            PRIMARY KEY (line, version)
        );

        INSERT INTO river.river_migration
            (created_at, line, version)
        SELECT created_at, 'main', version
        FROM river.river_migration_old;

        DROP TABLE river.river_migration_old;
    END IF;
END;
$body$
LANGUAGE 'plpgsql';

ALTER TABLE river.river_job
    ADD COLUMN IF NOT EXISTS unique_key bytea;

CREATE UNIQUE INDEX IF NOT EXISTS river_job_kind_unique_key_idx ON river.river_job (kind, unique_key) WHERE unique_key IS NOT NULL;

CREATE UNLOGGED TABLE river.river_client (
    id text PRIMARY KEY NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    metadata jsonb NOT NULL DEFAULT '{}',
    paused_at timestamptz,
    updated_at timestamptz NOT NULL,
    CONSTRAINT name_length CHECK (char_length(id) > 0 AND char_length(id) < 128)
);

CREATE UNLOGGED TABLE river.river_client_queue (
    river_client_id text NOT NULL REFERENCES river.river_client (id) ON DELETE CASCADE,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    max_workers bigint NOT NULL DEFAULT 0,
    metadata jsonb NOT NULL DEFAULT '{}',
    num_jobs_completed bigint NOT NULL DEFAULT 0,
    num_jobs_running bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (river_client_id, name),
    CONSTRAINT name_length CHECK (char_length(name) > 0 AND char_length(name) < 128),
    CONSTRAINT num_jobs_completed_zero_or_positive CHECK (num_jobs_completed >= 0),
    CONSTRAINT num_jobs_running_zero_or_positive CHECK (num_jobs_running >= 0)
);

-- River main migration 006 [up]
CREATE OR REPLACE FUNCTION river.river_job_state_in_bitmask(bitmask BIT(8), state river.river_job_state)
RETURNS boolean
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT CASE state
        WHEN 'available' THEN get_bit(bitmask, 7)
        WHEN 'cancelled' THEN get_bit(bitmask, 6)
        WHEN 'completed' THEN get_bit(bitmask, 5)
        WHEN 'discarded' THEN get_bit(bitmask, 4)
        WHEN 'pending'   THEN get_bit(bitmask, 3)
        WHEN 'retryable' THEN get_bit(bitmask, 2)
        WHEN 'running'   THEN get_bit(bitmask, 1)
        WHEN 'scheduled' THEN get_bit(bitmask, 0)
        ELSE 0
    END = 1;
$$;

ALTER TABLE river.river_job ADD COLUMN IF NOT EXISTS unique_states BIT(8);

CREATE UNIQUE INDEX IF NOT EXISTS river_job_unique_idx ON river.river_job (unique_key)
    WHERE unique_key IS NOT NULL
      AND unique_states IS NOT NULL
      AND river.river_job_state_in_bitmask(unique_states, state);

DROP INDEX river.river_job_kind_unique_key_idx;
