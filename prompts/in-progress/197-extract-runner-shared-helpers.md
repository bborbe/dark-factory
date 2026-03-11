---
status: draft
created: "2026-03-11T20:22:56Z"
queued: "2026-03-11T20:22:56Z"
---

<summary>
- Duplicated runner methods extracted to package-level functions
- Both runner and oneShotRunner share the same implementation
- No behavioral change — pure refactor
- Reduces maintenance burden when adding directories
- Existing tests continue to pass
</summary>

<objective>
Extract the three methods duplicated verbatim between `runner` and `oneShotRunner` into shared package-level functions. Currently `createDirectories`, `migrateQueueDir`, and `normalizeFilenames` are copy-pasted between the two structs.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/runner/runner.go` — find the `normalizeFilenames` (line ~152), `migrateQueueDir` (line ~167), and `createDirectories` (line ~187) methods on `runner`.
Read `pkg/runner/oneshot.go` — find the identical methods on `oneShotRunner` (line ~229, ~242, ~259).
Both structs have the same fields: `inboxDir`, `inProgressDir`, `completedDir`, `logDir`, `specsInboxDir`, `specsInProgressDir`, `specsCompletedDir`, `specsLogDir`, `promptManager`.
</context>

<requirements>
1. Create `pkg/runner/lifecycle.go` (new file) with three package-level functions:
   - `func normalizeFilenames(ctx context.Context, mgr prompt.Manager, inProgressDir string) error`
   - `func migrateQueueDir(ctx context.Context, inProgressDir string) error`
   - `func createDirectories(ctx context.Context, dirs []string) error`
2. In `pkg/runner/runner.go`, replace the three method bodies with calls to the shared functions:
   - `func (r *runner) normalizeFilenames(ctx) error { return normalizeFilenames(ctx, r.promptManager, r.inProgressDir) }`
   - `func (r *runner) migrateQueueDir(ctx) error { return migrateQueueDir(ctx, r.inProgressDir) }`
   - `func (r *runner) createDirectories(ctx) error { return createDirectories(ctx, []string{r.inboxDir, r.inProgressDir, ...}) }`
3. In `pkg/runner/oneshot.go`, do the same replacement for `oneShotRunner`
4. Add GoDoc comments to the three shared functions
5. Add copyright header to the new file
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Pure refactor — no behavioral changes
- Keep the method wrappers on both structs (they satisfy the interface contract)
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
