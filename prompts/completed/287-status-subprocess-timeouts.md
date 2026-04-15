---
status: completed
summary: Added pkg/subproc.Runner helper with 3s warn / 10s timeout semantics, wired it into all status subprocess calls (git status, docker ps), added Skipped fields to Status struct, updated formatter to render skipped state, and added full test coverage for warn/timeout/skip behavior.
container: dark-factory-287-status-subprocess-timeouts
dark-factory-version: v0.108.0-dirty
created: "2026-04-15T00:00:00Z"
queued: "2026-04-15T12:56:19Z"
started: "2026-04-15T13:19:43Z"
completed: "2026-04-15T13:52:18Z"
---

<summary>
- `dark-factory status` cannot hang indefinitely on slow git or busy docker anymore
- Each subprocess call used by status (git status, docker ps) has a hard 10s ceiling
- After 3 seconds, a warning is printed to stderr naming the slow operation
- After 10 seconds, the operation is cancelled, a skip message is printed to stderr, and status continues with the remaining calls
- Skipped calls leave their status fields at zero values — the human formatter labels them clearly (e.g. "dirty files: (skipped)") instead of reporting misleading zeros
- JSON output is unaffected (warnings and skip messages go to stderr, never stdout)
- A small, testable helper centralizes the warn+timeout pattern so future subprocess calls can reuse it
- New unit tests verify the warn and skip behavior using short thresholds to keep tests fast
</summary>

<objective>
Bound the latency of every subprocess call used by `dark-factory status` so the command returns within ~10 seconds even when git or docker is unresponsive. Users see a warning at 3s for any slow call and a clear "skipped" marker if a call is abandoned at 10s. JSON output stays clean.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `pkg/status/status.go` — contains `isContainerRunning`, `populateGeneratingSpec`, `populateGitWarnings`, and the `Status` struct
- `pkg/executor/checker.go` — contains `dockerContainerCounter.CountRunning` and the existing subprocess pattern using `exec.CommandContext(ctx, ...).Output()` / `.Run()`
- `pkg/status/formatter.go` — `Format`, `formatWarnings`, `formatDirtyFileWarning` (need `(skipped)` rendering for dirty file count and container count when marked skipped)
- `pkg/status/status_suite_test.go` — Ginkgo test suite setup (use Ginkgo + Gomega for new tests)
- `pkg/status/status_test.go` — existing Ginkgo test style to mirror
- `pkg/executor/checker.go` — `IsRunning` pattern; this call is NOT in scope, but use it as a style reference for error wrapping

Also read (coding plugin docs):
- `~/.claude/plugins/marketplaces/coding/docs/go-concurrency-patterns.md` — goroutine + done channel patterns
- `~/.claude/plugins/marketplaces/coding/docs/go-context-cancellation-in-loops.md` — context-with-timeout patterns
- `~/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega conventions

## Background

All four call sites below use `exec.CommandContext(ctx, ...)` where `ctx` is only cancelled by SIGINT. In a slow monorepo (e.g. `sm-octopus/billomat`) `git status --porcelain` or `docker ps` can block for minutes. The user sees nothing until Ctrl+C, then the output prints. The goal is to bound each call: warn at 3s, abandon at 10s.
</context>

<requirements>

## 1. Create a reusable subprocess helper

Create a new package `pkg/subproc/` with file `pkg/subproc/subproc.go`:

```go
// Package subproc provides bounded-duration subprocess execution for
// short-lived read-only commands (git status, docker ps). Each call emits a
// warning to stderr after warnAfter and is cancelled at timeout.
package subproc

import (
    "context"
    "log/slog"
    "os/exec"
    "time"

    "github.com/bborbe/errors"
)

// Default thresholds for RunWithWarnAndTimeout.
const (
    DefaultWarnAfter = 3 * time.Second
    DefaultTimeout   = 10 * time.Second
)

// Runner runs short subprocesses with warn + timeout semantics.
type Runner interface {
    // RunWithWarnAndTimeout runs `name args...` bounded by the configured
    // warnAfter/timeout. On timeout it returns a non-nil error and the
    // caller should treat the operation as skipped.
    //
    // op is a human-readable operation label used in warn/skip messages
    // (e.g. "git status --porcelain").
    RunWithWarnAndTimeout(ctx context.Context, op string, name string, args ...string) ([]byte, error)
}

// NewRunner returns a Runner using the default 3s/10s thresholds.
func NewRunner() Runner {
    return &runner{warnAfter: DefaultWarnAfter, timeout: DefaultTimeout}
}

// NewRunnerWithThresholds returns a Runner with custom thresholds (for tests).
func NewRunnerWithThresholds(warnAfter, timeout time.Duration) Runner {
    return &runner{warnAfter: warnAfter, timeout: timeout}
}

type runner struct {
    warnAfter time.Duration
    timeout   time.Duration
}
```

Implement `RunWithWarnAndTimeout` using `github.com/bborbe/run.CancelOnFirstFinishWait` (project convention — see `go-concurrency-patterns.md`, "go func() is a smell"). `github.com/bborbe/run v1.9.12` is already in `go.mod` — do NOT add a new dependency, do NOT use raw `go func()`.

Pattern:

```go
func (r *runner) runInternal(ctx context.Context, op string, dir string, name string, args ...string) ([]byte, error) {
    cmdCtx, cancel := context.WithTimeout(ctx, r.timeout)
    defer cancel()

    var output []byte
    var cmdErr error

    err := run.CancelOnFirstFinishWait(
        cmdCtx,
        // Subprocess runner — finishes first in the fast path, which cancels the warn goroutine via ctx.
        func(ctx context.Context) error {
            cmd := exec.CommandContext(ctx, name, args...)
            if dir != "" {
                cmd.Dir = dir
            }
            output, cmdErr = cmd.Output()
            return nil // surface cmd error via cmdErr, not here, so both funcs exit cleanly
        },
        // Warn-after-threshold watcher — exits immediately when cmdCtx is cancelled (by cmd finishing or timeout).
        func(ctx context.Context) error {
            select {
            case <-ctx.Done():
                return nil
            case <-time.After(r.warnAfter):
                slog.Warn("subprocess slow", "op", op, "threshold", r.warnAfter)
            }
            <-ctx.Done() // block until cmd finishes or ctx times out
            return nil
        },
    )
    if err != nil {
        return nil, errors.Wrap(ctx, err, "run subprocess")
    }

    // Detect timeout: cmdCtx deadline exceeded means our 10s kicked in.
    if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
        slog.Warn("subprocess skipped", "op", op, "timeout", r.timeout)
        return nil, context.DeadlineExceeded
    }
    if cmdErr != nil {
        return nil, errors.Wrapf(ctx, cmdErr, "%s failed", op)
    }
    return output, nil
}
```

Notes:
- Return `context.DeadlineExceeded` directly (NOT wrapped) on timeout so callers' `errors.Is(err, context.DeadlineExceeded)` works without relying on `github.com/bborbe/errors` wrapping semantics. This is a clean, well-known sentinel — wrapping adds noise here.
- `RunWithWarnAndTimeout` and `RunWithWarnAndTimeoutDir` both delegate to `runInternal`; the former passes `dir=""`.
- Do NOT use raw `go func()` anywhere in this package.

## 2. Add `Skipped` tracking to the status

In `pkg/status/status.go`, extend the `Status` struct with boolean fields that record which subprocess calls were skipped:

```go
type Status struct {
    // ... existing fields ...

    // Skipped flags — true when the corresponding subprocess call was
    // cancelled at timeout. Callers should NOT treat the zero value of
    // related fields as authoritative when the matching Skipped flag is true.
    DirtyFileCheckSkipped    bool `json:"dirty_file_check_skipped,omitempty"`
    ContainerRunningSkipped  bool `json:"container_running_skipped,omitempty"`
    GeneratingSpecSkipped    bool `json:"generating_spec_skipped,omitempty"`
    ContainerCountSkipped    bool `json:"container_count_skipped,omitempty"`
}
```

Do NOT change any existing field names or types. Do NOT rename any JSON tags.

## 3. Wire the helper into `pkg/status/status.go`

Add a `subprocRunner subproc.Runner` field to the `checker` struct (unexported). Add a `subprocRunner` parameter to `NewChecker` as the LAST parameter, and update every call site that constructs the checker. Exact sites (verified by grep):

**Production:**
- `pkg/factory/factory.go:657` — pass `subproc.NewRunner()`
- `pkg/factory/factory.go:712` — pass `subproc.NewRunner()`
- `pkg/factory/factory.go:897` (inside `CreateCombinedStatusCommand`) — pass `subproc.NewRunner()`

**Tests (pkg/status/status_test.go):** 9 sites — lines 52, 287, 312, 459, 486, 511, 543, 566, 589. Pass the generated counterfeiter fake (see §9) configured to return empty output + nil error unless the test specifically needs other behavior.

Then refactor the three existing subprocess calls in `pkg/status/status.go`:

### 3a. `isContainerRunning` (currently around `cmd := exec.CommandContext(ctx, "docker", "ps", ...)`):

Old:
```go
cmd := exec.CommandContext(ctx, "docker", "ps", "--filter", "name="+containerName, "--format", "{{.Names}}")
out, err := cmd.Output()
if err != nil {
    slog.Debug("docker ps failed", "container", containerName, "err", err)
    return false
}
return strings.Contains(string(out), containerName)
```

New — note the additional return value signaling "skipped":
```go
// isContainerRunning checks if a Docker container is running.
// Returns (running, skipped). When skipped is true the caller MUST treat
// the running value as unknown and surface the skip to the user.
func (s *checker) isContainerRunning(ctx context.Context, containerName string) (running bool, skipped bool) {
    if containerName == "" {
        return false, false
    }
    op := "docker ps --filter name=" + containerName
    out, err := s.subprocRunner.RunWithWarnAndTimeout(ctx, op, "docker", "ps", "--filter", "name="+containerName, "--format", "{{.Names}}")
    if errors.Is(err, context.DeadlineExceeded) {
        return false, true
    }
    if err != nil {
        slog.Debug("docker ps failed", "container", containerName, "err", err)
        return false, false
    }
    return strings.Contains(string(out), containerName), false
}
```

Update the caller of `isContainerRunning` in `populateExecutingPrompt` at `pkg/status/status.go:393`. Old:
```go
st.ContainerRunning = s.isContainerRunning(ctx, executing.Container)
```
New:
```go
running, skipped := s.isContainerRunning(ctx, executing.Container)
st.ContainerRunning = running
st.ContainerRunningSkipped = skipped
```

### 3b. `populateGeneratingSpec`:

Replace the `exec.CommandContext(...).Output()` pattern with `s.subprocRunner.RunWithWarnAndTimeout(ctx, "docker ps --filter name=dark-factory-gen-", "docker", "ps", ...)`. On `errors.Is(err, context.DeadlineExceeded)` set `st.GeneratingSpecSkipped = true` and return. On other errors, keep the existing `slog.Debug` behavior.

Also replace the existing `bytes.Split([]byte(output), []byte("\n"))` loop with string-based parsing — the helper returns `[]byte` but string splits are simpler:
```go
output := strings.TrimSpace(string(out))
if output == "" {
    return
}
var containerName string
for _, line := range strings.Split(output, "\n") {
    name := strings.TrimSpace(line)
    if strings.HasPrefix(name, genPrefix) {
        containerName = name
        break
    }
}
```
This eliminates the `bytes` import from `pkg/status/status.go` (confirm no other `bytes.*` usage remains before removing).

### 3c. `populateGitWarnings`:

Replace the `exec.CommandContext(ctx, "git", "status", "--porcelain")` pattern. Note this call uses `cmd.Dir = s.projectDir` — you MUST preserve the working directory. Extend the helper to allow a working directory:

**EXTENSION**: Add a second helper method to `subproc.Runner`:

```go
type Runner interface {
    RunWithWarnAndTimeout(ctx context.Context, op string, name string, args ...string) ([]byte, error)
    RunWithWarnAndTimeoutDir(ctx context.Context, op string, dir string, name string, args ...string) ([]byte, error)
}
```

`RunWithWarnAndTimeoutDir` is identical to `RunWithWarnAndTimeout` but sets `cmd.Dir = dir` before running. Keep the implementation DRY — factor a common helper.

Use `RunWithWarnAndTimeoutDir(ctx, "git status --porcelain", s.projectDir, "git", "status", "--porcelain")`.

On `errors.Is(err, context.DeadlineExceeded)` set `st.DirtyFileCheckSkipped = true` and return (leave `DirtyFileCount = 0`). On other errors, keep the existing `slog.Debug`. On success, count dirty files as before.

**Keep the existing `st.GitIndexLock` detection (the `os.Stat` on `.git/index.lock`) unchanged** — that is a fast local stat, not a subprocess.

## 4. Wire the helper into `pkg/executor/checker.go`

`dockerContainerCounter.CountRunning` must also use the helper. Inject `subproc.Runner` via constructor:

```go
func NewDockerContainerCounter(runner subproc.Runner) ContainerCounter {
    return &dockerContainerCounter{runner: runner}
}

type dockerContainerCounter struct {
    runner subproc.Runner
}
```

Update `CountRunning`:

Old:
```go
cmd := exec.CommandContext(ctx, "docker", "ps", "--filter", "label=dark-factory.project", "--format", "{{.Names}}")
var out strings.Builder
cmd.Stdout = &out
if err := cmd.Run(); err != nil {
    return 0, errors.Wrap(ctx, err, "docker ps for container count")
}
```

New — note the explicit sentinel branch to preserve `errors.Is` on timeout:
```go
out, err := c.runner.RunWithWarnAndTimeout(ctx, "docker ps --filter label=dark-factory.project", "docker", "ps", "--filter", "label=dark-factory.project", "--format", "{{.Names}}")
if errors.Is(err, context.DeadlineExceeded) {
    return 0, err // return sentinel unwrapped so caller's errors.Is check still works
}
if err != nil {
    return 0, errors.Wrap(ctx, err, "docker ps for container count")
}
```

Then parse `out` instead of `out.String()`.

**Skipped handling for CountRunning**: When `errors.Is(err, context.DeadlineExceeded)`, the caller in `GetStatus` should set `status.ContainerCountSkipped = true` and leave `ContainerCount=0`, `ContainerMax=0` unchanged. Update the `GetStatus` block that currently calls `s.containerCounter.CountRunning(ctx)`:

```go
if s.containerCounter != nil && s.maxContainers > 0 {
    count, err := s.containerCounter.CountRunning(ctx)
    if errors.Is(err, context.DeadlineExceeded) {
        status.ContainerCountSkipped = true
    } else if err != nil {
        slog.Debug("failed to count running containers for status", "error", err)
    } else {
        status.ContainerCount = count
        status.ContainerMax = s.maxContainers
    }
}
```

Update every call site of `NewDockerContainerCounter()` to pass `subproc.NewRunner()`. Exact sites (verified by grep):
- `pkg/factory/factory.go:260`
- `pkg/factory/factory.go:372`
- `pkg/factory/factory.go:665`
- `pkg/factory/factory.go:720`
- `pkg/factory/factory.go:905`
- `pkg/factory/factory_test.go:87` — pass `subproc.NewRunner()` (the factory test doesn't exercise timeout behavior; real runner is fine and keeps the factory test simple)

Do NOT change `dockerContainerChecker.IsRunning` or `WaitUntilRunning` in this prompt — those are used by the executor/runner, not status, and the spec explicitly lists only the four call sites above.

## 5. Format skipped fields in the human formatter

In `pkg/status/formatter.go`:

### 5a. `formatDirtyFileWarning` should render `(skipped)` when the check was skipped:

```go
// Replace formatWarnings to also consider DirtyFileCheckSkipped:
func (f *formatter) formatWarnings(b *strings.Builder, st *Status) {
    if !st.GitIndexLock && st.DirtyFileCount == 0 && !st.DirtyFileCheckSkipped {
        return
    }
    b.WriteString("  Warnings:\n")
    if st.GitIndexLock {
        b.WriteString("    \u26a0 .git/index.lock exists \u2014 daemon will skip prompts\n")
    }
    if st.DirtyFileCheckSkipped {
        b.WriteString("    \u26a0 dirty files: (skipped — git status timed out)\n")
    } else if st.DirtyFileCount > 0 {
        f.formatDirtyFileWarning(b, st)
    }
}
```

### 5b. `Format` (container count line) should show `(skipped)` when `ContainerCountSkipped`:

```go
// Container count (only when limit is configured OR the check was skipped)
if st.ContainerMax > 0 {
    fmt.Fprintf(&b, "  Containers: %d/%d (system-wide)\n", st.ContainerCount, st.ContainerMax)
} else if st.ContainerCountSkipped {
    b.WriteString("  Containers: (skipped — docker ps timed out)\n")
}
```

### 5c. `formatCurrentPrompt` should handle `ContainerRunningSkipped`:

Where the current code writes `containerStatus += " (running)"` vs `" (not running)"`, add a skipped case:

```go
if st.Container != "" {
    containerStatus := st.Container
    switch {
    case st.ContainerRunningSkipped:
        containerStatus += " (status unknown — docker ps skipped)"
    case st.ContainerRunning:
        containerStatus += " (running)"
    default:
        containerStatus += " (not running)"
    }
    fmt.Fprintf(b, "  Container:  %s\n", containerStatus)
}
```

### 5d. `GeneratingSpecSkipped` notice: in `Format`, only when `CurrentPrompt == ""` (i.e. the same condition that runs `populateGeneratingSpec`) AND `GeneratingSpecSkipped == true`:

```go
if st.CurrentPrompt == "" && st.GeneratingSpecSkipped {
    b.WriteString("  (generating-spec check skipped — docker ps timed out)\n")
}
```

## 6. Tests for the helper (`pkg/subproc/subproc_test.go`)

Use Ginkgo + Gomega matching the style of `pkg/status/status_test.go`. Create a suite file `pkg/subproc/subproc_suite_test.go` mirroring `status_suite_test.go`.

Tests:

1. **Fast command returns output** — `RunWithWarnAndTimeoutDir(ctx, "true", "/", "true")` returns `nil` error, empty output.
2. **Slow command warns then succeeds** — use `sh -c "sleep 0.05 && echo done"` (50ms sleep) with `NewRunnerWithThresholds(30*time.Millisecond, 1*time.Second)`. 50ms > 30ms warn threshold so warn fires; 50ms < 1s timeout so cmd succeeds. Capture slog output (see test hygiene below) and assert the "subprocess slow" message fires. Output must eventually be `done\n`.
3. **Slow command times out** — `sh -c "sleep 5"` with `NewRunnerWithThresholds(10*time.Millisecond, 50*time.Millisecond)`. Assert `errors.Is(err, context.DeadlineExceeded)` is true. Assert the "subprocess skipped" slog message fired.
4. **Parent context cancellation propagates** — pass a ctx that is cancelled after 20ms with a 1s timeout runner. Assert the command is killed and returns promptly.
5. **No goroutine leak** — `go.uber.org/goleak` is already available in the dependency tree (verify with `grep goleak go.sum`). Run 100 fast commands in a tight loop and call `goleak.VerifyNone(t)` at the end.

**IMPORTANT test hygiene** (see `go-testing-guide.md`):
- Tests must complete in well under 1 second (keep thresholds in milliseconds).
- Do NOT use `time.Sleep` in the main test body — drive timing through the runner's thresholds.
- Use Ginkgo `It(...)` blocks, not raw `testing.T` subtests.
- When capturing slog output via `slog.SetDefault(newHandler)`, save the previous default in `BeforeEach` and restore it in `AfterEach` to avoid cross-test contamination (slog default is global state).

## 7. Tests for the status skip behavior (`pkg/status/status_test.go`)

Add Ginkgo blocks that construct a `checker` with the counterfeiter-generated `mocks.SubprocRunner` (see §9). Configure it per test to return `context.DeadlineExceeded` for the subprocess call under test. Example:

```go
runner := &mocks.SubprocRunner{}
runner.RunWithWarnAndTimeoutDirReturns(nil, context.DeadlineExceeded)
checker := status.NewChecker(..., runner)
```

New tests:
- When `git status` times out, `GetStatus` sets `DirtyFileCheckSkipped=true`, `DirtyFileCount=0`.
- When `docker ps` for generating-spec times out, `GeneratingSpecSkipped=true`.
- When `isContainerRunning`'s `docker ps` times out, `ContainerRunningSkipped=true`.
- `GetStatus` still returns `status, nil` (no error surfaced to the caller) when any subprocess times out.

## 8. Formatter tests (`pkg/status/formatter_test.go`)

Add Ginkgo blocks:
- Given `Status{DirtyFileCheckSkipped: true}`, `Format` output contains `dirty files: (skipped`.
- Given `Status{ContainerRunningSkipped: true, Container: "x", CurrentPrompt: "p"}`, output contains `status unknown`.
- Given `Status{ContainerCountSkipped: true}`, output contains `Containers: (skipped`.

## 9. Counterfeiter mock (REQUIRED — repo style)

Add a counterfeiter generate directive at the top of `pkg/subproc/subproc.go`:

```go
//counterfeiter:generate -o ../../mocks/subproc-runner.go --fake-name SubprocRunner . Runner
```

Then run `go generate ./pkg/subproc/...` to create the mock at `mocks/subproc-runner.go`. Use the generated `mocks.SubprocRunner` fake in `pkg/status/status_test.go` and any test that needs to simulate timeouts. Do NOT write a hand-rolled fake.

## 10. CHANGELOG entry

Insert an `## Unreleased` section into `CHANGELOG.md` immediately AFTER the header preamble and BEFORE the current top version entry (`## v0.109.0` or whatever is latest at implementation time). If `## Unreleased` already exists, append the bullet under it.

Example:

```markdown
## Unreleased
- Bound `dark-factory status` subprocess calls (git status, docker ps) with a 3s warning and 10s hard timeout. Skipped calls are marked `(skipped)` in human output and as `*_skipped: true` flags in JSON output.
```

## 11. Cleanup

After the refactor, `populateGeneratingSpec` no longer uses `bytes.Split` (it receives `[]byte` from the helper and can use `strings.Split` on the string form or split bytes differently). Remove the `"bytes"` import from `pkg/status/status.go` if it becomes unused.

## 12. Final verification

Run `make precommit`. All existing tests must still pass, and new tests for the helper and the skipped-state formatting must pass.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT change the public API of `status.Checker.GetStatus` or the JSON schema of existing `Status` fields (only ADD the four `*Skipped` fields with `omitempty`)
- Do NOT change `dockerContainerChecker.IsRunning` / `WaitUntilRunning` (different call path; out of scope)
- Warning and skip messages must go to stderr via `slog.Warn` — NOT to stdout (keeps JSON output clean)
- Thresholds are compile-time constants (3s warn, 10s timeout). No config field, no flag.
- Preserve `cmd.Dir` for the `git status` call — use the `RunWithWarnAndTimeoutDir` variant
- Preserve the existing `st.GitIndexLock` detection (fast `os.Stat`, not a subprocess) unchanged
- All file paths in prompts are repo-relative
- Use `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors` for error wrapping (match repo convention)
- Use `slog` (log/slog stdlib) for all logging (match repo convention)
- All new tests use Ginkgo + Gomega (match `pkg/status/status_suite_test.go` style)
- Use `errors.Is(err, context.DeadlineExceeded)` to detect timeout — do NOT string-match
- No goroutine leaks: every spawned goroutine must terminate via a `done` channel close OR timer fire
</constraints>

<verification>
Run `make precommit` — must pass.
Run `make test` — all tests including the new `pkg/subproc/` tests must pass.

Manual smoke test (describe in comment, do not execute in CI): in a slow git repo, run `dark-factory status`. Observe that the command returns in ~10s even if git is hung, a warning appears on stderr after ~3s, and a skip message appears at ~10s. JSON output (`dark-factory status --json`) contains `"dirty_file_check_skipped": true` and no phantom zeros for skipped fields.

Also: grep the repo for existing consumers of JSON fields likely to collide with the new schema:
- `grep -rn "dirty_file_count\|container_running\|generating_spec\|container_count" --include="*.go" --include="*.md" --include="*.sh"`
- Confirm adding the four `*_skipped` fields (with `omitempty`) does not break any existing test assertion or script.
</verification>
