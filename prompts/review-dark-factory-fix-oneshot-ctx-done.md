---
status: draft
created: "2026-04-06T00:00:00Z"
---

<summary>
- The one-shot runner's main loop can start a full prompt execution cycle after context cancellation
- Each iteration may call ProcessQueue which itself runs a Docker container and can take minutes
- Without a ctx.Done() check at the top of the loop, shutdown is delayed by a full execution cycle
- Adding a non-blocking select at the start of each iteration allows clean and prompt shutdown
- The fix is a small addition that does not change the loop's normal execution path
</summary>

<objective>
Add a non-blocking `ctx.Done()` guard at the top of the `for` loop body in `pkg/runner/oneshot.go` (~line 141) so that context cancellation is detected before starting a new iteration, preventing an unnecessary full `ProcessQueue` cycle after shutdown is requested.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read the coding plugin's `go-context-cancellation-in-loops.md` guide for the required pattern.

Files to read before making changes (read ALL first):
- `pkg/runner/oneshot.go` — the `for` loop at ~line 141; understand the full loop body before and after the change
</context>

<requirements>
1. In `pkg/runner/oneshot.go`, locate the `for {` loop at ~line 141.

2. Add a non-blocking context check as the very first statement inside the loop body:
   ```go
   for {
       select {
       case <-ctx.Done():
           return ctx.Err()
       default:
       }
       // ... rest of the loop body unchanged ...
   }
   ```

3. The rest of the loop body (calls to `generateFromApprovedSpecs`, `ListQueued`, `ProcessQueue`, and the `break` condition) must remain unchanged.

4. Ensure the return type of the enclosing function accepts `error` so `ctx.Err()` can be returned (it should already, since `ProcessQueue` returns an error).

5. Update or add a test in the runner test suite that verifies: when context is cancelled before the loop body runs, the function returns promptly with a context error rather than starting a `ProcessQueue` cycle.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use the non-blocking `select { case <-ctx.Done(): return ctx.Err(); default: }` pattern
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
