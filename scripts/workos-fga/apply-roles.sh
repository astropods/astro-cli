#!/usr/bin/env bash
#
# Create the FGA roles in model.json and set their permissions, through the
# WorkOS Authorization API.
#
# Run apply-permissions.sh first: a role cannot reference a permission that does
# not exist yet.
#
#   ./apply-roles.sh                                # dry run against staging
#   ./apply-roles.sh --apply
#   ./apply-roles.sh --prod --apply
#   ./apply-roles.sh --only deployment-viewer --apply
#
# Permissions are always set, not just on create, so a re-run repairs a role
# whose permission list has drifted from model.json.
#
# Staging is the default; --prod selects production. See apply-permissions.sh
# for why the environment check is weak and what actually backstops it.

set -euo pipefail

# Astro AI's Project.
STAGING_ENV_ID=environment_01K1VMRD87VEESRC9DZ17PZ4CM
PROD_ENV_ID=environment_01K1VMRDS4NZ8KW17C1WME0FAE

MODEL="$(dirname "$0")/model.json"
apply=false
only=""
want=$STAGING_ENV_ID
want_label=staging

while [ $# -gt 0 ]; do
  case "$1" in
    --apply) apply=true ;;
    --prod) want=$PROD_ENV_ID; want_label=production ;;
    --only) only="${2:?--only needs a slug}"; shift ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
  shift
done

command -v workos >/dev/null || { echo "workos CLI not found" >&2; exit 1; }
command -v jq >/dev/null || { echo "jq not found" >&2; exit 1; }

active_id=$(workos profile list --json 2>/dev/null \
  | jq -r 'first(.data[] | select(.active)) | .environmentId // empty')
[ -n "$active_id" ] || {
  echo "no active profile; run workos profile add <name> <api-key>" >&2
  exit 1
}

echo "target: Astro $want_label ($want)"
[ "$want" = "$active_id" ] || {
  echo "credential is $active_id, which is not Astro $want_label" >&2
  echo "add that environment's API key with workos profile add, then re-run" >&2
  exit 1
}

$apply || echo "dry run, pass --apply to create"

existing=$(workos api "/authorization/roles?limit=100" --json 2>/dev/null \
  | jq -r '[(.data // [])[].slug] | join("\n")')

created=0 updated=0 failed=0

while IFS= read -r role; do
  slug=$(printf '%s' "$role" | jq -r .slug)
  [ -n "$only" ] && [ "$slug" != "$only" ] && continue

  perms=$(printf '%s' "$role" | jq -c '{permissions}')
  count=$(printf '%s' "$role" | jq '.permissions | length')

  if ! $apply; then
    printf 'would set  %-28s %-16s %s permissions\n' \
      "$slug" "$(printf '%s' "$role" | jq -r .resource_type_slug)" "$count"
    continue
  fi

  if printf '%s\n' "$existing" | grep -qxF "$slug"; then
    echo "exists   $slug"
  else
    body=$(printf '%s' "$role" | jq -c '{slug, name, description, resource_type_slug}')
    if output=$(workos api /authorization/roles -X POST -d "$body" --json --yes 2>&1); then
      echo "created  $slug"
      created=$((created + 1))
    elif printf '%s' "$output" | grep -q http_409; then
      echo "exists   $slug"
    else
      echo "FAILED   $slug: $output" >&2
      failed=$((failed + 1))
      break
    fi
  fi

  if output=$(workos api "/authorization/roles/$slug/permissions" \
        -X PUT -d "$perms" --json --yes 2>&1); then
    echo "         $slug <- $count permissions"
    updated=$((updated + 1))
  else
    echo "FAILED   $slug permissions: $output" >&2
    failed=$((failed + 1))
    break
  fi
done < <(jq -c '.roles[] | {slug, name, description, resource_type_slug: .resourceType, permissions}' "$MODEL")

$apply && echo "created=$created permissions_set=$updated failed=$failed"
[ "$failed" -eq 0 ]
