-- Rename the blueprint-count quota key from "agents" to "blueprints" so any
-- per-account override rows keep applying after the code-side rename. The agents
-- table the quota counts is unchanged; only this stored resource value moves.
--
-- Guarded on table existence so an isolated replay of this directory (which does
-- not create account_limits; the declarative schema does) is a clean no-op. The
-- declarative schema is applied before these migrations in every environment, so
-- the table exists at real apply time.
DO $$
BEGIN
  IF to_regclass('public.account_limits') IS NOT NULL THEN
    UPDATE account_limits SET resource = 'blueprints' WHERE resource = 'agents';
  END IF;
END $$;
