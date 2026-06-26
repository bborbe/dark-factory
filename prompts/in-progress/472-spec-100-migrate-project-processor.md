---
status: approved
spec: [100-centralize-subprocess-runner]
created: "2026-06-26T07:30:00Z"
queued: "2026-06-26T07:57:18Z"
branch: dark-factory/centralize-subprocess-runner
---

<summary>

- Routes project-name resolution and the processor's dirty-file check through the single bounded subprocess runner instead of raw process spawning.
- Project-name resolution today spawns git with NO context at all, so a slow git remote can hang the process forever; after this change it honours cancellation and a timeout.
- The dirty-file check today can stall on a broken filesystem; after this change it is bounded by the runner's default timeout.
- Adds a test proving project-name resolution returns quickly with a cancellation error when its context is already cancelled.
- Adds a test proving the dirty-file check returns a deadline-exceeded error when the underlying git call exceeds its timeout.
- All existing callers of project-name resolution continue to compile and behave the same on the happy path.

</summary>

<objective>
Migrate `pkg/project/name.go` (two ctx-less `exec.Command` git calls) and `pkg/processor/dirty.go` (one `exec.CommandContext` git call) to spawn through `pkg/subproc.Runner`. Give project-name resolution a real `context.Context` so cancellation and timeout are honoured. Bound the dirty-file check by the runner's default timeout.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read these coding-plugin docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-composition.md` — inject the runner as a dependency; do not call package functions directly.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 / Gomega, counterfeiter mocks, external `_test` packages, coverage ≥80%.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `bborbe/errors.Wrap`, never `fmt.Errorf`, never `context.Background()` in `pkg/`.

Read the parent spec end-to-end:
- `/workspace/specs/in-progress/100-centralize-subprocess-runner.md` — Desired Behavior 1, 2, 3; Acceptance Criteria 4, 5; Failure Modes table.

Read these source files before editing:
- `/workspace/pkg/subproc/subproc.go` — the runner. The interface (frozen, do NOT change) is:
  ```go
  type Runner interface {
      RunWithWarnAndTimeout(ctx context.Context, op string, name string, args ...string) ([]byte, error)
      RunWithWarnAndTimeoutDir(ctx context.Context, op string, dir string, name string, args ...string) ([]byte, error)
  }
  func NewRunner() Runner
  func NewRunnerWithThresholds(warnAfter, timeout time.Duration) Runner
  const DefaultTimeout = 10 * time.Second
  ```
  Note: the runner uses `cmd.Output()` internally (captures stdout only). On a child-process failure it returns `errors.Wrapf(ctx, cmdErr, "%s failed", op)` where `cmdErr` is the `*exec.ExitError`. On timeout it returns the bare sentinel `context.DeadlineExceeded`. On parent-context cancellation it returns a non-nil wrapped error (the child is killed via the timeout/cancel ctx).
- `/workspace/pkg/project/name.go` — `Resolve(configOverride string) Name`, `tryGitRoot()`, `tryGitRemote()`. The two raw `exec.Command("git", ...)` calls are at lines 52 and 66 (NO ctx — this is the worst site).
- `/workspace/pkg/processor/dirty.go` — `gitDirtyFileChecker.CountDirtyFiles(ctx)`, raw `exec.CommandContext(ctx, "git", "status", "--short")` at line 32, `cmd.Dir = c.repoDir`.
- `/workspace/pkg/factory/factory.go` — ALL `project.Resolve(...)` call sites: lines 358, 510, 619, 650, 1165, 1298, 1650 (7 sites), and the `processor.NewDirtyFileChecker(".")` sites at lines 398 and 520. You MUST update every `project.Resolve` call when its signature changes (Go has no default params).
- `/workspace/pkg/config/config.go` line 263-265 — `ResolvedProjectOverride()` returns the override string passed to `project.Resolve`.
- The mocks: `/workspace/mocks/subproc-runner.go` (fake name `SubprocRunner`) and `/workspace/mocks/dirty-file-checker.go` already exist — use the `SubprocRunner` fake in tests.

Inspect the existing `project.Resolve` call sites to learn how the result is consumed (most call `.String()` immediately). This determines the migration approach in requirement 1.
</context>

<requirements>

## 1. Migrate `pkg/project/name.go` to a context-aware resolver backed by `subproc.Runner`

The spec AC 4 requires "a unit test in `pkg/project` cancels the context before calling the resolver and asserts the function returns within 500 ms with a non-nil error wrapping `context.Canceled`". The current `Resolve` returns only `Name` (no ctx, no error). Therefore introduce a context-and-error-returning resolver and route all callers to it.

1.1. Change `Resolve` to accept a context and a `subproc.Runner`, and return an error. New signature:
```go
func Resolve(ctx context.Context, runner subproc.Runner, configOverride string) (Name, error)
```
Rationale for the design (resolve here, do not leave ambiguity): the runner is injected (testable), ctx is threaded (cancellable), and the error lets callers surface a hard failure. The fallback chain is preserved: config override → git root → git remote → working directory → literal `"dark-factory"`. The function returns a non-nil error ONLY when the context is already cancelled/timed out before or during a git probe (so AC 4 can assert a `context.Canceled`-wrapping error); a git command merely failing (e.g. not in a repo) is NOT an error — it falls through to the next fallback exactly as today.

1.2. Rewrite `tryGitRoot` and `tryGitRemote` to take `(ctx context.Context, runner subproc.Runner)` and use the runner:
```go
func tryGitRoot(ctx context.Context, runner subproc.Runner) (string, error) {
    output, err := runner.RunWithWarnAndTimeout(ctx, "git rev-parse --show-toplevel", "git", "rev-parse", "--show-toplevel")
    if err != nil {
        return "", err
    }
    root := strings.TrimSpace(string(output))
    if root == "" {
        return "", nil
    }
    return filepath.Base(root), nil
}
```
And similarly `tryGitRemote(ctx, runner)` for `git remote get-url origin`, preserving the `.git`-suffix stripping and `filepath.Base` URL parsing logic verbatim.

1.3. In `Resolve`, distinguish "git failed for a benign reason (not a repo)" from "context cancelled". After each `tryGitRoot`/`tryGitRemote` call: if the returned error wraps `context.Canceled` or `context.DeadlineExceeded` (test with `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)`, using `github.com/bborbe/errors`), return `("", that-error wrapped via errors.Wrap)` immediately — do NOT fall through. For any other error (or empty result), fall through to the next fallback. Use the empty-string sentinel from `tryGitRoot`/`tryGitRemote` to decide whether to advance the chain. The config-override and working-directory fallbacks never spawn a subprocess, so they cannot be cancelled and never return an error in the cancelled-before-git-probe case — but because requirement 1.3 returns early on a cancelled git probe, AC 4 (cancel before calling, assert error) is satisfied: with a cancelled ctx the runner returns immediately with a cancellation error on the first git probe.

   IMPORTANT for AC 4: when `configOverride != ""`, `Resolve` returns early with the override and NO git probe runs — so the AC 4 test MUST call `Resolve` with an EMPTY `configOverride` and a pre-cancelled context, so the first git probe runs and returns the cancellation error. State this in the test (requirement 3.1).

1.4. Add `import "github.com/bborbe/dark-factory/pkg/subproc"` and `"github.com/bborbe/errors"` to `name.go`. Remove the now-unused `"os/exec"` import. Keep `"os"`, `"path/filepath"`, `"strings"` (still used by the working-directory fallback and URL parsing).

## 2. Update every `project.Resolve` caller in `pkg/factory/factory.go`

Go has no default parameters — all 7 call sites (lines 358, 510, 619, 650, 1165, 1298, 1650) MUST be updated or the package will not compile.

2.1. At each call site, construct (or reuse) a `subproc.NewRunner()` and pass `ctx` and the runner. Each factory method that calls `project.Resolve` already has a `ctx context.Context` in scope (confirm by reading the enclosing function — they are factory `Create*`/wiring methods). Pass that ctx. Where a single function calls `Resolve` once, inline `subproc.NewRunner()` at the call:
```go
name, err := project.Resolve(ctx, subproc.NewRunner(), cfg.ResolvedProjectOverride())
if err != nil {
    return /* zero value */, errors.Wrap(ctx, err, "resolve project name")
}
```
Adjust the surrounding code to thread the new `error` return. If a call site currently does `project.Resolve(...).String()` inline (e.g. line 619, 1298), split it into the two-value form first, handle the error, then call `.String()` on the resulting `Name`.

2.2. If any of the 7 enclosing functions does NOT already return an `error`, prefer threading the error up; if that is impossible without a large signature cascade, at that specific site log the error via the context-bound logger and fall back to the resolved `Name` (which will be the working-directory or `"dark-factory"` fallback, never empty) — but FIRST attempt the clean error-return. Document any site where you had to log-and-continue in `## Improvements` (category PROMPT). Do NOT leave a `_ = err` discard.

2.3. Add `"github.com/bborbe/dark-factory/pkg/subproc"` to factory.go imports if not already present (grep first; the executor wiring may already import it).

## 3. Add the `pkg/project` cancellation test (spec AC 4)

3.1. In `pkg/project` (external `project_test` package, Ginkgo), add a spec whose `It` description contains `CancelledCtx` (so `go test -v ./pkg/project/... -run CancelledCtx` selects it). The test:
   - Creates a `context.Context` and cancels it immediately (`ctx, cancel := context.WithCancel(context.Background()); cancel()`).
   - Calls `project.Resolve(ctx, subproc.NewRunner(), "")` — note EMPTY override so a git probe actually runs.
   - Asserts the call returns within 500 ms (wrap with a timing assertion: capture `start := time.Now()` before, assert `time.Since(start) < 500*time.Millisecond` after).
   - Asserts the returned error is non-nil and `errors.Is(err, context.Canceled)` is true (use `github.com/bborbe/errors`).

   Use a real `subproc.NewRunner()` (not a fake) — the runner with a pre-cancelled ctx returns a cancellation error from the underlying `exec.CommandContext`, which is exactly the path under test. If on your environment a pre-cancelled ctx yields `context.DeadlineExceeded` instead of `context.Canceled` (it should be `Canceled` for `cancel()`), assert `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` to be robust — but prefer the strict `context.Canceled` assertion if it passes.

3.2. Keep/adjust existing `pkg/project` tests for the new `Resolve` signature. Any test that called the old `Resolve(string)` must be updated to `Resolve(ctx, subproc.NewRunner(), override)` and handle the error return.

## 4. Migrate `pkg/processor/dirty.go` to `subproc.Runner` with the default timeout

4.1. Inject a `subproc.Runner` into `gitDirtyFileChecker`. Change the constructor:
```go
func NewDirtyFileChecker(repoDir string) DirtyFileChecker {
    return &gitDirtyFileChecker{repoDir: repoDir, runner: subproc.NewRunner()}
}
```
Add a field `runner subproc.Runner` to the struct. Also add an unexported test constructor so the timeout test (requirement 5) can inject a fake/short-threshold runner:
```go
func newDirtyFileCheckerWithRunner(repoDir string, runner subproc.Runner) *gitDirtyFileChecker {
    return &gitDirtyFileChecker{repoDir: repoDir, runner: runner}
}
```
Export this to the test via `export_test.go` if `pkg/processor` tests are in the external `processor_test` package (check the existing test package name first; if internal `processor`, the unexported constructor is directly usable).

4.2. Rewrite `CountDirtyFiles` to use the runner with the working directory:
```go
func (c *gitDirtyFileChecker) CountDirtyFiles(ctx context.Context) (int, error) {
    output, err := c.runner.RunWithWarnAndTimeoutDir(ctx, "git status --short", c.repoDir, "git", "status", "--short")
    if err != nil {
        return 0, errors.Wrap(ctx, err, "git status --short")
    }
    trimmed := strings.TrimSpace(string(output))
    if trimmed == "" {
        return 0, nil
    }
    return len(strings.Split(trimmed, "\n")), nil
}
```
Remove the `"os/exec"` import; keep `"context"`, `"strings"`, `"github.com/bborbe/errors"`. Add `"github.com/bborbe/dark-factory/pkg/subproc"`.

4.3. The default runner gives the spec's bounded behavior (DefaultTimeout = 10s). A stuck filesystem no longer stalls the processor loop (spec Desired Behavior 3).

## 5. Add the dirty-file-check timeout test (spec AC 5)

5.1. In `pkg/processor` tests, add a Ginkgo spec whose `It` description contains `DirtyTimeout` (so `go test -v ./pkg/processor/... -run DirtyTimeout` selects it). The test:
   - Constructs a `gitDirtyFileChecker` with a SHORT-timeout runner via `subproc.NewRunnerWithThresholds(10*time.Millisecond, 50*time.Millisecond)` and a `repoDir` of `"/"` (or any dir), but make the underlying command hang. Simplest reliable approach: inject the short-threshold runner AND point it at a hanging command is not possible (the op is fixed to `git status`). Instead use a `mocks.SubprocRunner` fake whose `RunWithWarnAndTimeoutDirStub` sleeps past `2 × DefaultTimeout`'s lower bound then returns `context.DeadlineExceeded`. Preferred: use the real short-threshold runner with a fake by injecting a `SubprocRunner` fake whose stub does:
     ```go
     fake.RunWithWarnAndTimeoutDirStub = func(ctx context.Context, op, dir, name string, args ...string) ([]byte, error) {
         select {
         case <-ctx.Done():
             return nil, context.DeadlineExceeded
         case <-time.After(100 * time.Millisecond):
             return nil, context.DeadlineExceeded
         }
     }
     ```
     Build the checker with `newDirtyFileCheckerWithRunner(".", fake)` (exported via export_test.go as needed).
   - Calls `CountDirtyFiles(context.Background())`.
   - Asserts it returns within `2 × subproc.DefaultTimeout` (upper bound 20s — generous; the fake returns in ~100ms) and that the returned error matches `context.DeadlineExceeded` (`errors.Is(err, context.DeadlineExceeded)` is true).

5.2. The fake `SubprocRunner` is in `mocks/subproc-runner.go` (import `"github.com/bborbe/dark-factory/mocks"`). Confirm the stub field name by reading the generated mock (`RunWithWarnAndTimeoutDirStub`).

## 6. CHANGELOG

Append to `## Unreleased` in `/workspace/CHANGELOG.md`:
```
- fix: route project-name resolution and the processor dirty-file check through pkg/subproc.Runner so a slow git remote or stuck filesystem no longer hangs — both now honour context cancellation and a bounded timeout (spec 100 prompt 2)
```

</requirements>

<constraints>

- Do NOT change `pkg/subproc.Runner`'s exported interface — use existing `RunWithWarnAndTimeout` / `RunWithWarnAndTimeoutDir` only (spec Constraint — interface frozen).
- Never use `context.Background()` inside `pkg/` business logic — thread the caller's ctx. `context.Background()` is allowed ONLY at the top of a test `It` (spec / go-error-wrapping-guide).
- Wrap errors with `github.com/bborbe/errors` (`errors.Wrap(ctx, err, "...")`), never `fmt.Errorf` (spec / go-error-wrapping-guide).
- All 7 `project.Resolve` call sites in factory.go MUST compile after the signature change — Go has no default parameters (spec / sibling-entry-point rule). Do NOT leave any call site on the old signature.
- The fallback chain in `Resolve` is preserved exactly: override → git root → git remote → working dir → `"dark-factory"`. Only the cancellation/timeout path is new.
- Coverage ≥80% for `pkg/project` and `pkg/processor` changed code; test the error/cancellation paths explicitly (spec AC 4, 5).
- Do NOT migrate `pkg/git`, `pkg/gitprovider`, or `pkg/executor` in this prompt — those are prompts 3 and 4.
- Do NOT flip the execcheck gate to strict — that is prompt 5. The gate is in warn mode (prompt 1); migrating these two packages reduces the warn-mode offender list but does not yet need to zero it.
- No new Go dependencies (spec Constraint).
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.

</constraints>

<verification>

```bash
cd /workspace

# Package compiles + unit tests pass
go test -mod=mod ./pkg/project/... ./pkg/processor/... ./pkg/factory/...

# AC 4 — project-name cancellation test
go test -v -mod=mod ./pkg/project/... -run CancelledCtx   # output must contain PASS

# AC 5 — dirty-file timeout test
go test -v -mod=mod ./pkg/processor/... -run DirtyTimeout  # output must contain PASS

# No raw exec left in the two migrated files
grep -nE 'exec\.Command(Context)?\(' pkg/project/name.go pkg/processor/dirty.go   # expected: 0 matches

# Coverage on changed packages
go test -coverprofile=/tmp/cover.out -mod=mod ./pkg/project/... ./pkg/processor/... && go tool cover -func=/tmp/cover.out | tail -1

# Full precommit (execcheck still warn mode — must stay green)
make precommit                                  # expected: exit 0
```

</verification>
