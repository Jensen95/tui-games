#!/usr/bin/env bash
# depguard: enforce the pure-engine seam.
#
# internal/engine and internal/games/* must never depend on the TUI, the charm
# stack, or os. This keeps the engine portable (Android later) and honest
# (no I/O, no terminal). See docs/plan/docs/01-architecture.md.
set -euo pipefail
cd "$(dirname "$0")/.."

fail=0

pkgs=$(go list ./internal/engine/... ./internal/games/... 2>/dev/null || true)
if [ -z "$pkgs" ]; then
  echo "depguard: no engine packages found (nothing to check)"
  exit 0
fi

for pkg in $pkgs; do
  # Full transitive dependencies: no TUI, no charm stack.
  banned=$(go list -deps "$pkg" | grep -E '^(github\.com/Jensen95/tui-games/internal/tui|charm\.land/|github\.com/charmbracelet/)' || true)
  if [ -n "$banned" ]; then
    echo "depguard FAIL: $pkg transitively imports:"
    echo "$banned" | sed 's/^/    /'
    fail=1
  fi
  # Direct imports (non-test): no os — engine code does no I/O.
  # (Tests may use os for testdata/golden files; .Imports excludes test files.)
  if go list -f '{{join .Imports "\n"}}' "$pkg" | grep -qx 'os'; then
    echo "depguard FAIL: $pkg directly imports os"
    fail=1
  fi
done

if [ "$fail" -eq 0 ]; then
  echo "depguard OK: engine seam is clean ($(echo "$pkgs" | wc -l) packages)"
fi
exit "$fail"
