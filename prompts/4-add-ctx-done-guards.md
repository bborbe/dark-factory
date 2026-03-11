---
status: created
created: "2026-03-11T16:45:24Z"
---

<summary>
- Long-running loops that execute Docker containers now respect context cancellation between iterations
- Cancelling the process during spec generation or inbox approval stops promptly instead of continuing through all remaining files
- Startup scan of existing specs also respects cancellation
</summary>

<objective>
Add `ctx.Done()` cancellation checks at the top of three loops that perform expensive per-item work (Docker execution, file I/O) without checking for context cancellation between iterations.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/runner/oneshot.go` — find `generateFromApprovedSpecs` (~line 145) and `approveInboxPrompts` (~line 189).
Read `pkg/specwatcher/watcher.go` — find `scanExistingInProgress` (~line 165).
Each of these loops iterates over directory entries and performs work per item (Docker execution or file operations) without a `select` on `ctx.Done()`.
</context>

<requirements>
1. In `pkg/runner/oneshot.go`, in the `generateFromApprovedSpecs` method, add a cancellation check at the top of the `for _, entry := range entries` loop (the one that calls `r.specGenerator.Generate`):
   ```go
   select {
   case <-ctx.Done():
       return 0, errors.Wrap(ctx, ctx.Err(), "context cancelled during spec generation")
   default:
   }
   ```

2. In `pkg/runner/oneshot.go`, in the `approveInboxPrompts` method, add the same pattern at the top of its `for _, entry := range entries` loop:
   ```go
   select {
   case <-ctx.Done():
       return 0, errors.Wrap(ctx, ctx.Err(), "context cancelled during inbox approval")
   default:
   }
   ```

3. In `pkg/specwatcher/watcher.go`, in the `scanExistingInProgress` method, add a cancellation check at the top of the `for _, entry := range entries` loop (the one that calls `w.handleFileEvent`):
   ```go
   select {
   case <-ctx.Done():
       return
   default:
   }
   ```
   Note: `scanExistingInProgress` returns no error, so just `return`.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
- Use `github.com/bborbe/errors` for wrapping — import `"github.com/bborbe/errors"` (already imported in both files).
- Place the `select` block as the first statement inside the loop body, before any `entry.IsDir()` checks.
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
