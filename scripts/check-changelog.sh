#!/usr/bin/env bash
#
# Lints CHANGELOG.md for header-preservation violations that would strand
# the SemVer preamble (the failure mode that produced the v0.160.1 fix).
#
# Rules:
#   1. File must start with "# Changelog"
#   2. SemVer preamble line must appear BEFORE the first ## heading
#   3. SemVer preamble line must appear EXACTLY ONCE (no stranded copy)
#   4. MAJOR bullet must appear EXACTLY ONCE (no stranded copy)
#
# Run from repo root (Makefile target `check-changelog`).

set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

CHANGELOG="CHANGELOG.md"

ok=true

# Rule 1: first non-blank line must be "# Changelog"
FIRST_LINE=$(head -1 "$CHANGELOG")
if [ "$FIRST_LINE" != "# Changelog" ]; then
  echo "CHANGELOG.md lint failed: rule 1 — first line must be '# Changelog', got: '$FIRST_LINE'"
  ok=false
fi

# Rule 2: SemVer preamble must appear before first ## heading
SEMVER_LINENO=$(grep -n "Please choose versions by \[Semantic Versioning\]" "$CHANGELOG" | head -1 | cut -d: -f1 || true)
FIRST_HEADING_LINENO=$(grep -n "^## " "$CHANGELOG" | head -1 | cut -d: -f1 || true)

if [ -z "$SEMVER_LINENO" ]; then
  echo "CHANGELOG.md lint failed: rule 2 — SemVer preamble line not found in $CHANGELOG"
  ok=false
elif [ -z "$FIRST_HEADING_LINENO" ]; then
  echo "CHANGELOG.md lint failed: rule 2 — no ## heading found in $CHANGELOG"
  ok=false
elif [ "$SEMVER_LINENO" -ge "$FIRST_HEADING_LINENO" ]; then
  echo "CHANGELOG.md lint failed: rule 2 — SemVer preamble at line $SEMVER_LINENO appears after first ## heading at line $FIRST_HEADING_LINENO (stranded)"
  ok=false
fi

# Rule 3: SemVer preamble must appear exactly once
SEMVER_COUNT=$(grep -c "Please choose versions by \[Semantic Versioning\]" "$CHANGELOG" || true)
if [ "$SEMVER_COUNT" -ne 1 ]; then
  EXTRA_LINENO=$(grep -n "Please choose versions by \[Semantic Versioning\]" "$CHANGELOG" | tail -1 | cut -d: -f1 || true)
  echo "CHANGELOG.md lint failed: rule 3 — SemVer preamble appears $SEMVER_COUNT times, expected 1. Stranded copy at line $EXTRA_LINENO."
  ok=false
fi

# Rule 4: MAJOR bullet must appear exactly once
MAJOR_COUNT=$(grep -c "^\* MAJOR version when you make incompatible API changes," "$CHANGELOG" || true)
if [ "$MAJOR_COUNT" -ne 1 ]; then
  EXTRA_MAJOR_LINENO=$(grep -n "^\* MAJOR version when you make incompatible API changes," "$CHANGELOG" | tail -1 | cut -d: -f1 || true)
  echo "CHANGELOG.md lint failed: rule 4 — MAJOR bullet appears $MAJOR_COUNT times, expected 1. Stranded copy at line $EXTRA_MAJOR_LINENO."
  ok=false
fi

if $ok; then
  exit 0
fi
exit 1
