---
status: completed
summary: Extracted duplicated bump-detection logic into `pkg/git/changelog.go` as `DetermineBumpFromChangelog`, updated both `pkg/processor` and `pkg/cmd` to use the shared function, and migrated all tests accordingly.
container: dark-factory-198-extract-determine-bump
dark-factory-version: v0.48.0
created: "2026-03-11T20:22:56Z"
queued: "2026-03-11T20:22:56Z"
started: "2026-03-12T01:59:18Z"
completed: "2026-03-12T02:10:21Z"
---

<summary>
- Changelog bump detection logic extracted to a shared location
- Both processor and prompt_verify use the same implementation
- Eliminates duplicated CHANGELOG.md parsing code
- No behavioral change — pure refactor
- Existing tests continue to pass
</summary>

<objective>
Extract the duplicated `determineBump` / `determineBumpFromChangelog` logic into `pkg/git` so both `pkg/processor` and `pkg/cmd` share one implementation.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/processor/processor.go` — find `determineBump` (line ~1105) and `extractUnreleasedSection` (line ~1121). These read CHANGELOG.md from the working directory and parse the `## Unreleased` section.
Read `pkg/cmd/prompt_verify.go` — find `determineBumpFromChangelog` (line ~171). This is a re-implementation of the same logic.
Read `pkg/git/git.go` — this package already owns changelog manipulation via `updateChangelog`.
</context>

<requirements>
1. Add a new exported function in `pkg/git/changelog.go` (new file or extend existing):
   - `func DetermineBumpFromChangelog(ctx context.Context, dir string) VersionBump`
   - Accepts the directory containing CHANGELOG.md instead of using the working directory
   - Contains the logic from `extractUnreleasedSection` + the bump determination
2. Update `pkg/processor/processor.go`:
   - Replace `determineBump()` with a call to `git.DetermineBumpFromChangelog(ctx, r.workDir)` (or the appropriate directory variable)
   - Remove the private `determineBump` and `extractUnreleasedSection` functions
3. Update `pkg/cmd/prompt_verify.go`:
   - Replace `determineBumpFromChangelog()` with a call to `git.DetermineBumpFromChangelog(ctx, ".")`
   - Remove the private `determineBumpFromChangelog` function
4. Add tests in `pkg/git/changelog_test.go` for the new function:
   - CHANGELOG with `## Unreleased` containing `- feat:` lines → `VersionBumpMinor`
   - CHANGELOG with `## Unreleased` containing only fix lines → `VersionBumpPatch`
   - CHANGELOG without `## Unreleased` → `VersionBumpPatch` (default)
   - Missing CHANGELOG.md → `VersionBumpPatch` (default)
5. Add GoDoc comment and copyright header
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- The function signature must accept a directory parameter, not use os.Getwd()
- Keep `VersionBump` type in `pkg/git` where it already lives
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
