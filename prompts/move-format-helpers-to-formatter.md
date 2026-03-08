---
status: created
created: "2026-03-08T21:12:08Z"
---

<objective>
Move `formatDuration` and its helpers (`formatHMS`, `formatMS`, `formatS`) from `pkg/status/status.go` to the existing `pkg/status/formatter.go`. These are pure display functions with no struct dependency — they belong in the formatter file.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/status/status.go` — find `formatDuration`, `formatHMS`, `formatMS`, `formatS`, `formatTime`, `formatBytes` functions. These are unexported helpers used by the checker.
Read `pkg/status/formatter.go` — existing formatter file where these should live.
Read `pkg/status/format_test.go` — tests for the format functions (should not need changes since same package).
</context>

<requirements>
1. Identify all unexported format helper functions in `pkg/status/status.go`:
   - `formatDuration`
   - `formatHMS`
   - `formatMS`
   - `formatS`
   - `formatTime`
   - `formatBytes`

2. Move all of them to `pkg/status/formatter.go`. Cut from `status.go`, paste into `formatter.go`.

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
