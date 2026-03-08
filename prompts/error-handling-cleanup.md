---
status: created
created: "2026-03-08T21:06:35Z"
---

<objective>
Fix ~12 unwrapped `return err` statements across `pkg/processor/`, `pkg/cmd/`, `pkg/server/`, and `pkg/runner/`. Replace `fmt.Errorf` with `errors.Wrap` in `pkg/review/fix_prompt_generator.go`. Replace `errors.Wrap(ctx, err, fmt.Sprintf(...))` with `errors.Wrapf` in `pkg/runner/runner.go`. All errors must be wrapped with context per project conventions.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/processor/processor.go` — unwrapped returns at lines ~194, 357, 375, 434, 465.
Read `pkg/cmd/prompt_verify.go` — unwrapped returns at lines ~69, 105, 109.
Read `pkg/cmd/spec_approve.go` — unwrapped return at line ~54.
Read `pkg/cmd/spec_complete.go` — unwrapped return at line ~52.
Read `pkg/cmd/spec_show.go` — unwrapped return at line ~78.
Read `pkg/server/completed_handler.go` — unwrapped return at line ~47.
Read `pkg/server/status_handler.go` — unwrapped return at line ~31.
Read `pkg/server/queue_handler.go` — unwrapped return at line ~31.
Read `pkg/server/inbox_handler.go` — unwrapped return at line ~42.
Read `pkg/review/fix_prompt_generator.go` — `fmt.Errorf` at line ~72, should be `errors.Wrap`.
Read `pkg/runner/runner.go` — `errors.Wrap(ctx, err, fmt.Sprintf(...))` at line ~201, should be `errors.Wrapf`.
Read `pkg/review/poller.go` — unwrapped `return nil, err` at line ~99 from `os.ReadDir`.
Read `pkg/config/workflow.go` — line ~34 uses `'%s'` quoting, change to `%q`.
Read `/home/node/.claude/docs/go-patterns.md`.
</context>

<requirements>
1. In every file listed above, find `return err` or `return nil, err` where the error lacks wrapping context. Replace with `errors.Wrap(ctx, err, "descriptive message")`.

2. In `pkg/review/fix_prompt_generator.go` line ~72:
   ```go
   // Before:
   fmt.Errorf("write fix prompt: %w", err)
   // After:
   errors.Wrap(ctx, err, "write fix prompt")
   ```

3. In `pkg/runner/runner.go` line ~201:
   ```go
   // Before:
   errors.Wrap(ctx, err, fmt.Sprintf("create directory %s", dir))
   // After:
   errors.Wrapf(ctx, err, "create directory %s", dir)
   ```

4. In `pkg/config/workflow.go` line ~34:
   ```go
   // Before:
   errors.Wrapf(ctx, validation.Error, "unknown workflow '%s'", w)
   // After:
   errors.Wrapf(ctx, validation.Error, "unknown workflow %q", w)
   ```

5. In `pkg/review/poller.go` line ~99:
   ```go
   // Before:
   return nil, err
   // After:
   return nil, errors.Wrap(ctx, err, "read queue dir")
   ```

6. Remove any unused `fmt` imports after replacing `fmt.Errorf`.
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
grep -n "return err$\|return nil, err$" pkg/processor/processor.go pkg/cmd/*.go pkg/server/*.go pkg/review/poller.go
# Expected: no output (or only in functions where error is already wrapped by callee)
```
</verification>

<success_criteria>
- All ~12 unwrapped `return err` replaced with `errors.Wrap`
- `fmt.Errorf` replaced with `errors.Wrap` in fix_prompt_generator.go
- `errors.Wrap(ctx, err, fmt.Sprintf(...))` replaced with `errors.Wrapf` in runner.go
- `%q` used instead of `'%s'` in workflow.go
- `make precommit` passes
</success_criteria>
