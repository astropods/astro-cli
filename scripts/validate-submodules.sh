#!/bin/bash

set -euo pipefail

ref="${1:-HEAD}"
failures=0

echo "🔍 Validating submodule pointers at $ref..."
echo

entries=$(mktemp)
trap 'rm -f "$entries"' EXIT

while read -r section path; do
  recorded=$(git ls-tree "$ref" "$path" 2>/dev/null | awk '{print $3}')
  if [ -z "$recorded" ]; then
    echo "❌ $path: no submodule gitlink at $ref"
    failures=$((failures + 1))
    continue
  fi

  url=$(git config -f .gitmodules --get "submodule.${section}.url")
  printf '%s %s %s\n' "$recorded" "$path" "$url" >>"$entries"
  echo "✅ $path @ ${recorded:0:12}"
done < <(git config -f .gitmodules --get-regexp '^submodule\..+\.path$' | awk '{gsub(/^submodule\./, "", $1); gsub(/\.path$/, "", $1); print $1, $2}')

while read -r sha path_a url_a path_b url_b; do
  echo "❌ duplicate SHA $sha shared by $path_a and $path_b"
  echo "   $path_a → $url_a"
  echo "   $path_b → $url_b"
  if [ "$url_a" != "$url_b" ]; then
    echo "   hint: different remotes — likely cross-repo pointer mix-up during a bulk bump"
  fi
  failures=$((failures + 1))
done < <(awk '
  {
    sha = $1
    path = $2
    url = $3
    if (sha in first_path) {
      print sha, first_path[sha], first_url[sha], path, url
    } else {
      first_path[sha] = path
      first_url[sha] = url
    }
  }
' "$entries")

echo
if [ "$failures" -gt 0 ]; then
  echo "❌ $failures submodule pointer issue(s)."
  echo "   After fixing, run: bash scripts/update-submodules.sh"
  exit 1
fi

echo "✅ All submodule pointers look consistent."
