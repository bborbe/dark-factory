---
status: draft
created: "2026-04-06T00:00:00Z"
---

<summary>
- A goroutine in the processor is launched with a raw go func() with no lifecycle management
- Raw goroutines violate the project's concurrency guidelines which require using the run package
- The goroutine waits until a container is running then releases a lock; if it hangs, there is no way to cancel it
- Replacing it with a tracked approach prevents goroutine leaks on shutdown
- The fix adds proper error handling and context propagation to this background task
</summary>

<objective>
Replace the raw `go func()` goroutine in `startContainerLockRelease` in `pkg/processor/processor.go` with a structured approach using `github.com/bborbe/run`, ensuring the goroutine lifecycle is tracked and the task can be cancelled via context.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-concurrency-patterns.md` for the run package pattern.

Files to read before making changes (read ALL first):
- `pkg/processor/processor.go` — `startContainerLockRelease` function at ~line 738; understand how it is called from `prepareContainerSlot` or similar and what the `release` func does
- `pkg/processor/processor.go` — find where the run group or errgroup is used in the processor to understand the existing concurrency pattern
</context>

<requirements>
1. In `pkg/processor/processor.go`, locate `startContainerLockRelease` (~line 738).

2. The current implementation:
   ```go
   func (p *processor) startContainerLockRelease(ctx context.Context, name string, release func()) {
       if p.containerChecker == nil {
           return
       }
       cc := p.containerChecker
       go func() {
           _ = cc.WaitUntilRunning(ctx, name, 30*time.Second)
           release()
       }()
   }
   ```

3. Replace with a pattern that:
   a. Still calls `cc.WaitUntilRunning(ctx, name, 30*time.Second)` followed by `release()`.
   b. Respects context cancellation: if `ctx` is cancelled before `WaitUntilRunning` returns, `release()` should still be called (to avoid leaking the lock), but the goroutine should terminate promptly.
   c. Uses `run.FireAndForget` from `github.com/bborbe/run` if available, or adds the goroutine to the processor's run group. If neither is viable without large refactoring, use a well-commented raw goroutine that calls `release()` in a `defer` to ensure the lock is always released:
   ```go
   go func() {
       defer release()
       _ = cc.WaitUntilRunning(ctx, name, 30*time.Second)
   }()
   ```
   This at minimum ensures the lock is released even if `WaitUntilRunning` returns early due to context cancellation.

4. Add a code comment explaining why a goroutine is used here and that `release()` is deferred to guarantee execution.

5. Ensure the `"github.com/bborbe/run"` import is added if used.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Prefer `run` package patterns over raw goroutines
- The lock release function must always be called, even if the context is cancelled
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
