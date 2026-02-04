#!/usr/bin/env bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

# Default values
BUMP_TYPE="patch"
BASE_REF="HEAD~1"
DRY_RUN=false
SKIP_CONFIRM=false

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --bump)
      BUMP_TYPE="$2"
      shift 2
      ;;
    --base)
      BASE_REF="$2"
      shift 2
      ;;
    --since-tag)
      # Find the most recent publish tag
      BASE_REF=$(git describe --tags --abbrev=0 --match "publish-*" 2>/dev/null || echo "")
      if [ -z "$BASE_REF" ]; then
        echo -e "${YELLOW}No previous publish tag found, will check all packages${NC}"
        BASE_REF=""
      else
        echo -e "${BLUE}Using base ref from tag: $BASE_REF${NC}"
      fi
      shift
      ;;
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    --yes|-y)
      SKIP_CONFIRM=true
      shift
      ;;
    --help)
      echo "Usage: $0 [options]"
      echo ""
      echo "Options:"
      echo "  --bump <type>    Version bump type: patch, minor, major (default: patch)"
      echo "  --base <ref>     Git ref to compare against (default: HEAD~1)"
      echo "  --since-tag      Use most recent publish-* tag as base"
      echo "  --dry-run        Show what would be published without actually publishing"
      echo "  --yes, -y        Skip confirmation prompt (for CI)"
      echo "  --help           Show this help message"
      exit 0
      ;;
    *)
      echo -e "${RED}Unknown option: $1${NC}"
      exit 1
      ;;
  esac
done

cd "$ROOT_DIR"

# Publishable packages in dependency order (for correct publish sequencing)
PUBLISH_ORDER=(
  "astro-types"
  "astro-nodes"
  "astro-engine"
  "astro-graph"
  "astro-workflows"
  "astro-agent"
  "astro-playground"
)

# Function to check if array contains element
contains() {
  local item="$1"
  shift
  for elem in "$@"; do
    if [ "$elem" = "$item" ]; then
      return 0
    fi
  done
  return 1
}

# Function to check if package is private
is_private() {
  local pkg_dir="$1"
  node -p "require('$pkg_dir/package.json').private || false"
}

# Function to get current version
get_version() {
  local pkg_dir="$1"
  node -p "require('$pkg_dir/package.json').version"
}

# Function to bump version
bump_version() {
  local version="$1"
  local bump_type="$2"

  IFS='.' read -r major minor patch <<< "$version"

  case $bump_type in
    major)
      echo "$((major + 1)).0.0"
      ;;
    minor)
      echo "$major.$((minor + 1)).0"
      ;;
    patch)
      echo "$major.$minor.$((patch + 1))"
      ;;
  esac
}

# Function to update package.json version
update_version() {
  local pkg_dir="$1"
  local new_version="$2"

  node -e "
    const fs = require('fs');
    const path = '$pkg_dir/package.json';
    const pkg = JSON.parse(fs.readFileSync(path, 'utf8'));
    pkg.version = '$new_version';
    fs.writeFileSync(path, JSON.stringify(pkg, null, 2) + '\n');
  "
}

# Function to resolve workspace:* references to actual versions
resolve_workspace_refs() {
  local pkg_dir="$1"
  local root_dir="$2"

  node -e "
    const fs = require('fs');
    const path = require('path');

    const pkgPath = '$pkg_dir/package.json';
    const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));

    const resolveDeps = (deps) => {
      if (!deps) return deps;
      const resolved = {};
      for (const [name, version] of Object.entries(deps)) {
        if (version.startsWith('workspace:')) {
          // Extract package name (e.g., @saswatds/astro-types -> astro-types)
          const shortName = name.replace('@saswatds/', '');
          const depPkgPath = path.join('$root_dir', 'packages', shortName, 'package.json');
          try {
            const depPkg = JSON.parse(fs.readFileSync(depPkgPath, 'utf8'));
            resolved[name] = depPkg.version;
          } catch (e) {
            resolved[name] = version; // Keep original if not found
          }
        } else {
          resolved[name] = version;
        }
      }
      return resolved;
    };

    pkg.dependencies = resolveDeps(pkg.dependencies);
    pkg.devDependencies = resolveDeps(pkg.devDependencies);

    fs.writeFileSync(pkgPath, JSON.stringify(pkg, null, 2) + '\n');
  "
}

# Function to restore workspace:* references after publishing
restore_workspace_refs() {
  local pkg_dir="$1"

  node -e "
    const fs = require('fs');

    const pkgPath = '$pkg_dir/package.json';
    const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));

    const restoreDeps = (deps) => {
      if (!deps) return deps;
      const restored = {};
      for (const [name, version] of Object.entries(deps)) {
        // If it's a @saswatds/ scoped package with a version number, restore to workspace:*
        if (name.startsWith('@saswatds/') && /^\d+\.\d+\.\d+/.test(version)) {
          restored[name] = 'workspace:*';
        } else {
          restored[name] = version;
        }
      }
      return restored;
    };

    pkg.dependencies = restoreDeps(pkg.dependencies);
    pkg.devDependencies = restoreDeps(pkg.devDependencies);

    fs.writeFileSync(pkgPath, JSON.stringify(pkg, null, 2) + '\n');
  "
}

# Function to get packages that depend on a given package
get_dependents() {
  local pkg_name="$1"

  for pkg in "${PUBLISH_ORDER[@]}"; do
    local pkg_dir="$ROOT_DIR/packages/$pkg"
    if [ -d "$pkg_dir" ]; then
      local has_dep=$(node -p "
        const pkg = require('$pkg_dir/package.json');
        const deps = {...(pkg.dependencies || {}), ...(pkg.devDependencies || {})};
        Object.keys(deps).includes('@saswatds/$pkg_name') ? 'yes' : 'no'
      ")
      if [ "$has_dep" = "yes" ]; then
        echo "$pkg"
      fi
    fi
  done
}

echo -e "${BLUE}Detecting affected packages...${NC}"

# Get directly affected packages using Moon
DIRECTLY_AFFECTED=()

if [ -n "$BASE_REF" ]; then
  # Use Moon to query affected projects (MOON_BASE env var sets the comparison ref)
  while IFS= read -r project; do
    if [ -n "$project" ]; then
      DIRECTLY_AFFECTED+=("$project")
    fi
  done < <(MOON_BASE="$BASE_REF" moon query projects --affected --json 2>/dev/null | node -e "
    const data = JSON.parse(require('fs').readFileSync('/dev/stdin', 'utf8'));
    data.projects.forEach(p => {
      if (p.id && p.id.startsWith('astro-')) console.log(p.id);
    });
  " 2>/dev/null || true)
fi

# If nothing detected, fall back to checking all publishable packages
if [ ${#DIRECTLY_AFFECTED[@]} -eq 0 ]; then
  echo -e "${YELLOW}No affected packages detected by Moon, checking all packages...${NC}"
  for pkg in "${PUBLISH_ORDER[@]}"; do
    DIRECTLY_AFFECTED+=("$pkg")
  done
fi

echo -e "${BLUE}Directly affected: ${DIRECTLY_AFFECTED[*]:-none}${NC}"

# Build full list including transitive dependents
TO_PUBLISH=("${DIRECTLY_AFFECTED[@]}")

# Add transitive dependents iteratively
changed=true
while $changed; do
  changed=false
  new_additions=()

  for pkg in "${TO_PUBLISH[@]}"; do
    while IFS= read -r dep; do
      if [ -n "$dep" ] && ! contains "$dep" "${TO_PUBLISH[@]}" && ! contains "$dep" "${new_additions[@]}"; then
        new_additions+=("$dep")
        changed=true
        echo -e "${BLUE}  Adding transitive dependent: $dep (depends on $pkg)${NC}"
      fi
    done < <(get_dependents "$pkg")
  done

  TO_PUBLISH+=("${new_additions[@]}")
done

# Filter to only publishable packages and maintain dependency order
PACKAGES_TO_PUBLISH=()
for pkg in "${PUBLISH_ORDER[@]}"; do
  if contains "$pkg" "${TO_PUBLISH[@]}"; then
    pkg_dir="$ROOT_DIR/packages/$pkg"
    if [ -d "$pkg_dir" ] && [ "$(is_private "$pkg_dir")" != "true" ]; then
      PACKAGES_TO_PUBLISH+=("$pkg")
    fi
  fi
done

if [ ${#PACKAGES_TO_PUBLISH[@]} -eq 0 ]; then
  echo -e "${GREEN}No packages need publishing.${NC}"
  exit 0
fi

echo ""
echo -e "${YELLOW}Packages to publish (in order):${NC}"
for pkg in "${PACKAGES_TO_PUBLISH[@]}"; do
  pkg_dir="$ROOT_DIR/packages/$pkg"
  current_version=$(get_version "$pkg_dir")
  new_version=$(bump_version "$current_version" "$BUMP_TYPE")
  echo -e "  ${GREEN}$pkg${NC}: $current_version → $new_version"
done

if [ "$DRY_RUN" = true ]; then
  echo ""
  echo -e "${YELLOW}Dry run - no changes made${NC}"
  exit 0
fi

if [ "$SKIP_CONFIRM" != true ]; then
  echo ""
  read -p "Proceed with version bump and publish? [y/N] " -n 1 -r
  echo ""

  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${YELLOW}Aborted.${NC}"
    exit 0
  fi
fi

# Step 1: Bump all versions first
echo ""
echo -e "${YELLOW}Bumping versions...${NC}"

for pkg in "${PACKAGES_TO_PUBLISH[@]}"; do
  pkg_dir="$ROOT_DIR/packages/$pkg"
  current_version=$(get_version "$pkg_dir")
  new_version=$(bump_version "$current_version" "$BUMP_TYPE")

  echo -e "  ${GREEN}$pkg${NC}: $current_version → $new_version"
  update_version "$pkg_dir" "$new_version"
done

# Step 2: Resolve workspace:* references to actual versions
# This is necessary because bun publish doesn't reliably resolve workspace refs
echo ""
echo -e "${YELLOW}Resolving workspace references...${NC}"

for pkg in "${PACKAGES_TO_PUBLISH[@]}"; do
  pkg_dir="$ROOT_DIR/packages/$pkg"
  resolve_workspace_refs "$pkg_dir" "$ROOT_DIR"
  echo -e "  ${GREEN}$pkg${NC}: resolved workspace:* → actual versions"
done

# Step 3: Run bun install to update lockfile
echo ""
echo -e "${YELLOW}Updating lockfile...${NC}"
cd "$ROOT_DIR"
bun install

# Step 4: Build all packages
echo ""
echo -e "${YELLOW}Building all packages...${NC}"
bun run build

# Step 5: Publish all packages in dependency order
echo ""
echo -e "${YELLOW}Publishing packages...${NC}"

for pkg in "${PACKAGES_TO_PUBLISH[@]}"; do
  pkg_dir="$ROOT_DIR/packages/$pkg"
  pkg_name=$(node -p "require('$pkg_dir/package.json').name")
  pkg_version=$(get_version "$pkg_dir")

  echo ""
  echo -e "${GREEN}[$pkg] Publishing $pkg_name@$pkg_version...${NC}"
  cd "$pkg_dir"

  if bun publish --access restricted; then
    echo -e "${GREEN}[$pkg] Published successfully${NC}"
  else
    echo -e "${RED}[$pkg] Failed to publish${NC}"
    exit 1
  fi
done

# Step 6: Restore workspace:* references
echo ""
echo -e "${YELLOW}Restoring workspace references...${NC}"

for pkg in "${PACKAGES_TO_PUBLISH[@]}"; do
  pkg_dir="$ROOT_DIR/packages/$pkg"
  restore_workspace_refs "$pkg_dir"
  echo -e "  ${GREEN}$pkg${NC}: restored workspace:*"
done

# Step 7: Update lockfile with restored references
echo ""
echo -e "${YELLOW}Updating lockfile...${NC}"
cd "$ROOT_DIR"
bun install

# Create a publish tag
PUBLISH_TAG="publish-$(date +%Y%m%d-%H%M%S)"
cd "$ROOT_DIR"
git add packages/*/package.json bun.lock
git commit -m "chore: publish packages

Packages published:
$(for pkg in "${PACKAGES_TO_PUBLISH[@]}"; do
  pkg_dir="$ROOT_DIR/packages/$pkg"
  echo "- @saswatds/$pkg@$(get_version "$pkg_dir")"
done)
"
git tag "$PUBLISH_TAG"

echo ""
echo -e "${GREEN}All packages published successfully!${NC}"
echo -e "${BLUE}Created tag: $PUBLISH_TAG${NC}"
echo -e "${YELLOW}Don't forget to push: git push && git push --tags${NC}"
