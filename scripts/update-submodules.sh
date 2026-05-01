#!/bin/bash

set -e

echo "🔄 Updating all git submodules to latest main/head..."
echo

# Initialize any uninitialized submodules
echo "📥 Initializing submodules..."
git submodule update --init --recursive

echo
echo "🌿 Checking out main branch for all submodules..."
git submodule foreach 'git checkout main'

echo
echo "⬇️  Pulling latest changes for all submodules..."
git submodule foreach 'git pull origin main'

echo
echo "✅ Final submodule status:"
git submodule status

echo
echo "🎉 All submodules updated successfully!"
echo
echo "💡 To commit these changes to the parent repository, run:"
echo "   git add modules && git commit -m \"chore: update all submodules to latest main\""
