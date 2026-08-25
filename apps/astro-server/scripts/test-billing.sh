#!/usr/bin/env bash
# Every package runs whole. Billing test names in a shared package share no
# common token, so a name filter silently dropped TestSelfLimitReached,
# TestCollectAfterCard, and TestNoopProviderIsNotAProvisioner.
#
# Postgres-backed billing tests are behind //go:build integration. Set
# DATABASE_URL and they run here too, folded into the same coverage number;
# without it the run is partial and says so at the end, because a pass that
# quietly skipped the SQL-backed paths reads exactly like a full one.
# BILLING_REQUIRE_DB=1 turns that skip into a failure.
set -euo pipefail

cd "$(dirname "$0")/.."

# Checked before anything runs: a suite told to require the database should say
# so immediately, not after a passing unit run.
if [ -n "${BILLING_REQUIRE_DB:-}" ] && [ -z "${DATABASE_URL:-}" ]; then
  echo "BILLING_REQUIRE_DB is set but DATABASE_URL is not, so the SQL-backed" >&2
  echo "billing tests cannot run and this suite cannot pass." >&2
  exit 1
fi

OWNED=(
  ./internal/billing/...
  ./internal/quota/...
  ./internal/payment/...
  ./internal/aigateway/...
  ./internal/riverqueue/...
)

# An unrelated failure here fails the billing suite: the price of not filtering.
SHARED=(
  ./handlers/...
  ./internal/middleware/...
  ./internal/k8s/...
  ./internal/account/...
)

# Packages holding Postgres-backed billing tests. Explicit rather than the whole
# set: integration tests elsewhere want a cluster, not just a database.
INTEGRATION=(
  ./internal/account/...
)

COVER_PROFILE="${COVER_PROFILE:-/tmp/astro-billing.cov}"
OWNED_PROFILE=$(mktemp -t astro-billing-owned)
SHARED_PROFILE=$(mktemp -t astro-billing-shared)
INTEGRATION_PROFILE=$(mktemp -t astro-billing-integration)
trap 'rm -f "$OWNED_PROFILE" "$SHARED_PROFILE" "$INTEGRATION_PROFILE"' EXIT
COVERPKG=$(IFS=,; echo "${OWNED[*]},${SHARED[*]}")
# Matches a function name or the file path holding it. budget/exempt/ceiling are
# here because the gateway ceiling functions carry none of the other words, so
# the report counted them as absent rather than uncovered.
BILLING_RE='billing|metronome|quota|entitle|suspend|meter|aigateway|payment|budget|exempt|ceiling'

runner=(go test)
if command -v gotestsum >/dev/null 2>&1; then
  runner=(gotestsum --format testdox --hide-summary skipped --)
fi

echo "==> billing-owned packages"
"${runner[@]}" -coverprofile="$OWNED_PROFILE" -coverpkg="$COVERPKG" "${OWNED[@]}"

echo
echo "==> packages that host a billing primitive"
"${runner[@]}" -coverprofile="$SHARED_PROFILE" -coverpkg="$COVERPKG" "${SHARED[@]}"

PROFILES=("$OWNED_PROFILE" "$SHARED_PROFILE")
DB_SKIPPED=
echo
if [ -n "${DATABASE_URL:-}" ]; then
  echo "==> Postgres-backed billing tests"
  "${runner[@]}" -tags integration -coverprofile="$INTEGRATION_PROFILE" \
    -coverpkg="$COVERPKG" "${INTEGRATION[@]}"
  PROFILES+=("$INTEGRATION_PROFILE")
else
  DB_SKIPPED=1
  echo "==> skipping Postgres-backed billing tests: DATABASE_URL is unset."
  echo "    ${INTEGRATION[*]} hold them; they will read as uncovered below."
fi

# Every run measures the same -coverpkg set, so each block appears once per run.
# The counts are summed per block rather than concatenated.
{
  head -1 "$OWNED_PROFILE"
  awk 'FNR > 1 {
         if (!($1 in seen)) { order[++n] = $1; stmts[$1] = $2; seen[$1] = 1 }
         count[$1] += $3
       }
       END { for (i = 1; i <= n; i++) print order[i], stmts[order[i]], count[order[i]] }' \
    "${PROFILES[@]}"
} > "$COVER_PROFILE"

echo
echo "==> coverage of billing sources"
go tool cover -func="$COVER_PROFILE" \
  | grep -iE "$BILLING_RE" \
  | awk '
      { gsub(/%/, "", $3); total++; sum += $3; if ($3 + 0 == 0) uncovered++ }
      END {
        if (total == 0) { print "no billing statements in the profile"; exit 1 }
        printf "  functions:  %d\n  uncovered:  %d\n  mean stmt coverage: %.1f%%\n", total, uncovered+0, sum/total
      }'

echo
echo "Uncovered billing functions (top 20):"
go tool cover -func="$COVER_PROFILE" \
  | grep -iE "$BILLING_RE" \
  | awk '{ gsub(/%/, "", $3); if ($3 + 0 == 0) print "  " $1 " " $2 }' \
  | sed 's#github.com/astropods/astro/apps/astro-server/##' \
  | head -20

if [ -n "$DB_SKIPPED" ]; then
  echo
  echo "PARTIAL: the Postgres-backed billing tests did not run, so any SQL-backed"
  echo "path above is unmeasured rather than untested. Run this with DATABASE_URL"
  echo "set, or 'moon run astro-server:test-integration', before trusting the"
  echo "coverage number."
fi
