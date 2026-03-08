---
status: completed
summary: Wrapped bare error returns in server handlers, review poller, fix prompt generator, runner with errors.Wrap/Wrapf, replaced fmt.Sprintf inside errors.Wrap with errors.Wrapf, and changed '%s' to %q in workflow validation
container: dark-factory-145-error-handling-cleanup
dark-factory-version: v0.30.3
created: "2026-03-08T21:06:35Z"
queued: "2026-03-08T23:18:05Z"
started: "2026-03-08T23:40:04Z"
completed: "2026-03-08T23:46:02Z"
---

<summary>
- Wrap 4 bare `return err` in server handlers with `errors.Wrap`
- Wrap 1 bare `return nil, err` in review poller
- Replace `fmt.Errorf` with `errors.Wrap` in fix_prompt_generator
- Replace `errors.Wrap(ctx, err, fmt.Sprintf(...))` with `errors.Wrapf` in runner
- Replace `'%s'` with `%q` in workflow error message
</summary>

<objective>
Fix ~7 error handling issues: unwrapped returns in server handlers and review poller, incorrect `fmt.Errorf` usage, `fmt.Sprintf` inside `errors.Wrap`, and non-idiomatic string quoting. All errors must follow project conventions.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-patterns.md` — error wrapping conventions.
Read `pkg/server/completed_handler.go` — unwrapped `return err` at line ~47 from `GetCompletedPrompts`.
Read `pkg/server/status_handler.go` — unwrapped `return err` at line ~31 from `GetStatus`.
Read `pkg/server/queue_handler.go` — unwrapped `return err` at line ~31 from `GetQueuedPrompts`.
Read `pkg/server/inbox_handler.go` — unwrapped `return err` at line ~42 from `os.ReadDir`.
Read `pkg/review/poller.go` — unwrapped `return nil, err` at line ~99 from `os.ReadDir`.
Read `pkg/review/fix_prompt_generator.go` — `fmt.Errorf` at line ~72, should be `errors.Wrap`.
Read `pkg/runner/runner.go` — `errors.Wrap(ctx, err, fmt.Sprintf(...))` at line ~201, should be `errors.Wrapf`.
Read `pkg/config/workflow.go` — line ~37 uses `'%s'` quoting, change to `%q`.
</context>

<requirements>
1. In `pkg/server/completed_handler.go` line ~47:
   ```go
   // Before:
   return err
   // After:
   return errors.Wrap(ctx, err, "get completed prompts")
   ```

2. In `pkg/server/status_handler.go` line ~31:
   ```go
   // Before:
   return err
   // After:
   return errors.Wrap(ctx, err, "get status")
   ```

3. In `pkg/server/queue_handler.go` line ~31:
   ```go
   // Before:
   return err
   // After:
   return errors.Wrap(ctx, err, "get queued prompts")
   ```

4. In `pkg/server/inbox_handler.go` line ~42:
   ```go
   // Before:
   return err
   // After:
   return errors.Wrap(ctx, err, "read inbox dir")
   ```

5. In `pkg/review/poller.go` line ~99:
   ```go
   // Before:
   return nil, err
   // After:
   return nil, errors.Wrap(ctx, err, "read queue dir")
   ```

6. In `pkg/review/fix_prompt_generator.go` line ~72:
   ```go
   // Before:
   fmt.Errorf("write fix prompt: %w", err)
   // After:
   errors.Wrap(ctx, err, "write fix prompt")
   ```

7. In `pkg/runner/runner.go` line ~201:
   ```go
   // Before:
   errors.Wrap(ctx, err, fmt.Sprintf("create directory %s", dir))
   // After:
   errors.Wrapf(ctx, err, "create directory %s", dir)
   ```

8. In `pkg/config/workflow.go` line ~37:
   ```go
   // Before:
   errors.Wrapf(ctx, validation.Error, "unknown workflow '%s'", w)
   // After:
   errors.Wrapf(ctx, validation.Error, "unknown workflow %q", w)
   ```

9. Add `"github.com/bborbe/errors"` import where missing (server handlers). Remove unused `fmt` import if `fmt.Errorf` was the only usage.
</requirements>

<constraints>
- Only change error return statements — do not modify any logic
- Wrap messages should be lowercase, descriptive, no punctuation
- Do NOT add new imports beyond `"github.com/bborbe/errors"` where missing
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Check no unwrapped returns remain in changed files:
```bash
grep -n "return err$\|return nil, err$" pkg/server/completed_handler.go pkg/server/status_handler.go pkg/server/queue_handler.go pkg/server/inbox_handler.go pkg/review/poller.go
# Expected: no output
```
</verification>

<success_criteria>
- All 4 server handler `return err` wrapped with `errors.Wrap`
- `return nil, err` wrapped in poller.go
- `fmt.Errorf` replaced with `errors.Wrap` in fix_prompt_generator.go
- `errors.Wrap(ctx, err, fmt.Sprintf(...))` replaced with `errors.Wrapf` in runner.go
- `%q` used instead of `'%s'` in workflow.go
- `make precommit` passes
</success_criteria>
