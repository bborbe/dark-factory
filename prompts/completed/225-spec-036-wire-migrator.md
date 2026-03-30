---
status: completed
spec: ["036"]
summary: Wired slugmigrator.Migrator into generator post-processing, runner startup, and one-shot runner startup via factory helpers
container: dark-factory-225-spec-036-wire-migrator
dark-factory-version: v0.69.0
created: "2026-03-30T17:00:00Z"
queued: "2026-03-30T17:29:26Z"
started: "2026-03-30T18:16:21Z"
completed: "2026-03-30T18:31:11Z"
branch: dark-factory/full-slug-spec-references
---
<summary>
- Generator post-processes newly created prompt files to replace bare spec numbers with full slugs immediately after YOLO execution
- Runner startup migrates all prompt lifecycle directories (inbox, in-progress, completed, log) from bare spec numbers to full slugs before processing begins
- OneShotRunner startup performs the same migration before processing its queue
- Factory wires the `SpecSlugMigrator` into all three callsites with the correct spec dirs
- No behavior change for any callers that read spec references — `HasSpec` already handles both formats
</summary>

<objective>
Wire the `pkg/slugmigrator.Migrator` (created in the previous prompt) into three callsites: the generator post-processing step (so freshly generated prompts immediately get full slugs), the daemon runner startup, and the one-shot runner startup. This completes spec 036 by ensuring all spec references — both new and existing — use the full slug format.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/slugmigrator/migrator.go — the Migrator interface and NewMigrator constructor (created in the previous prompt).
Read pkg/generator/generator.go — the `Generate` method and `inheritFromSpec` post-processing step; this is where we add the migration call for newly created files.
Read pkg/runner/runner.go — the `Run` method startup sequence and `runner` struct.
Read pkg/runner/oneshot.go — the `Run` method startup sequence and `oneShotRunner` struct.
Read pkg/factory/factory.go — `CreateRunner`, `CreateOneShotRunner`, `CreateSpecGenerator`, and `NewRunner`/`NewOneShotRunner` call sites.
Read /home/node/.claude/docs/ for Go conventions (patterns, testing, factory pattern).
</context>

<requirements>
1. **`pkg/generator/generator.go`** — add slug migration for newly created prompts:

   a. Add `slugMigrator slugmigrator.Migrator` field to `dockerSpecGenerator` struct.

   b. Update `NewSpecGenerator` constructor to accept a `slugMigrator slugmigrator.Migrator` parameter
      and store it on the struct.

   c. In `Generate`, after `newFiles := diffFiles(beforeFiles, afterFiles)` is computed and
      `len(newFiles) > 0` is confirmed, add — before `inheritFromSpec` — a migration step:
      ```go
      // Resolve bare spec number refs to full slugs in newly generated prompts.
      if err := g.slugMigrator.MigrateDirs(ctx, []string{g.inboxDir}); err != nil {
          slog.Warn("failed to migrate spec slugs in inbox", "error", err)
      }
      ```
      (log warning and continue — migration failure must not block generation)

   d. Apply the same migration call in `reattachAndFinalize` after `newFiles` are identified
      (before the `inheritFromSpec` call).

2. **`pkg/runner/runner.go`** — add slug migration at startup:

   a. Add `slugMigrator slugmigrator.Migrator` field to `runner` struct.

   b. Update `NewRunner` to accept `slugMigrator slugmigrator.Migrator` and store it.

   c. Add a `migrateSpecSlugs` method:
      ```go
      func (r *runner) migrateSpecSlugs(ctx context.Context) error {
          return r.slugMigrator.MigrateDirs(ctx, []string{
              r.inboxDir, r.inProgressDir, r.completedDir, r.logDir,
          })
      }
      ```

   d. In `Run`, after `normalizeFilenames` and before starting the parallel runners, call:
      ```go
      if err := r.migrateSpecSlugs(ctx); err != nil {
          return errors.Wrap(ctx, err, "migrate spec slugs")
      }
      ```

3. **`pkg/runner/oneshot.go`** — add slug migration at startup:

   a. Add `slugMigrator slugmigrator.Migrator` field to `oneShotRunner` struct.

   b. Update `NewOneShotRunner` to accept `slugMigrator slugmigrator.Migrator` and store it.

   c. In `Run`, after `normalizeFilenames` and before the `for` loop, call:
      ```go
      if err := r.slugMigrator.MigrateDirs(ctx, []string{
          r.inboxDir, r.inProgressDir, r.completedDir, r.logDir,
      }); err != nil {
          return errors.Wrap(ctx, err, "migrate spec slugs")
      }
      ```

4. **`pkg/factory/factory.go`** — wire migrator into all callsites:

   a. Add a private helper (zero business logic, just wiring):
      ```go
      func createSpecSlugMigrator(cfg config.Config, currentDateTimeGetter libtime.CurrentDateTimeGetter) slugmigrator.Migrator {
          return slugmigrator.NewMigrator(
              []string{cfg.Specs.InboxDir, cfg.Specs.InProgressDir, cfg.Specs.CompletedDir},
              currentDateTimeGetter,
          )
      }
      ```

   b. In `CreateRunner`:
      - Call `createSpecSlugMigrator(cfg, currentDateTimeGetter)` to get `migrator`.
      - Pass `migrator` to `CreateSpecGenerator` (see step c).
      - Pass `migrator` to `runner.NewRunner` (add as last parameter before closing paren).

   c. In `CreateSpecGenerator` (at line ~314), add `slugMigrator slugmigrator.Migrator` as a
      parameter and thread it through to `generator.NewSpecGenerator`.

   d. In `CreateOneShotRunner`:
      - Call `createSpecSlugMigrator(cfg, currentDateTimeGetter)` to get `migrator`.
      - Pass `migrator` to `CreateSpecGenerator`.
      - Pass `migrator` to `runner.NewOneShotRunner`.

5. **Update tests** — fix any compilation errors in existing tests caused by the updated
   constructor signatures. Use the counterfeiter mock `mocks.SpecSlugMigrator` where needed.
   Do NOT add new test files — only fix compile errors in existing test files.
</requirements>

<constraints>
- Import path for slugmigrator: `github.com/bborbe/dark-factory/pkg/slugmigrator`
- Import path for mocks: `github.com/bborbe/dark-factory/mocks`
- Migration failure in generator must be non-fatal: log warning and continue (do not return error)
- Migration failure in runner/oneshot must be fatal: return the error (startup fails)
- Do NOT change the Migrator interface — that was defined in the previous prompt
- Use `github.com/bborbe/errors` for error wrapping (not fmt.Errorf)
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
Verify compilation: `go build -mod=vendor ./...`
</verification>
