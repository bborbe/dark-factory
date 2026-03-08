---
status: completed
summary: Moved 5 standalone format helper functions (formatDuration, formatTime, formatHMS, formatMS, formatS) from status.go to formatter.go, making formatter.go the single source of formatting logic in the status package.
container: dark-factory-146-move-format-helpers-to-formatter
dark-factory-version: v0.30.3
created: "2026-03-08T21:12:08Z"
queued: "2026-03-08T23:18:05Z"
started: "2026-03-08T23:46:05Z"
completed: "2026-03-08T23:51:05Z"
---

<summary>
- Move 5 standalone format helper functions from status.go to formatter.go
- Pure display functions with no struct dependency belong in the formatter file
- Same package, so no import changes needed in callers or tests
</summary>

<objective>
Consolidate all standalone format helper functions into `formatter.go` so it is the single source of formatting logic in the status package.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/status/status.go` — find `formatDuration`, `formatHMS`, `formatMS`, `formatS`, `formatTime`, `formatBytes` functions. These are unexported helpers used by the checker.
Read `pkg/status/formatter.go` — existing formatter file where these should live.
Read `pkg/status/format_test.go` — tests for the format functions (should not need changes since same package).
</context>

<requirements>
1. Identify all unexported format helper functions in `pkg/status/status.go`:
   - `formatDuration` (~L333)
   - `formatTime` (~L354)
   - `formatHMS` (~L365)
   - `formatMS` (~L380)
   - `formatS` (~L389)

   Note: `formatBytes` is already in `formatter.go` — do NOT move it.

2. Move all 5 functions to `pkg/status/formatter.go`. Cut from `status.go`, paste into `formatter.go`.

3. Move any imports that are only used by these functions (e.g., `"fmt"` if only used for formatting).

4. Do NOT move any struct methods — only standalone package-level functions.

5. Verify `format_test.go` still compiles (same package, no import changes needed).
</requirements>

<constraints>
- Do NOT rename any functions — only move them
- Do NOT change function signatures or logic
- Same package (`package status`) so no import changes in callers
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Verify functions moved:
```bash
grep -n "func format" pkg/status/status.go
# Expected: no output

grep -n "func format" pkg/status/formatter.go
# Expected: all format functions listed
```
</verification>

<success_criteria>
- All `format*` functions in `pkg/status/formatter.go`
- None in `pkg/status/status.go`
- `format_test.go` passes unchanged
- `make precommit` passes
</success_criteria>
