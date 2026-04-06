---
status: draft
created: "2026-04-06T00:00:00Z"
---

<summary>
- The NewContainerLock constructor uses context.Background() when wrapping errors
- Using context.Background() in pkg/ business logic violates the project context propagation rule
- The constructor currently has no ctx parameter, so no caller context is available
- Adding ctx as the first parameter allows proper error context to propagate from the caller
- All call sites in the factory must be updated to pass their ctx
</summary>

<objective>
Add `ctx context.Context` as the first parameter to `NewContainerLock()` in `pkg/containerlock/containerlock.go`, replace the two `errors.Wrap(context.Background(), ...)` calls with `errors.Wrap(ctx, ...)`, and update all callers to pass `ctx`. This ensures the caller's context flows through error wrapping in this constructor.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `pkg/containerlock/containerlock.go` — `NewContainerLock()` constructor at ~line 34; find the two `context.Background()` usages at ~lines 37 and 41
- `pkg/factory/factory.go` — find the call to `NewContainerLock()` (search for `containerlock.NewContainerLock`) and the enclosing function to determine what `ctx` is available
</context>

<requirements>
1. In `pkg/containerlock/containerlock.go`:
   a. Change the signature of `NewContainerLock()` to `NewContainerLock(ctx context.Context) (ContainerLock, error)`.
   b. Replace `errors.Wrap(context.Background(), err, "get user home dir")` with `errors.Wrap(ctx, err, "get user home dir")`.
   c. Replace `errors.Wrap(context.Background(), err, "create lock dir")` with `errors.Wrap(ctx, err, "create lock dir")`.
   d. Remove the `"context"` standard library import if it was only used for `context.Background()` — but keep it since `ctx context.Context` requires it.

2. In `pkg/factory/factory.go`:
   - Find every call to `containerlock.NewContainerLock()` and add `ctx` as the first argument.

3. Update any other callers of `NewContainerLock()` found by grepping the repository.

4. If `NewContainerLock` is called in test files with no ctx, pass `context.Background()` there — test files are exempt from the no-`context.Background()`-in-pkg rule.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Never use `context.Background()` in pkg/ non-test code
- Use `errors.Wrap(ctx, err, "message")` from `github.com/bborbe/errors`
- All paths are repo-relative
</constraints>

<verification>
make precommit
</verification>
