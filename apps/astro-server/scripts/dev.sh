#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

# Ensure .env exists
if [ ! -f .env ]; then
  echo "==> No .env found, copying .env.example..."
  cp .env.example .env
fi

# Load DATABASE_URL from .env
DATABASE_URL=$(grep '^DATABASE_URL=' .env | cut -d= -f2-)
if [ -z "$DATABASE_URL" ]; then
  echo "ERROR: DATABASE_URL not set in .env"
  exit 1
fi

# Default ENVIRONMENT to "local" for dev
export ENVIRONMENT="${ENVIRONMENT:-local}"

# Ensure air is installed
if ! command -v air &>/dev/null; then
  echo "==> Installing air (hot reload)..."
  go install github.com/air-verse/air@latest
fi

# Ensure atlas is installed
if ! command -v atlas &>/dev/null; then
  echo "==> Installing atlas..."
  curl -sSf https://atlasgo.sh | sh
fi

# Apply schema via Atlas (idempotent — safe to re-run)
echo "==> Applying schema..."
case "$DATABASE_URL" in *\?*) sep="&" ;; *) sep="?" ;; esac
atlas schema apply \
  --url "${DATABASE_URL}${sep}search_path=public" \
  --to "file://../../sql/astro-server/schema.sql" \
  --dev-url "docker://postgres/16/dev?search_path=public" \
  --exclude atlas_schema_revisions \
  --exclude river \
  --auto-approve

# Backfill undeployed_at for soft-deletes that predate the bugfix making
# UpdateStatus(StatusUndeployed) stamp the timestamp in the same UPDATE.
# Before the fix, the undeploy worker called MarkUndeployedByID after
# transitioning to 'undeployed', and that helper's `WHERE status='active'`
# guard always failed in the normal flow — so undeployed_at stayed NULL
# for every soft-delete. status_changed_at carries the same moment, so
# it's a safe proxy. Idempotent: after the first run the WHERE matches
# zero rows.
echo "==> Backfilling undeployed_at for legacy soft-deletes..."
psql "$DATABASE_URL" -c "UPDATE deployments SET undeployed_at = status_changed_at WHERE status = 'undeployed' AND undeployed_at IS NULL;" >/dev/null

# Apply River queue migrations (idempotent — CREATE IF NOT EXISTS)
echo "==> Applying River migrations..."
atlas migrate apply \
  --url "$DATABASE_URL" \
  --dir "file://../../sql/river" \
  --revisions-schema atlas_schema_revisions \
  --allow-dirty

# Bootstrap OpenMeter (idempotent — 409s treated as success).
# astro-server's startup ValidateMeters check refuses to boot when
# expected meters are missing, so we seed them before launching air.
# The bootstrap script itself short-circuits when OPENMETER_URL is
# unset, so this is a no-op for setups that don't run a local
# OpenMeter instance.
OPENMETER_URL_VALUE=$(grep '^OPENMETER_URL=' .env 2>/dev/null | cut -d= -f2- | tr -d "\"'" | tr -d '[:space:]')
if [ -n "$OPENMETER_URL_VALUE" ]; then
  OPENMETER_URL="$OPENMETER_URL_VALUE" bash ../../scripts/bootstrap-openmeter.sh

  # Backfill subscriptions for any existing accounts that have an
  # openmeter customer record but no active plan attached. Older
  # accounts ended up in this state because OPENMETER_DEFAULT_PLAN
  # was unset at the time they were created; without a subscription
  # the entitlement check rejects Deployments etc. The script reads
  # DATABASE_URL / OPENMETER_URL / OPENMETER_DEFAULT_PLAN out of the
  # same .env so we don't need to pass anything explicitly.
  bash ../../scripts/backfill-openmeter-subscriptions.sh
fi

# exec so air replaces bash and receives parent signals directly — no trap
# hop, so air has time to kill its astro-server child (which lives in its
# own process group via Setpgid:true) before we exit.
echo "==> Starting astro-server (hot reload via air)..."
exec air
