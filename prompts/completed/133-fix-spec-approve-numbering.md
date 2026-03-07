---
status: completed
summary: spec approve now assigns sequential NNN- numeric prefixes to spec files using NormalizeSpecFilename, consistent with prompt numbering
container: dark-factory-133-fix-spec-approve-numbering
dark-factory-version: v0.25.1
created: "2026-03-07T22:05:00Z"
queued: "2026-03-07T21:25:51Z"
started: "2026-03-07T21:25:54Z"
completed: "2026-03-07T21:32:08Z"
---
<summary>
- Approving a spec now assigns it a sequential number, consistent with how prompts are numbered
- The assigned number is unique across all spec lifecycle directories
- Re-approving an already-numbered spec preserves its existing number
- Unnumbered specs get the next available number in zero-padded 3-digit format
</summary>

<objective>
`spec approve` should assign a sequential number prefix to spec files, just like `prompt approve` does for prompts. Currently specs move to in-progress without numbering, making ordering and referencing inconsistent.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/cmd/spec_approve.go` — current approve logic: finds spec, sets status, moves to in-progress. No numbering. Line 71 builds `dest` using `filepath.Base(path)` unchanged.
Read `pkg/cmd/approve.go` — prompt approve: moves file then calls `promptManager.NormalizeFilenames` (line 86) which assigns numbers.
Read `pkg/prompt/prompt.go` — `NormalizeFilenames` function at line 820: scans dir for `.md` files, finds highest number across inbox/in-progress/completed, assigns `NNN-slug.md` format. Uses `parseFilename` (line 885) and `FileMover` interface. Reference only — do not modify this file.
Read `pkg/spec/spec.go` — `parseSpecNumber` function extracts numeric prefix from spec filenames. Reuse this for scanning existing numbers.
Read `pkg/factory/factory.go` — `CreateSpecApproveCommand` wiring.
</context>

<requirements>
1. Add a function to `pkg/spec/` (e.g. in a new file `pkg/spec/normalize.go` or in `spec.go`):
   - `NormalizeSpecFilename(ctx context.Context, name string, dirs ...string) (string, error)`
   - Scans all given dirs for `.md` files, extracts numeric prefixes using `parseSpecNumber`.
   - Finds the highest number across all dirs.
   - If `name` already has a valid numeric prefix, return it unchanged.
   - If `name` has no numeric prefix, return `fmt.Sprintf("%03d-%s", highest+1, name)`.

2. Update `pkg/cmd/spec_approve.go`:
   - Add `completedDir string` to the `specApproveCommand` struct.
   - Update constructor: `func NewSpecApproveCommand(inboxDir string, inProgressDir string, completedDir string) SpecApproveCommand`.
   - Before building `dest` (line 71), call `NormalizeSpecFilename` with `filepath.Base(path)` and all three dirs (inboxDir, inProgressDir, completedDir).
   - Use the returned filename for the dest path instead of `filepath.Base(path)`.

3. Update `pkg/factory/factory.go`:
   - Pass `cfg.Specs.CompletedDir` to `NewSpecApproveCommand`.

4. Add tests in `pkg/spec/normalize_test.go` (or `spec_test.go`):
   - Unnumbered file gets next number after highest existing.
   - Already-numbered file keeps its number.
   - Empty dirs → assigns `001-`.
   - Scans across multiple dirs correctly.

5. Update tests in `pkg/cmd/spec_approve_test.go`:
   - Add `completedDir` to constructor calls.
   - Test that approved spec file gets numbered filename.

6. Remove any imports that become unused.
</requirements>

<constraints>
- Reuse `parseSpecNumber` for extracting existing numbers — do NOT duplicate parsing logic
- Follow the prompt `NormalizeFilenames` pattern but keep it simpler (specs don't need batch rename, just single-file numbering on approve)
- Do NOT modify prompt numbering logic
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
