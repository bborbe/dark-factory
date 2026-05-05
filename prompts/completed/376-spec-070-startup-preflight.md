---
status: completed
spec: [070-preflight-on-daemon-start]
summary: Added startup preflight call to runner.Run() via extracted runStartupPreflight() helper, generated PreflightChecker mock by adding go:generate directive to preflight suite test, updated all 10 NewRunner call sites in runner_test.go, and added 3 new tests covering nil checker, failing check, and passing check cases.
container: dark-factory-376-spec-070-startup-preflight
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-05T18:00:00Z"
queued: "2026-05-05T19:40:53Z"
started: "2026-05-05T20:11:36Z"
completed: "2026-05-05T20:27:07Z"
branch: dark-factory/preflight-on-daemon-start
---

<summary>
- The `dark-factory daemon` command now runs the configured `preflightCommand` before entering the watcher loop
- If preflight fails at startup, the daemon exits non-zero with the same log and notification behavior as a mid-run preflight failure
- If the preflight result is cached within `preflightInterval`, the startup call reuses the cache — no command is executed
- When `preflightCommand` is empty or `--skip-preflight` is passed (nil checker), the startup call is a no-op
- The per-prompt preflight call in the processor is unchanged — it stays as the safety net for cache expiry mid-run
- The startup preflight shares the same checker instance as the per-prompt preflight — a successful startup check warms the cache, so the first prompt gets a cache hit instead of re-running the command
- Three new unit tests cover: nil checker (no-op), failing check (daemon exits ErrPreflightFailed), passing check (daemon enters watcher loop)
- CHANGELOG updated
</summary>

<objective>
Add a startup preflight call to `runner.Run()` that runs the configured baseline check before the watcher loop begins. This closes the operator-feedback gap: instead of discovering a broken baseline only when a prompt is queued, the daemon either confirms the baseline is green at start or exits immediately with a clear failure.
</objective>

<context>
Read `CLAUDE.md` for project conventions (errors, Ginkgo/Gomega, Counterfeiter).
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Files to read before editing:
- `pkg/runner/runner.go` — `runner` struct, `NewRunner` constructor, `Run()` method
- `pkg/preflight/preflight.go` — `Checker` interface and `Check()` return contract
- `pkg/preflightconditions/conditions.go` — how `ShouldSkip` handles Check() results; the log lines it emits
- `pkg/processor/processor.go` — `ErrPreflightFailed` sentinel (re-exported from preflightconditions)
- `pkg/factory/factory.go` — `CreateRunner` (lines ~280–472): `preflightChecker` variable and `runner.NewRunner` call
- `pkg/runner/runner_test.go` — `newTestRunner` helper (~line 67–96) and existing test patterns

Architecture note (from `docs/architecture-flow.md`): preflight failure is terminal — the daemon exits. Never silently swallow it.

Key invariants:
- `preflight.Checker.Check()` ALWAYS returns `(bool, nil)` — it swallows errors internally, logs them, and returns `(false, nil)`. The error return is part of the interface but never non-nil in the concrete implementation.
- The same `preflightChecker` instance is created once in `CreateRunner` and passed to BOTH `CreateProcessor` (via `preflightconditions.NewConditions`) and `runner.NewRunner`. The `checker` struct caches results by timestamp. A startup call that passes at time T means the per-prompt call within `preflightInterval` after T gets a cache hit.
- `processor.ErrPreflightFailed` is the re-exported sentinel from `preflightconditions`. Runner already imports `processor`, so use `processor.ErrPreflightFailed` — no new package import needed beyond `preflight`.
</context>

<requirements>

## 1. Generate `mocks/preflight-checker.go`

The counterfeiter directive already exists in `pkg/preflight/preflight.go` (line 18):
```go
//counterfeiter:generate -o ../../mocks/preflight-checker.go --fake-name PreflightChecker . Checker
```

Run:
```bash
cd /workspace && go generate ./pkg/preflight/...
```

Verify `mocks/preflight-checker.go` exists with `type PreflightChecker struct`.

## 2. Update `pkg/runner/runner.go`

### 2a. Add import

In the import block, add:
```go
"github.com/bborbe/dark-factory/pkg/preflight"
```

`pkg/processor` is already imported — `processor.ErrPreflightFailed` is used in step 2c.

### 2b. Add field to `runner` struct

Insert `preflightChecker preflight.Checker` after `hideGit bool` and before `logWriter io.Writer`:

```go
hideGit               bool
preflightChecker      preflight.Checker
logWriter             io.Writer
```

### 2c. Add parameter to `NewRunner`

Add `preflightChecker preflight.Checker` as the second-to-last parameter (after `hideGit bool`, before `logWriter io.Writer`):

```go
func NewRunner(
    inboxDir string,
    inProgressDir string,
    completedDir string,
    logDir string,
    specsInboxDir string,
    specsInProgressDir string,
    specsCompletedDir string,
    specsLogDir string,
    promptManager PromptManager,
    locker lock.Locker,
    watcher watcher.Watcher,
    processor processor.Processor,
    server server.Server,
    reviewPoller review.ReviewPoller,
    specWatcher specwatcher.SpecWatcher,
    projectName project.Name,
    containerChecker executor.ContainerChecker,
    n notifier.Notifier,
    slugMigrator slugmigrator.Migrator,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    mover prompt.FileMover,
    maxPromptDuration time.Duration,
    containerStopper executor.ContainerStopper,
    startupLogger func(),
    hideGit bool,
    preflightChecker preflight.Checker,
    logWriter io.Writer,
) Runner {
```

Wire the new field in the returned struct literal:
```go
hideGit:               hideGit,
preflightChecker:      preflightChecker,
logWriter:             logWriter,
```

### 2d. Add startup preflight call in `Run()`

Insert the following block AFTER `processor.ResumeCommitting` and BEFORE `runners := []run.Func{`:

```go
// Startup preflight: verify baseline is green before the watcher loop begins.
if r.preflightChecker != nil {
    ok, err := r.preflightChecker.Check(ctx)
    if err != nil {
        slog.Warn("preflight checker error", "error", err)
        return processor.ErrPreflightFailed
    }
    if !ok {
        slog.Info("preflight: baseline broken — dark-factory will exit")
        return processor.ErrPreflightFailed
    }
}
```

This mirrors exactly what `preflightconditions.ShouldSkip()` does — same log lines, same sentinel — ensuring "the same log/notify behavior as a mid-run preflight failure".

The placement AFTER all resume steps ensures stuck/crashed containers are cleaned up even when the baseline is broken. The placement BEFORE `run.CancelOnFirstError` ensures the preflight log line appears BEFORE the processor's "processor started" / "waiting for changes" log lines.

## 3. Update `pkg/factory/factory.go`

In `CreateRunner`, the `runner.NewRunner(...)` call ends with (approximate lines 458–471):
```go
    createStartupLogger(ctx, cfg, globalCfg, sources),
    cfg.HideGit,
    logWriter,
)
```

Change to:
```go
    createStartupLogger(ctx, cfg, globalCfg, sources),
    cfg.HideGit,
    preflightChecker,
    logWriter,
)
```

The `preflightChecker` variable is already declared above in `CreateRunner` (lines ~370–389). Passing the same instance to both `CreateProcessor` and `runner.NewRunner` is intentional — the shared cache means a successful startup check prevents the per-prompt check from re-running the command within `preflightInterval`.

## 4. Update `pkg/runner/runner_test.go`

### 4a. Update ALL `runner.NewRunner` call sites in `pkg/runner/runner_test.go`

**Important:** the test file has **10 separate `runner.NewRunner(...)` call sites**, not just `newTestRunner`. Adding a new parameter breaks compilation at every site. Verify with:

```bash
grep -n "runner.NewRunner(" pkg/runner/runner_test.go
# expected: 10 lines (newTestRunner at ~68, plus 9 inline at ~364, 426, 478, 546, 764, 847, 940, 1029, 1092)
```

At every one of those call sites, the last two trailing arguments are:
```go
    false, // hideGit
    nil,   // logWriter: no file in tests
```

Insert a `nil, // preflightChecker` argument between them so each call ends with:
```go
    false, // hideGit
    nil,   // preflightChecker: no preflight in tests
    nil,   // logWriter: no file in tests
```

Recommended: use `sed` to edit all sites in one pass, then re-grep to confirm 10 occurrences:

```bash
# Insert preflightChecker line before every "nil,   // logWriter: no file in tests" line
sed -i.bak '/nil,   \/\/ logWriter: no file in tests/i\
\t\t\tnil,   // preflightChecker: no preflight in tests' pkg/runner/runner_test.go
rm pkg/runner/runner_test.go.bak

# Verify
grep -c "preflightChecker: no preflight in tests" pkg/runner/runner_test.go
# expected: 10
```

Adjust indentation if `sed` insertion drops indentation off — the inserted line must match the surrounding indent level at each call site (mix of 2 and 3 leading tabs). If `sed` mangles indentation, fall back to manual edit at each line, but every site must be updated.

### 4b. Add new import lines

Add to the import block:
```go
stderrors "errors"

"github.com/bborbe/dark-factory/pkg/processor"
```

(`stderrors` is needed for `stderrors.Is`.)

### 4c. Add preflight startup tests

Add a new `Describe` block inside the top-level `Describe("Runner", ...)` block, after the last existing `It` test:

```go
Describe("startup preflight", func() {
    var preflightChecker *mocks.PreflightChecker

    BeforeEach(func() {
        preflightChecker = &mocks.PreflightChecker{}
        locker.AcquireReturns(nil)
        locker.ReleaseReturns(nil)
        manager.NormalizeFilenamesReturns(nil, nil)
    })

    makeRunnerWithPreflight := func(inboxDir, inProgressDir, completedDir string) runner.Runner {
        return runner.NewRunner(
            inboxDir,
            inProgressDir,
            completedDir,
            filepath.Join(promptsDir, "logs"),
            filepath.Join(specsDir, "inbox"),
            filepath.Join(specsDir, "in-progress"),
            filepath.Join(specsDir, "completed"),
            filepath.Join(specsDir, "logs"),
            manager,
            locker,
            watcher,
            processor,
            nil, // server
            nil, // reviewPoller
            nil, // specWatcher
            "",
            containerChecker,
            notifier.NewMultiNotifier(),
            &mocks.SpecSlugMigrator{},
            libtime.NewCurrentDateTime(),
            &mocks.FileMover{},
            0,
            nil, // containerStopper
            nil, // startupLogger
            false, // hideGit
            preflightChecker,
            nil, // logWriter
        )
    }

    It("exits with ErrPreflightFailed when check returns false", func() {
        preflightChecker.CheckReturns(false, nil)

        r := makeRunnerWithPreflight(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))
        err := r.Run(ctx)

        Expect(stderrors.Is(err, processor.ErrPreflightFailed)).To(BeTrue())
        Expect(preflightChecker.CheckCallCount()).To(Equal(1))
    })

    It("exits with ErrPreflightFailed when check returns an error", func() {
        preflightChecker.CheckReturns(false, stderrors.New("preflight timed out"))

        r := makeRunnerWithPreflight(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))
        err := r.Run(ctx)

        Expect(stderrors.Is(err, processor.ErrPreflightFailed)).To(BeTrue())
        Expect(preflightChecker.CheckCallCount()).To(Equal(1))
    })

    It("enters watcher loop when check passes", func() {
        preflightChecker.CheckReturns(true, nil)

        watcher.WatchStub = func(ctx context.Context) error {
            <-ctx.Done()
            return nil
        }
        processor.ProcessStub = func(ctx context.Context) error {
            <-ctx.Done()
            return nil
        }

        r := makeRunnerWithPreflight(promptsDir, promptsDir, filepath.Join(promptsDir, "completed"))

        runCtx, runCancel := context.WithTimeout(ctx, 500*time.Millisecond)
        defer runCancel()

        err := r.Run(runCtx)
        Expect(err).To(BeNil())
        Expect(preflightChecker.CheckCallCount()).To(Equal(1))
    })
})
```

## 5. Update `CHANGELOG.md`

Add or append to `## Unreleased`:
```
## Unreleased

- feat: Run preflight at daemon startup before the watcher loop; daemon exits non-zero immediately when baseline is broken at start
```

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT change `OneShotRunner` — the `run` subcommand already runs preflight at start (the spec Non-goals section confirms this)
- Do NOT change `preflightInterval`, cache key, or cache duration semantics
- Do NOT remove or weaken the existing on-prompt-found preflight call in `processor.ProcessPrompt` — it stays as the safety net for cache expiry mid-run
- The startup preflight must use the SAME `preflightChecker` instance passed to `CreateProcessor` (shared cache) — do NOT create a second checker instance
- Wrap errors with `errors.Wrapf` from `github.com/bborbe/errors` in general — but for `ErrPreflightFailed`, return it UNWRAPPED so callers can use `stderrors.Is` (consistent with how it is returned in `preflightconditions.ShouldSkip`)
- The `--skip-preflight` flag already sets `preflightChecker = nil` in `CreateRunner` — the nil guard `if r.preflightChecker != nil` handles this case automatically; no separate flag plumbing needed
- `preflightCommand` empty → `preflightChecker = nil` in factory → nil guard handles it automatically
- Existing tests must still pass
- Do not touch `go.mod`, `go.sum`, or `vendor/`
</constraints>

<verification>
```bash
cd /workspace && go generate ./pkg/preflight/... && make test
```
All tests pass including the three new startup preflight tests.

```bash
cd /workspace && make precommit
```
Must exit 0.

Spot checks:
```bash
grep -n "preflightChecker" pkg/runner/runner.go
# expect: struct field, constructor param, struct init, conditional in Run()

grep -n "preflightChecker" pkg/factory/factory.go
# expect: one occurrence in the runner.NewRunner call

grep -c "preflightChecker" pkg/runner/runner_test.go
# expect: ≥ 14 (10 call-site nil args + test block uses)

ls mocks/preflight-checker.go
# must exist
```
</verification>
