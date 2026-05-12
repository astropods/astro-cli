#!/usr/bin/env bash
set -euo pipefail

POSTMAN=false
UI=false
for arg in "$@"; do
  case $arg in
    --postman) POSTMAN=true ;;
    --ui) UI=true ;;
  esac
done

: "${ASTRO_ENV:=dev}"
: "${ASTRO_TEST_HOST:=http://localhost}"
: "${ASTRO_TEST_EMAIL:?ASTRO_TEST_EMAIL is required}"
: "${ASTRO_TEST_PASSWORD:?ASTRO_TEST_PASSWORD is required}"
if [ "${ASTRO_ENV}" = "dev" ]; then
  : "${ASTRO_TEST_USERNAME:?ASTRO_TEST_USERNAME is required in dev mode}"
else
  : "${ASTRO_TEST_USERNAME:=}"
fi

export ASTRO_ENV ASTRO_TEST_HOST ASTRO_TEST_EMAIL ASTRO_TEST_PASSWORD ASTRO_TEST_USERNAME

CONFIG="apps/tests/smoke/playwright.smoke.config.ts"
PW_CMD="bunx playwright test --config=$CONFIG"

if $UI; then
  PW_CMD="$PW_CMD --ui"
fi

if $POSTMAN; then
  CI=true postman app test --command "$PW_CMD"
else
  eval "$PW_CMD"
fi
