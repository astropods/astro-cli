#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Publishable packages in dependency order
# Packages at the same level can be published in parallel, but we do sequential for reliability
PACKAGES=(
  "astro-types"
  "astro-nodes"
  "astro-engine"
  "astro-graph"
  "astro-workflows"
  "astro-agent"
  "astro-playground"
)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

echo -e "${YELLOW}Building all packages first...${NC}"
cd "$ROOT_DIR"
bun run build

echo ""
echo -e "${YELLOW}Publishing packages to GitHub Package Registry...${NC}"
echo ""

for pkg in "${PACKAGES[@]}"; do
  PKG_DIR="$ROOT_DIR/packages/$pkg"

  if [ ! -d "$PKG_DIR" ]; then
    echo -e "${RED}Package directory not found: $PKG_DIR${NC}"
    continue
  fi

  cd "$PKG_DIR"

  # Get package name and version
  PKG_NAME=$(node -p "require('./package.json').name")
  PKG_VERSION=$(node -p "require('./package.json').version")
  IS_PRIVATE=$(node -p "require('./package.json').private || false")

  if [ "$IS_PRIVATE" = "true" ]; then
    echo -e "${YELLOW}Skipping private package: $PKG_NAME${NC}"
    continue
  fi

  echo -e "${GREEN}Publishing $PKG_NAME@$PKG_VERSION...${NC}"

  # Use --access restricted for scoped packages on GitHub registry
  if bun publish --access restricted 2>&1; then
    echo -e "${GREEN}Successfully published $PKG_NAME@$PKG_VERSION${NC}"
  else
    # Check if it failed because version already exists
    if bun publish --access restricted 2>&1 | grep -q "already exists"; then
      echo -e "${YELLOW}Version $PKG_VERSION already exists for $PKG_NAME, skipping...${NC}"
    else
      echo -e "${RED}Failed to publish $PKG_NAME${NC}"
      exit 1
    fi
  fi

  echo ""
done

echo -e "${GREEN}All packages published successfully!${NC}"
