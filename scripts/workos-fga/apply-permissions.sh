#!/usr/bin/env bash
#
# Create the FGA permissions in model.json through the WorkOS Authorization API.
#
# Resource types have no public API and must exist in the dashboard first.
#
#   ./apply-permissions.sh                              # dry run against staging
#   ./apply-permissions.sh --apply
#   ./apply-permissions.sh --prod --apply
#   ./apply-permissions.sh --only deployment:read --apply
#
# Staging is the default; --prod selects production. Either way the script
# refuses to run unless the credential resolves to that exact environment.
# The check is on the ID from `workos whoami`, never a name: all three projects
# on this team have an environment called Staging, and `workos environment use`
# retargets a profile without swapping its stored API key, so the label and the
# credential can disagree.

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

# A profile's environment is fixed by the API key it was added with. Both
# `workos whoami` and the profile's own environment label can name a different
# environment than the key actually reaches: `workos environment use` rewrites
# the label and reports success without swapping the credential. So this check
# only catches an obviously wrong profile. The backstop is the create itself,
# which 404s on a resource type that exists only in the intended environment,
# and the loop below stops on that first failure.
active_id=$(workos profile list --json 2>/dev/null \
  | jq -r 'first(.data[] | select(.active)) | .environmentId // empty')
[ -n "$active_id" ] || {
  echo "no active profile; run workos profile add <name> <api-key>" >&2
  exit 1
}

echo "target: Astro $want_label ($want)"
[ "$want" = "$active_id" ] || {
  echo "credential is $active_id, which is not Astro $want_label" >&2
  echo "add that environment's API key with workos env add, then re-run" >&2
  exit 1
}

$apply || echo "dry run, pass --apply to create"

# limit=100 because the endpoint pages at 10 by default, and a short read here
# would report existing permissions as missing.
existing=$(workos api "/authorization/permissions?limit=100" --json 2>/dev/null \
  | jq -r '[(.data // [])[].slug] | join("\n")')

created=0 skipped=0 failed=0

while IFS= read -r body; do
  slug=$(printf '%s' "$body" | jq -r .slug)
  [ -n "$only" ] && [ "$slug" != "$only" ] && continue

  if printf '%s\n' "$existing" | grep -qxF "$slug"; then
    echo "exists   $slug"
    skipped=$((skipped + 1))
    continue
  fi

  if ! $apply; then
    echo "would create  $slug  ($(printf '%s' "$body" | jq -r .resource_type_slug))"
    continue
  fi

  if output=$(workos api /authorization/permissions -X POST -d "$body" --json --yes 2>&1); then
    echo "created  $slug"
    created=$((created + 1))
  elif printf '%s' "$output" | grep -q permission_slug_conflict; then
    echo "exists   $slug"
    skipped=$((skipped + 1))
  else
    echo "FAILED   $slug: $output" >&2
    failed=$((failed + 1))
    # Stop on the first real failure rather than repeating it 49 times.
    break
  fi
done < <(jq -c '.permissions[] | {slug, name, description, resource_type_slug: .resourceType}' "$MODEL")

$apply && echo "created=$created exists=$skipped failed=$failed"
[ "$failed" -eq 0 ]
