---
status: completed
summary: Threaded ctx from main() through factory and status functions, removing all context.Background() calls from non-test production code except the two allowed locations (main.go root ctx and containerlock.go constructor).
container: dark-factory-258-thread-ctx-through-factory
dark-factory-version: v0.103.0
created: "2026-04-06T13:30:39Z"
queued: "2026-04-06T13:30:39Z"
started: "2026-04-06T13:30:55Z"
completed: "2026-04-06T13:58:01Z"
---

<summary>
- All factory functions receive ctx from main() instead of creating context.Background()
- Signal cancellation propagates through the entire call chain
- Global config loading uses the caller's ctx for proper cancellation
- isContainerRunning in status.go uses the passed ctx instead of creating its own
- Existing fmt.Errorf calls in factory are converted to errors.Wrap
- No new context.Background() calls outside of main.go and containerlock constructor
</summary>

<objective>
Thread the ctx created in `main()` through all factory and status functions that currently create their own `context.Background()`. This ensures signal cancellation (SIGINT/SIGTERM) propagates everywhere and no long-running operation ignores shutdown.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `main.go` — see how ctx is created with `signal.NotifyContext` and passed to `run(ctx)` → `runCommand(ctx, ...)`.
Read `pkg/factory/factory.go` — find all `context.Background()` calls. Search with `grep -n 'context.Background' pkg/factory/factory.go`.
Read `pkg/status/status.go` — find `isContainerRunning` which creates its own `context.Background()`.
</context>

<requirements>
1. **Add `ctx context.Context` to factory functions** in `pkg/factory/factory.go` that call `context.Background()`:

   - `createBitbucketProviderDeps` — this is the function with `ctx := context.Background()` (NOT `createProviderDeps`). Add `ctx context.Context` as first parameter, remove the internal `ctx := context.Background()` line.
   - `createProviderDeps` — add `ctx context.Context` as first parameter so it can pass ctx to `createBitbucketProviderDeps` and `createGitHubProviderDeps`.
   - `CreateRunner` — add `ctx context.Context` as first parameter. Replace `globalconfig.NewLoader().Load(context.Background())` with `Load(ctx)`. Also replace `fmt.Errorf("globalconfig: %w", err)` with `errors.Wrap(ctx, err, "globalconfig")`. Pass ctx to internal calls: `createProviderDeps(ctx, cfg, ...)`, and `CreateServer(ctx, ...)`.
   - `CreateOneShotRunner` — same pattern as `CreateRunner`.
   - `CreateServer` — add `ctx context.Context` as first parameter. Replace `globalconfig.NewLoader().Load(context.Background())` with `Load(ctx)`. This function is called from within `CreateRunner` (not from main.go), so update that call site too.
   - `CreateStatusCommand` — add `ctx context.Context` as first parameter. Replace `globalconfig.NewLoader().Load(context.Background())` with `Load(ctx)`.
   - `CreateCombinedStatusCommand` — same pattern as `CreateStatusCommand`.

   For each: also convert any `fmt.Errorf(...)` error returns to `errors.Wrap(ctx, err, ...)` or `errors.Errorf(ctx, ...)` since ctx is now available.

2. **Update all callers in `main.go`**:

   Search for all `factory.Create` calls in `main.go`. Add `ctx` as first argument where the factory function signature changed:
   - `factory.CreateRunner(cfg, version.Version)` → `factory.CreateRunner(ctx, cfg, version.Version)`
   - `factory.CreateOneShotRunner(cfg, version.Version, autoApprove)` → `factory.CreateOneShotRunner(ctx, cfg, version.Version, autoApprove)`
   - `factory.CreateCombinedStatusCommand(cfg)` → `factory.CreateCombinedStatusCommand(ctx, cfg)`
   - `factory.CreateStatusCommand(cfg)` → `factory.CreateStatusCommand(ctx, cfg)`

   For factory functions that do NOT have `context.Background()` internally (like `CreateKillCommand`, `CreateListCommand`, `CreateCombinedListCommand`), leave them unchanged.

3. **Fix `isContainerRunning` in `pkg/status/status.go`**:
   - Change signature to accept `ctx context.Context` as first parameter
   - Remove `ctx := context.Background()` inside the function
   - Update the caller in `populateExecutingPrompt` to pass ctx

4. **Update test call sites**: Existing tests in `pkg/factory/` or other packages that call the modified factory functions will need `context.Background()` added as the first argument to compile. Update these — tests are allowed to use `context.Background()`.

5. **Do NOT change** `context.Background()` in:
   - `main.go` — this is the root ctx, correct as-is
   - `pkg/containerlock/containerlock.go` — constructor has no ctx available, acceptable

6. **Verify no stray `context.Background()` calls** remain in non-test `.go` files except the two allowed locations above.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Do NOT change function signatures in interfaces (Executor, Checker, etc.) — only concrete factory functions and unexported helpers
- Use `errors.Wrap(ctx, err, ...)` for error wrapping — never `fmt.Errorf`
- Convert existing `fmt.Errorf` calls to `errors.Wrap`/`errors.Errorf` when ctx becomes available
</constraints>

<verification>
```bash
# Verify no unexpected context.Background() remains in production code
grep -rn 'context.Background()' --include='*.go' pkg/ main.go | grep -v '_test.go'
# Expected: only containerlock.go (2 occurrences)

# Verify no fmt.Errorf in modified functions
grep -n 'fmt.Errorf' pkg/factory/factory.go
# Expected: no results

# Verify main.go has the root ctx
grep -n 'signal.NotifyContext' main.go

make precommit
```
All must pass.
</verification>
