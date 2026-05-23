#!/bin/bash

set -e

echo "🔄 Updating all git submodules to latest main/head..."
echo

# Use --remote so each submodule lands on the tip of its remote-tracking
# branch (typically origin/main), NOT on the SHA recorded in the superproject.
# This is robust against pointers that reference SHAs which were never pushed
# — someone commits a submodule bump locally without pushing the underlying
# commit, and every subsequent `update --init` fails with
# "upload-pack: not our ref". --remote sidesteps that entirely.
echo "📥 Initializing submodules and moving to latest remote head..."
git submodule update --init --remote --recursive

echo
echo "✅ Final submodule status:"
git submodule status

# When --remote moves a submodule past the superproject's recorded pointer,
# `git submodule status` flags it with a leading + (or - if behind).
# Surface that so the user commits the bump. When nothing diverged, stay
# silent — no point telling the user to commit when there's nothing to commit.
echo
divergent=$(git submodule status | grep -E '^[+-]' || true)
if [ -n "$divergent" ]; then
  echo "⚠️  Superproject pointers differ from submodule working trees:"
  echo "$divergent" | sed 's/^/   /'
  echo
  echo "   To commit these bumps in the superproject:"
  echo '     git add modules && git commit -m "chore: update all submodules to latest main"'
  echo
fi

echo "🎉 Done."
