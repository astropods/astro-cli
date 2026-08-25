#!/usr/bin/env bash
# Every package runs whole. Billing test names in a shared package share no
# common token, so a name filter silently dropped TestSelfLimitReached,
# TestCollectAfterCard, and TestNoopProviderIsNotAProvisioner.
#
# Postgres-backed billing tests are behind //go:build integration:
# `moon run astro-server:test-integration`.
set -euo pipefail

cd "$(dirname "$0")/.."

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
)

COVER_PROFILE="${COVER_PROFILE:-/tmp/astro-billing.cov}"
OWNED_PROFILE=$(mktemp -t astro-billing-owned)
SHARED_PROFILE=$(mktemp -t astro-billing-shared)
trap 'rm -f "$OWNED_PROFILE" "$SHARED_PROFILE"' EXIT
COVERPKG=$(IFS=,; echo "${OWNED[*]},${SHARED[*]}")
BILLING_RE='billing|metronome|quota|entitle|suspend|meter|aigateway|payment'

runner=(go test)
if command -v gotestsum >/dev/null 2>&1; then
  runner=(gotestsum --format testdox --hide-summary skipped --)
fi

echo "==> billing-owned packages"
"${runner[@]}" -coverprofile="$OWNED_PROFILE" -coverpkg="$COVERPKG" "${OWNED[@]}"

echo
echo "==> packages that host a billing primitive"
"${runner[@]}" -coverprofile="$SHARED_PROFILE" -coverpkg="$COVERPKG" "${SHARED[@]}"

# Both runs measure the same -coverpkg set, so every block appears twice. The
# counts are summed per block rather than concatenated.
{
  head -1 "$OWNED_PROFILE"
  awk 'FNR > 1 {
         if (!($1 in seen)) { order[++n] = $1; stmts[$1] = $2; seen[$1] = 1 }
         count[$1] += $3
       }
       END { for (i = 1; i <= n; i++) print order[i], stmts[order[i]], count[order[i]] }' \
    "$OWNED_PROFILE" "$SHARED_PROFILE"
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
