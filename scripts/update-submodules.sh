#!/bin/bash

set -e

LATEST=0
for arg in "$@"; do
  case "$arg" in
    --latest) LATEST=1 ;;
    -h|--help)
      echo "Usage: $0 [--latest]"
      echo
      echo "  (default)   Check out the commits recorded in the superproject,"
      echo "              repairing any pointers missing from their remote."
      echo "  --latest    Advance every submodule to its remote branch HEAD,"
      echo "              ignoring the recorded pointers (run 'git add modules"
      echo "              && git commit' afterward to record the new SHAs)."
      exit 0
      ;;
    *)
      echo "Unknown argument: $arg" >&2
      echo "Run '$0 --help' for usage." >&2
      exit 2
      ;;
  esac
done

echo "🔄 Syncing and updating git submodules..."
echo

git submodule sync --recursive

if [ "$LATEST" -eq 1 ]; then
  echo "📥 Advancing all submodules to remote branch HEAD..."
  git submodule update --init --remote --recursive
else
  echo "📥 Checking out superproject-recorded commits..."
  set +e
  update_output=$(git submodule update --init --recursive 2>&1)
  update_status=$?
  set -e

  if [ "$update_status" -ne 0 ]; then
    echo "$update_output"
    echo
    echo "⚠️  Some submodule pointers are stale or invalid; using remote HEAD for those paths only..."
    echo

    while IFS= read -r path; do
      recorded=$(git ls-tree HEAD "$path" | awk '{print $3}')
      if [ -z "$recorded" ]; then
        continue
      fi

      git submodule update --init "$path" >/dev/null 2>&1 || true

      if git -C "$path" cat-file -e "$recorded^{commit}" 2>/dev/null; then
        git -C "$path" checkout --quiet "$recorded"
        continue
      fi

      echo "  → $path (recorded $recorded missing from remote; checking out origin/HEAD)"
      git submodule update --init --remote "$path"
    done < <(git config -f .gitmodules --get-regexp '^submodule\..+\.path$' | awk '{print $2}')
  fi
fi

echo
echo "✅ Final submodule status:"
git submodule status

# When a fallback moved a submodule past the superproject's recorded pointer,
# `git submodule status` flags it with a leading + (or - if behind).
echo
divergent=$(git submodule status | grep -E '^[+-]' || true)
if [ -n "$divergent" ]; then
  echo "⚠️  Superproject pointers differ from submodule working trees:"
  echo "$divergent" | sed 's/^/   /'
  echo
  if [ "$LATEST" -eq 1 ]; then
    echo "   These paths were advanced to their remote HEAD."
    echo "   To record the new pointers:"
    echo '     git add modules && git commit -m "chore: bump submodules to latest"'
  else
    echo "   These paths had invalid or unpushed SHAs recorded in the superproject."
    echo "   To record the working-tree fixes:"
    echo '     git add modules && git commit -m "chore: fix stale submodule pointers"'
  fi
  echo
fi

echo "🎉 Done."
