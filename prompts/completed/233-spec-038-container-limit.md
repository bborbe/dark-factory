---
status: completed
spec: [038-global-container-limit]
summary: 'Added system-wide container limit: ContainerCounter interface, waitForContainerSlot method in processor, global config wired through factory to NewProcessor, mock generated, tests added'
container: dark-factory-233-spec-038-container-limit
dark-factory-version: v0.80.0-1-g2b37ac1
created: "2026-03-31T18:01:00Z"
queued: "2026-03-31T19:11:30Z"
started: "2026-03-31T19:53:33Z"
completed: "2026-03-31T20:21:14Z"
branch: dark-factory/global-container-limit
---

<summary>
- Before starting each container, the processor checks how many dark-factory containers are running system-wide
- When the count equals or exceeds the limit, the processor waits until a slot frees up
- The limit comes from the global config (default 3)
- Invalid global config prevents daemon startup with a clear error
- Two daemons may briefly exceed the limit by one (acceptable race)
- Docker's running container list is the single source of truth — no file-based locks
</summary>

<objective>
Add a system-wide container limit: before executing each prompt, the processor counts running dark-factory containers and waits until a slot is available. The limit comes from `~/.dark-factory/config.yaml` (created in the previous prompt) and defaults to 3.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `/home/node/.claude/docs/go-patterns.md` — interface→constructor→struct, counterfeiter annotations.
Read `/home/node/.claude/docs/go-testing.md` — Ginkgo/Gomega, counterfeiter mocks, ≥80% coverage.
Read `/home/node/.claude/docs/go-context-cancellation.md` — non-blocking select in loops.
Read `pkg/executor/checker.go` — existing `ContainerChecker` interface pattern to follow for `ContainerCounter`.
Read `pkg/executor/executor.go` lines 320–328 — existing Docker labels (`dark-factory.project`, `dark-factory.prompt`) set on containers.
Read `pkg/processor/processor.go` lines 44–127 — `NewProcessor` constructor, `processor` struct, field patterns.
Read `pkg/processor/processor.go` lines 528–610 — `processPrompt` method where `p.executor.Execute` is called (wait loop inserts here).
Read `pkg/factory/factory.go` lines 194–256 — `CreateRunner` and how `CreateProcessor` is called.
Read `pkg/factory/factory.go` lines 258–330 — `CreateOneShotRunner`.
Read `pkg/globalconfig/globalconfig.go` — `GlobalConfig` struct and `NewLoader()` created in the previous prompt.
</context>

<requirements>
1. Add `ContainerCounter` interface to `pkg/executor/checker.go`:

   ```go
   //counterfeiter:generate -o ../../mocks/container-counter.go --fake-name ContainerCounter . ContainerCounter

   // ContainerCounter counts running dark-factory containers system-wide.
   type ContainerCounter interface {
       CountRunning(ctx context.Context) (int, error)
   }

   // NewDockerContainerCounter creates a ContainerCounter that uses docker ps with label filtering.
   func NewDockerContainerCounter() ContainerCounter {
       return &dockerContainerCounter{}
   }

   // dockerContainerCounter implements ContainerCounter using docker ps.
   type dockerContainerCounter struct{}

   // CountRunning returns the number of currently running dark-factory containers system-wide.
   // It filters by the label dark-factory.project which is set on every container.
   func (c *dockerContainerCounter) CountRunning(ctx context.Context) (int, error) {
       // #nosec G204 -- filter value is a hardcoded label key, not user input
       cmd := exec.CommandContext(
           ctx,
           "docker", "ps",
           "--filter", "label=dark-factory.project",
           "--format", "{{.Names}}",
       )
       var out strings.Builder
       cmd.Stdout = &out
       if err := cmd.Run(); err != nil {
           return 0, errors.Wrapf(ctx, err, "docker ps for container count")
       }
       output := strings.TrimSpace(out.String())
       if output == "" {
           return 0, nil
       }
       lines := strings.Split(output, "\n")
       count := 0
       for _, line := range lines {
           if strings.TrimSpace(line) != "" {
               count++
           }
       }
       return count, nil
   }
   ```

   Add `"github.com/bborbe/errors"` to imports in `checker.go` (check if already imported — add if missing).

2. Run `make generate` after adding the interface to generate the mock:
   ```bash
   cd /workspace && make generate
   ```
   Verify `mocks/container-counter.go` exists.

3. Add `containerCounter` and `maxContainers` fields to the `processor` struct in `pkg/processor/processor.go`:

   Add to the `processor` struct:
   ```go
   containerCounter executor.ContainerCounter
   maxContainers    int
   ```

   Add to `NewProcessor` constructor — append two new parameters at the end:
   ```go
   containerCounter executor.ContainerCounter,
   maxContainers int,
   ```
   Wire them into the returned struct:
   ```go
   containerCounter: containerCounter,
   maxContainers:    maxContainers,
   ```

4. Add `waitForContainerSlot` method to `processor` in `pkg/processor/processor.go`:

   ```go
   // waitForContainerSlot blocks until the system-wide running container count
   // is below maxContainers, then returns. Checks every 10 seconds.
   // Returns immediately if maxContainers <= 0 (no limit).
   // Returns ctx.Err() if context is cancelled while waiting.
   func (p *processor) waitForContainerSlot(ctx context.Context) error {
       if p.maxContainers <= 0 {
           return nil
       }
       for {
           count, err := p.containerCounter.CountRunning(ctx)
           if err != nil {
               slog.Warn("failed to count running containers, proceeding anyway", "error", err)
               return nil
           }
           if count < p.maxContainers {
               return nil
           }
           slog.Info(
               "waiting for container slot",
               "running", count,
               "limit", p.maxContainers,
           )
           select {
           case <-ctx.Done():
               return ctx.Err()
           case <-time.After(10 * time.Second):
           }
       }
   }
   ```

5. In `processPrompt` in `pkg/processor/processor.go`, insert the wait call AFTER the `p.brancher.MergeOriginDefault` call and BEFORE loading the prompt file. Find this block (around line 536):

   ```go
   slog.Info("syncing with remote default branch")
   if err := p.brancher.Fetch(ctx); err != nil {
       return errors.Wrap(ctx, err, "git fetch origin")
   }
   if err := p.brancher.MergeOriginDefault(ctx); err != nil {
       return errors.Wrap(ctx, err, "git merge origin default branch")
   }

   // Load prompt file once
   pf, err := p.promptManager.Load(ctx, pr.Path)
   ```

   Insert after `MergeOriginDefault`:
   ```go
   // Wait until a system-wide container slot is available
   if err := p.waitForContainerSlot(ctx); err != nil {
       return errors.Wrap(ctx, err, "wait for container slot")
   }
   ```

6. Update `pkg/factory/factory.go` — update `CreateProcessor` factory function call in both `CreateRunner` and `CreateOneShotRunner` to pass the new parameters:

   a. At the top of `CreateRunner`, load the global config. Since `CreateRunner` returns `runner.Runner` (no error), handle load errors via an `errRunner` adapter that returns the error on `Run()`:

   ```go
   // errRunner is a Runner that immediately returns an error when Run is called.
   type errRunner struct{ err error }
   func (e *errRunner) Run(_ context.Context) error { return e.err }
   ```

   ```go
   globalCfg, err := globalconfig.NewLoader().Load(context.Background())
   if err != nil {
       return &errRunner{err: fmt.Errorf("globalconfig: %w", err)}
   }
   ```

   Pass `globalCfg.MaxContainers` and `executor.NewDockerContainerCounter()` as the last two params of `CreateProcessor(...)` in `CreateRunner`.

   b. Similarly in `CreateOneShotRunner`, load global config and pass error through via an `errOneShotRunner` adapter:
   ```go
   // errOneShotRunner is an OneShotRunner that immediately returns an error when Run is called.
   type errOneShotRunner struct{ err error }
   func (e *errOneShotRunner) Run(_ context.Context) error { return e.err }
   ```

   Pass `globalCfg.MaxContainers` and `executor.NewDockerContainerCounter()` as the last two params of `CreateProcessor(...)` in `CreateOneShotRunner`.

   c. Add import `"github.com/bborbe/dark-factory/pkg/globalconfig"` to `pkg/factory/factory.go` imports.

   d. Note: `runner.OneShotRunner` must have `Run(ctx context.Context) error` — confirm its interface signature in `pkg/runner/oneshot.go` before adding the adapter.

7. Update `pkg/factory/factory.go` — find the `CreateProcessor` factory function itself (the standalone factory function, not the calls above). Update its signature to accept the two new params:

   Read `pkg/factory/factory.go` to find `CreateProcessor` and add:
   ```go
   containerCounter executor.ContainerCounter,
   maxContainers int,
   ```
   as the last two parameters, then pass them to `processor.NewProcessor(...)`.

8. Update `pkg/processor/processor_test.go` and `pkg/processor/processor_internal_test.go` — any `NewProcessor(...)` calls must be updated to pass the new params. Use:
   - `executor.NewDockerContainerCounter()` or a counterfeiter `ContainerCounter` fake
   - `3` for maxContainers in existing tests (or `0` to disable the limit for unit tests where Docker is unavailable)

   For unit tests where Docker is not available, pass `maxContainers: 0` so `waitForContainerSlot` returns immediately.

9. Add tests for `ContainerCounter` and `waitForContainerSlot`:

   Create `pkg/executor/checker_test.go` (or add to existing `executor_test.go`) with tests for `CountRunning` using the counterfeiter mock to verify parsing logic. Since we cannot run real Docker in tests, test the internal parsing logic separately.

   Create `pkg/processor/processor_internal_test.go` additions (use `package processor`):
   - `waitForContainerSlot returns nil when maxContainers is 0` — sets maxContainers=0, verify immediate return
   - `waitForContainerSlot returns nil when count is below limit` — mock counter returns 2, limit is 3, verify immediate return
   - `waitForContainerSlot waits and returns nil when slot frees up` — mock counter returns 3 then 2, limit is 3, verify function returns after second poll
   - `waitForContainerSlot returns ctx error on cancellation` — cancelled context, verify returns ctx.Err()
   - `waitForContainerSlot proceeds when counter returns error` — mock counter returns error, verify function returns nil (log-and-proceed behavior)

   Use the counterfeiter fake `mocks.ContainerCounter` for these tests.

10. Update CHANGELOG.md — add to `## Unreleased` section:
    ```
    - feat: Enforce system-wide container limit via ~/.dark-factory/config.yaml maxContainers (default: 3)
    - feat: Add ContainerCounter interface counting running dark-factory containers via docker ps label filter
    ```
</requirements>

<constraints>
- `docker ps` command uses `--filter label=dark-factory.project` — this existing label is set by the executor on every container and is sufficient to count all dark-factory containers system-wide
- Two daemons may race for the last slot and briefly exceed the limit by one — this is intentional and acceptable (soft limit)
- No file locks, semaphores, or shared state beyond Docker's running container list
- When `CountRunning` fails (e.g., Docker not responding), the processor logs a warning and proceeds — it does NOT block or fail
- Wait loop uses 10-second poll interval; logs at `slog.Info` level with `running` and `limit` fields
- `maxContainers <= 0` → `waitForContainerSlot` is a no-op (defensive guard; validation prevents this at load time but guard protects against future changes)
- Global config load error → daemon startup fails with a clear error message
- Error wrapping must use `errors.Wrap(ctx, err, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf` (exception: the errRunner adapter can use `fmt.Errorf` since there is no ctx)
- All existing tests must continue to pass
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
Run `make precommit` — must pass.

Additional checks:
```bash
# Confirm ContainerCounter interface and impl exist
grep -n "ContainerCounter" pkg/executor/checker.go

# Confirm counterfeiter mock generated
ls mocks/container-counter.go

# Confirm waitForContainerSlot exists in processor
grep -n "waitForContainerSlot" pkg/processor/processor.go

# Confirm new fields in processor struct
grep -n "containerCounter\|maxContainers" pkg/processor/processor.go

# Run executor tests
go test -mod=vendor ./pkg/executor/...

# Run processor tests with coverage
go test -coverprofile=/tmp/cover.out -mod=vendor ./pkg/processor/...
go tool cover -func=/tmp/cover.out | grep total
# Expected: ≥80%

# Run factory tests
go test -mod=vendor ./pkg/factory/...

# Run all tests
make test
```
</verification>
