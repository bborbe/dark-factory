---
status: completed
spec: ["023"]
summary: Replaced keyword-based determineBump() with exact '- feat:' prefix detection, updated all internal tests to prefix-based cases, and fixed the integration test in processor_test.go to use the new prefix format.
container: dark-factory-117-spec-023-1-determinebump-prefix-logic
dark-factory-version: v0.19.0
created: "2026-03-06T20:00:00Z"
queued: "2026-03-06T18:45:26Z"
started: "2026-03-06T19:03:27Z"
completed: "2026-03-06T19:14:34Z"
---
<summary>
- Replaces fragile keyword matching in `determineBump()` with exact `- feat:` prefix detection
- Any `## Unreleased` entry starting with `- feat:` triggers a minor bump, everything else triggers a patch bump
- Entries without a prefix still work (backward compatible, treated as patch)
- `- feature:` does NOT match — only the exact prefix `- feat:` counts
- All old keyword-based tests replaced with prefix-based ones
- 15 test cases covering all prefixes, edge cases, and missing CHANGELOG scenarios
</summary>

<objective>
Replace the fragile keyword-matching logic in `determineBump()` with conventional prefix detection. After this change, `determineBump()` returns `MinorBump` if any `## Unreleased` entry starts with `- feat:`, and `PatchBump` for everything else — including entries with no prefix (backward compatible).
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/processor/processor.go` — contains `determineBump()` (line ~906) and `extractUnreleasedSection()`. These are the functions to change.
Read `pkg/processor/processor_internal_test.go` — contains all existing `determineBump` test cases (keyword-based); replace them entirely with prefix-based tests.
Read `/home/node/.claude/docs/go-patterns.md` and `go-testing.md` for patterns.
</context>

<requirements>
1. In `pkg/processor/processor.go`, rewrite `determineBump()`:

   Replace the current keyword-scan loop with a line-by-line prefix check:

   ```go
   // determineBump determines the version bump type by analyzing CHANGELOG.md content.
   // Returns MinorBump if any ## Unreleased entry starts with "- feat:", PatchBump otherwise.
   func determineBump() git.VersionBump {
       content, err := os.ReadFile("CHANGELOG.md")
       if err != nil {
           return git.PatchBump
       }

       unreleasedContent := extractUnreleasedSection(string(content))
       if unreleasedContent == "" {
           return git.PatchBump
       }

       for _, line := range strings.Split(unreleasedContent, "\n") {
           if strings.HasPrefix(strings.TrimSpace(line), "- feat:") {
               return git.MinorBump
           }
       }
       return git.PatchBump
   }
   ```

   The `strings` import is already present. No other imports need to change.

2. Leave `extractUnreleasedSection()` unchanged — it still correctly extracts the `## Unreleased` block.

3. In `pkg/processor/processor_internal_test.go`, replace ALL existing `determineBump` test cases with prefix-based ones. Remove every test that uses the old keyword approach (`add`, `implement`, `new`, `support`, `feature`, `Address`). Add the following test cases:

   - `- feat: Add SpecWatcher` → `MinorBump`
   - `- feat: Implement authentication` → `MinorBump`
   - Multiple entries including one `- feat:` line → `MinorBump`
   - `- fix: Remove stale container` → `PatchBump`
   - `- refactor: Extract worktree cleanup` → `PatchBump`
   - `- chore: Update github.com/bborbe/errors to v1.5.2` → `PatchBump`
   - `- test: Add processor test suite` → `PatchBump`
   - `- docs: Add changelog writing guide` → `PatchBump`
   - `- perf: Improve startup latency` → `PatchBump`
   - Entry with no prefix (e.g., `- Add container name tracking`) → `PatchBump` (backward compat)
   - Entry starting with `- feature:` (not `- feat:`) → `PatchBump` (not a match)
   - Entry with `feat:` in the middle of a line (not at start of `- `) → `PatchBump`
   - `CHANGELOG.md` does not exist → `PatchBump`
   - `CHANGELOG.md` has no `## Unreleased` section → `PatchBump`
   - `## Unreleased` section is empty (no bullet entries) → `PatchBump`

   Keep the `sanitizeContainerName` tests unchanged.

4. Verify test coverage for `pkg/processor` remains ≥80% after changes:
   ```bash
   go test -cover ./pkg/processor/...
   ```
</requirements>

<constraints>
- Only `determineBump()` changes — do NOT touch `extractUnreleasedSection()` or any other function
- Backward compatible: entries without a prefix must return `PatchBump`, never crash
- `- feature:` is NOT a match — only the exact prefix `- feat:` triggers minor bump
- The keyword list (`add`, `implement`, `new`, `support`, `feature`) must be fully removed
- Do NOT commit — dark-factory handles git
- Existing tests must still pass (`make test`)
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Additional checks:
```bash
# Confirm keyword list is gone
grep -n "implement\|support\|feature\|\"add\"\|\"new\"" pkg/processor/processor.go
# Should return no matches (or only unrelated occurrences)

# Confirm feat: prefix check is present
grep -n "feat:" pkg/processor/processor.go

# Run tests with coverage
go test -cover ./pkg/processor/...
```
</verification>
