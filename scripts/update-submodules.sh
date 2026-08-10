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
      echo "              ignoring the recorded pointers. Afterward the script"
      echo "              prints the exact 'git add -- <paths>' command needed"
      echo "              to record the new SHAs (only the paths that moved)."
      exit 0
      ;;
    *)
      echo "Unknown argument: $arg" >&2
      echo "Run '$0 --help' for usage." >&2
      exit 2
      ;;
  esac
done

# The submodule paths declared in .gitmodules — the ONLY paths that should ever
# be staged as gitlinks. Staging `modules/` wholesale sweeps in stray on-disk
# repos (e.g. submodules removed from .gitmodules whose checkouts still linger)
# as phantom gitlinks, which then break `git submodule status`.
submodule_paths() {
  git config -f .gitmodules --get-regexp '^submodule\..+\.path$' | awk '{print $2}'
}

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

# Flag orphaned submodule checkouts: on-disk repos under the submodule parent
# dirs that are NOT declared in .gitmodules. Left in place they get staged as
# phantom gitlinks and make `git submodule status` fail once committed.
orphans=""
declared=$(submodule_paths)
for d in modules/*/ packages/*/; do
  p="${d%/}"
  [ -e "$p/.git" ] || continue
  printf '%s\n' "$declared" | grep -qxF "$p" || orphans="$orphans $p"
done
if [ -n "$orphans" ]; then
  echo "⚠️  Orphaned checkouts not in .gitmodules:$orphans"
  echo "   Remove each before staging so it isn't committed as a phantom gitlink:"
  for p in $orphans; do echo "     rm -rf \"$p\" \".git/modules/$p\""; done
  echo
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
  # Stage ONLY the paths that actually moved — never `git add modules`, which
  # also sweeps in any stray/orphaned checkout as a phantom gitlink and misses
  # submodules outside modules/ (e.g. packages/astro-spec).
  divergent_paths=$(echo "$divergent" | awk '{print $2}' | tr '\n' ' ')
  if [ "$LATEST" -eq 1 ]; then
    echo "   These paths were advanced to their remote HEAD."
    echo "   To record the new pointers:"
    echo "     git add -- ${divergent_paths}&& git commit -m \"chore: bump submodules to latest\""
  else
    echo "   These paths had invalid or unpushed SHAs recorded in the superproject."
    echo "   To record the working-tree fixes:"
    echo "     git add -- ${divergent_paths}&& git commit -m \"chore: fix stale submodule pointers\""
  fi
  echo
fi

echo "🎉 Done."
