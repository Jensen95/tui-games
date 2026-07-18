#!/usr/bin/env bash
# Run every native Go fuzz target for FUZZTIME each (default 30s).
# Used by the nightly CI job; harmless when no fuzz targets exist yet.
set -euo pipefail
cd "$(dirname "$0")/.."

FUZZTIME="${FUZZTIME:-30s}"
found=0

for pkg in $(go list ./...); do
  targets=$(go test -list '^Fuzz' "$pkg" 2>/dev/null | grep '^Fuzz' || true)
  for t in $targets; do
    found=1
    echo "=== fuzz $pkg $t ($FUZZTIME)"
    go test -run "^$" -fuzz "^${t}\$" -fuzztime "$FUZZTIME" "$pkg"
  done
done

[ "$found" -eq 1 ] || echo "no fuzz targets found (yet)"
