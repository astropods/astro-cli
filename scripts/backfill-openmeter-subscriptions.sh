#!/usr/bin/env bash
# Backfill OpenMeter subscriptions for accounts that have a customer
# record but no active subscription.
#
# Context: before OPENMETER_DEFAULT_PLAN was set in .env, the
# auto-subscribe block in handlers/accounts.go and the
# openmeter_backfill worker both silently skipped subscription
# creation. That left a population of accounts with an
# openmeter_customer_id but no plan attached — entitlement checks
# then reject Deployments etc. The customer-creation backfill that
# runs in-process only handles accounts MISSING a customer, not
# customers missing a subscription.
#
# This script closes that gap. For each account with an
# openmeter_customer_id, it checks whether the customer has an active
# subscription and creates one against the configured plan if not.
#
# Usage:
#   DATABASE_URL=postgres://... \
#   OPENMETER_URL=https://meter.example.com \
#   OPENMETER_DEFAULT_PLAN=private_beta \
#     bash scripts/backfill-openmeter-subscriptions.sh
#
# All three env vars can also be loaded from apps/astro-server/.env
# when the script is run from the repo root — it auto-sources the
# values if the vars are unset.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# ── env loading ───────────────────────────────────────────────────────────────

# Pull DATABASE_URL / OPENMETER_URL / OPENMETER_DEFAULT_PLAN from
# apps/astro-server/.env when the caller hasn't passed them, so the
# script is one-command-runnable from the repo root.
load_env() {
  local var="$1"
  if [ -z "${!var:-}" ] && [ -f apps/astro-server/.env ]; then
    local v
    v=$(grep "^${var}=" apps/astro-server/.env | head -1 | cut -d= -f2- | tr -d "\"'" | tr -d '[:space:]')
    [ -n "$v" ] && export "$var=$v"
  fi
}
load_env DATABASE_URL
load_env OPENMETER_URL
load_env OPENMETER_DEFAULT_PLAN

PLAN_KEY="${OPENMETER_DEFAULT_PLAN:-private_beta}"

if [ -z "${DATABASE_URL:-}" ]; then
  echo "ERROR: DATABASE_URL is not set (and not found in apps/astro-server/.env)" >&2
  exit 1
fi
if [ -z "${OPENMETER_URL:-}" ]; then
  echo "ERROR: OPENMETER_URL is not set (and not found in apps/astro-server/.env)" >&2
  exit 1
fi
if ! command -v psql >/dev/null 2>&1; then
  # Don't fail the caller — most often this script is invoked from
  # dev.sh on startup, and a missing psql shouldn't block the server
  # from booting. Print a hint and skip cleanly. Users who actually
  # need the backfill can `brew install libpq` (Mac) or the
  # equivalent on their platform.
  echo "==> psql not found — skipping OpenMeter subscription backfill."
  echo "    Install the postgres client to enable (Mac: brew install libpq)."
  exit 0
fi

echo "==> Backfilling OpenMeter subscriptions"
echo "    OPENMETER_URL=$OPENMETER_URL"
echo "    plan=$PLAN_KEY"

# ── account enumeration ──────────────────────────────────────────────────────

# All accounts that already have an OpenMeter customer ID. We use the
# openmeter customer ID directly when talking to OpenMeter; account.id
# is only carried for log lines.
rows=$(psql "$DATABASE_URL" -t -A -F$'\t' -c "
  SELECT id, name, openmeter_customer_id
  FROM accounts
  WHERE openmeter_customer_id IS NOT NULL
  ORDER BY created_at
" 2>/dev/null)

if [ -z "$rows" ]; then
  echo "==> No accounts with an openmeter_customer_id — nothing to do."
  exit 0
fi

total=$(printf '%s\n' "$rows" | grep -c .)
echo "==> $total account(s) with an OpenMeter customer to check"

created=0
already=0
failed=0

# ── per-account subscription check + create ──────────────────────────────────

while IFS=$'\t' read -r account_id account_name customer_id; do
  [ -z "$account_id" ] && continue

  # Active subscriptions for this customer. The response is a paginated
  # envelope; an empty `items` array means no active subscription.
  status_code=$(curl -s -o /tmp/om_subs -w "%{http_code}" \
    "$OPENMETER_URL/api/v1/customers/$customer_id/subscriptions?status=active" || echo "000")

  case "$status_code" in
    200) ;;
    404)
      # Customer doesn't exist in OpenMeter but we have an ID stored —
      # this is an inconsistency the customer-backfill worker should
      # eventually fix. Flag it for visibility, skip the subscribe.
      echo "  [SKIP] $account_name ($account_id): customer $customer_id not found in OpenMeter"
      failed=$((failed + 1))
      continue
      ;;
    *)
      echo "  [FAIL] $account_name ($account_id): list subscriptions returned HTTP $status_code"
      failed=$((failed + 1))
      continue
      ;;
  esac

  # Has any active subscription → skip. The empty-items detection is a
  # substring check on "\"items\":[]" because the alternative shapes
  # ("items":[ {...} ] or with whitespace) always include at least one
  # object character after `[`.
  if grep -q '"items":\[\]' /tmp/om_subs; then
    has_active=0
  else
    has_active=1
  fi

  if [ "$has_active" -eq 1 ]; then
    echo "  [keep] $account_name ($account_id): already has an active subscription"
    already=$((already + 1))
    continue
  fi

  # Create the subscription.
  status_code=$(curl -s -o /tmp/om_out -w "%{http_code}" \
    -X POST "$OPENMETER_URL/api/v1/subscriptions" \
    -H "Content-Type: application/json" \
    -d "{\"customerId\":\"$customer_id\",\"plan\":{\"key\":\"$PLAN_KEY\"}}" || echo "000")

  case "$status_code" in
    200|201)
      echo "  [ ok ] $account_name ($account_id): subscribed to $PLAN_KEY"
      created=$((created + 1))
      ;;
    *)
      echo "  [FAIL] $account_name ($account_id): create subscription returned HTTP $status_code — $(cat /tmp/om_out 2>/dev/null)"
      failed=$((failed + 1))
      ;;
  esac
done <<< "$rows"

# ── summary ──────────────────────────────────────────────────────────────────

echo ""
echo "==> Done."
echo "    subscribed: $created"
echo "    untouched:  $already"
echo "    failed:     $failed"

# Non-zero exit if anything failed so CI / wrapper scripts can detect it.
[ "$failed" -eq 0 ]
