---
status: completed
summary: Wrapped all bare return err statements with errors.Wrap(ctx, err, "message") in generator, slugmigrator, server/queue_action_handler, runner/lifecycle, and cmd/kill
container: dark-factory-270-review-dark-factory-fix-bare-return-err-other
dark-factory-version: v0.104.2-dirty
created: "2026-04-06T00:00:00Z"
queued: "2026-04-06T17:04:41Z"
started: "2026-04-06T17:46:59Z"
completed: "2026-04-06T17:56:52Z"
---

<summary>
- Several packages outside the processor return errors bare where ctx is available in scope
- Bare error returns lose stack trace and context values the project error library provides
- Affected packages include the generator, slug migrator, server, runner, and cmd/kill
- The fix is mechanical: wrap each bare return with errors.Wrap(ctx, err, "message")
- No logic changes are needed — only error wrapping is added
</summary>

<objective>
Wrap all bare `return err` statements in `pkg/generator/generator.go`, `pkg/slugmigrator/migrator.go`, `pkg/server/queue_action_handler.go`, `pkg/runner/lifecycle.go`, and `pkg/cmd/kill.go` where `ctx context.Context` is in scope, using `errors.Wrap(ctx, err, "message")` from `github.com/bborbe/errors`.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `docs/dod.md` for Definition of Done.

Files to read before making changes (read ALL first):
- `pkg/generator/generator.go` — `return err` at ~line 118 after `g.markSpecGenerating(...)` and ~line 137 after `g.executeAndFinalize(...)`
- `pkg/slugmigrator/migrator.go` — `return err` at ~line 179 after `pf.Save(ctx)`
- `pkg/server/queue_action_handler.go` — `return err` at ~line 90 after `queueAllFiles(...)`
- `pkg/runner/lifecycle.go` — `return err` at ~line 128 after `resumeOrResetExecutingEntry(...)` and ~line 155 after `resumeOrResetGeneratingEntry(...)`
- `pkg/cmd/kill.go` — `Run` method discards context with `_ context.Context`; the bare `return err` at ~line 66 after `os.ReadFile` cannot be wrapped unless ctx is accepted; assess whether adding ctx to `Run` is appropriate given the interface it implements
</context>

<requirements>
1. In `pkg/generator/generator.go`:
   - After `g.markSpecGenerating(ctx, ...)` failure (~line 118): replace `return err` with `return errors.Wrap(ctx, err, "mark spec generating")`.
   - After `g.executeAndFinalize(ctx, ...)` failure (~line 137): replace `return err` with `return errors.Wrap(ctx, err, "execute and finalize")`.

2. In `pkg/slugmigrator/migrator.go`:
   - After `pf.Save(ctx)` failure (~line 179): replace `return err` with `return errors.Wrap(ctx, err, "save prompt file")`.

3. In `pkg/server/queue_action_handler.go`:
   - After `queueAllFiles(ctx, ...)` failure (~line 90): replace `return err` with `return errors.Wrap(ctx, err, "queue all files")`.

4. In `pkg/runner/lifecycle.go`:
   - After `resumeOrResetExecutingEntry(ctx, ...)` failure (~line 128): replace `return err` with `return errors.Wrap(ctx, err, "resume or reset executing entry")`.
   - After `resumeOrResetGeneratingEntry(ctx, ...)` failure (~line 155): replace `return err` with `return errors.Wrap(ctx, err, "resume or reset generating entry")`.

5. In `pkg/cmd/kill.go`:
   - The `Run` method signature uses `_ context.Context`. If the interface allows it, change to `ctx context.Context` and wrap the `return err` at ~line 66 with `errors.Wrap(ctx, err, "read lock file")`.
   - If changing the interface signature is not feasible in this prompt (it would require updating too many callers), leave `kill.go` untouched and note it as a separate task.

6. Ensure `"github.com/bborbe/errors"` is imported in each modified file.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrap(ctx, err, "message")` from `github.com/bborbe/errors`
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
