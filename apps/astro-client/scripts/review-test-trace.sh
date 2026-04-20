#!/usr/bin/env bash
# Usage: ./scripts/show-trace.sh [test-results-dir]
set -euo pipefail

RESULTS_DIR="${1:-test-results}"

# Collect all trace.zip files (bash 3.2 compatible)
traces=()
while IFS= read -r line; do
  traces+=("$line")
done < <(find "$RESULTS_DIR" -name "trace.zip" 2>/dev/null | sort)

if [[ ${#traces[@]} -eq 0 ]]; then
  echo "No trace.zip files found in $RESULTS_DIR"
  echo "Run tests with --trace on to generate them:"
  echo "  VITE_API_URL='' bun x playwright test --trace on"
  exit 1
fi

while true; do
  echo ""
  echo "Available traces:"
  echo ""
  for i in "${!traces[@]}"; do
    dir=$(dirname "${traces[$i]}")
    name=$(basename "$dir")
    printf "  %2d)  %s\n" "$((i + 1))" "$name"
  done
  echo ""

  read -rp "Pick a trace [1-${#traces[@]}, q to quit]: " choice

  [[ "$choice" == "q" || "$choice" == "Q" ]] && break

  if ! [[ "$choice" =~ ^[0-9]+$ ]] || (( choice < 1 || choice > ${#traces[@]} )); then
    echo "Invalid choice."
    continue
  fi

  selected="${traces[$((choice - 1))]}"
  echo "Opening: $selected"
  bun x playwright show-trace "$selected" &
done
