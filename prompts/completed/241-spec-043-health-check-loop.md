---
status: completed
spec: ["043"]
summary: Added periodic health check loop that detects disappeared executing prompt and generating spec containers at runtime, resetting them to approved within 30-60 seconds
container: dark-factory-241-spec-043-health-check-loop
dark-factory-version: v0.89.1-dirty
created: "2026-04-03T09:30:00Z"
queued: "2026-04-03T09:50:42Z"
started: "2026-04-03T10:11:52Z"
completed: "2026-04-03T10:31:27Z"
---

<summary>
- The daemon now continuously monitors all executing prompt containers every 30 seconds while running
- The daemon also continuously monitors all generating spec containers every 30 seconds while running
- When a prompt container disappears at runtime, the prompt is reset to `approved` within 60 seconds and a `stuck_container` notification is fired
- When a spec generation container disappears at runtime, the spec is reset to `approved` within 60 seconds and a warning is logged
- If the Docker API is unreachable during a check cycle, a warning is logged and the cycle is skipped — no state changes occur
- The health check loop exits cleanly when the daemon context is cancelled (no goroutine leaks)
- All existing prompt processing and spec watcher behavior is unchanged — the health check is purely additive
</summary>

<objective>
Add a periodic health check loop that runs every 30 seconds and detects disappeared containers for both executing prompts and generating specs. This closes the gap left by the startup-only `resumeOrResetExecuting` — when a container dies while the daemon is running, the health check detects it within 30-60 seconds and resets the prompt/spec to `approved` for automatic retry.
</objective>

<context>
Read CLAUDE.md for project conventions.

IMPORTANT: Prompt 1 of this spec (1-spec-043-generating-state.md) must already be applied before this prompt. That prompt added:
- `StatusGenerating Status = "generating"` to `pkg/spec/spec.go`
- `Generating string` field to `spec.Frontmatter`
- `resumeOrResetGenerating()` startup function in `pkg/runner/lifecycle.go`

Read these files before making changes:
- `pkg/runner/runner.go` — `runner` struct (~line 83), `Run()` (~line 113). The health check loop will be added as an additional `run.Func` in the `run.CancelOnFirstError(ctx, runners...)` call at line ~169.
- `pkg/runner/lifecycle.go` — `resumeOrResetExecuting()` and `resumeOrResetGeneratingEntry()` (added by prompt 1). Use as a model for the check functions.
- `pkg/runner/runner_test.go` — understand the testing patterns for the runner. The new loop must be included in the `run.CancelOnFirstError` so it exits cleanly when context is cancelled.
- `pkg/executor/checker.go` — `ContainerChecker` interface and `IsRunning(ctx, name) (bool, error)`.
- `pkg/spec/spec.go` — `StatusGenerating`, `spec.Load()`, `spec.Status`.
- `pkg/prompt/prompt.go` — `ExecutingPromptStatus`, `MarkApproved()`, `Manager.Load()`.
- `pkg/notifier/notifier.go` — `Event` struct and `EventType` constants.
- `mocks/` — `ContainerChecker`, `Manager`, `Notifier` mocks available for tests.
</context>

<requirements>
**Step 1 — Create `pkg/runner/health_check.go`**

Create a new file `pkg/runner/health_check.go` with the health check logic:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner

import (
    "context"
    "log/slog"
    "os"
    "path/filepath"
    "strings"
    "time"

    "github.com/bborbe/errors"
    libtime "github.com/bborbe/time"

    "github.com/bborbe/dark-factory/pkg/executor"
    "github.com/bborbe/dark-factory/pkg/notifier"
    "github.com/bborbe/dark-factory/pkg/prompt"
    "github.com/bborbe/dark-factory/pkg/spec"
)
```

1a. Add `runHealthCheckLoop()` — the main loop function (this is what gets added to `run.CancelOnFirstError`):

```go
// runHealthCheckLoop runs periodic container health checks every interval.
// It checks prompts in executing state and specs in generating state.
// Returns nil when ctx is cancelled (clean shutdown).
func runHealthCheckLoop(
    ctx context.Context,
    interval time.Duration,
    inProgressDir string,
    specsInProgressDir string,
    checker executor.ContainerChecker,
    mgr prompt.Manager,
    n notifier.Notifier,
    projectName string,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            slog.Debug("running container health check")
            if err := checkExecutingPrompts(ctx, inProgressDir, checker, mgr, n, projectName); err != nil {
                slog.Warn("health check for executing prompts failed", "error", err)
            }
            if err := checkGeneratingSpecs(ctx, specsInProgressDir, checker, currentDateTimeGetter); err != nil {
                slog.Warn("health check for generating specs failed", "error", err)
            }
        }
    }
}
```

1b. Add `checkExecutingPrompts()` — checks all prompts in `executing` state:

```go
// checkExecutingPrompts scans inProgressDir for prompts in executing state and resets any
// whose container is no longer running.
func checkExecutingPrompts(
    ctx context.Context,
    inProgressDir string,
    checker executor.ContainerChecker,
    mgr prompt.Manager,
    n notifier.Notifier,
    projectName string,
) error {
    entries, err := os.ReadDir(inProgressDir)
    if err != nil {
        if os.IsNotExist(err) {
            return nil
        }
        return errors.Wrap(ctx, err, "read in-progress dir for health check")
    }
    for _, entry := range entries {
        if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
            continue
        }
        path := filepath.Join(inProgressDir, entry.Name())
        pf, err := mgr.Load(ctx, path)
        if err != nil || pf == nil {
            continue
        }
        if prompt.PromptStatus(pf.Frontmatter.Status) != prompt.ExecutingPromptStatus {
            continue
        }
        containerName := pf.Frontmatter.Container
        running, err := checker.IsRunning(ctx, containerName)
        if err != nil {
            slog.Warn("health check: failed to check prompt container, skipping",
                "file", entry.Name(), "container", containerName, "error", err)
            continue
        }
        if running {
            slog.Debug("health check: prompt container running", "file", entry.Name(), "container", containerName)
            continue
        }
        slog.Warn("health check: prompt container gone, resetting to approved",
            "file", entry.Name(), "container", containerName)
        if n != nil {
            _ = n.Notify(ctx, notifier.Event{
                ProjectName: projectName,
                EventType:   "stuck_container",
                PromptName:  entry.Name(),
            })
        }
        pf.MarkApproved()
        if err := pf.Save(ctx); err != nil {
            slog.Warn("health check: failed to save reset prompt",
                "file", entry.Name(), "error", err)
        }
    }
    return nil
}
```

Key rule: on Docker API error for a specific prompt, `continue` (skip that prompt) — do NOT reset on check failure. Only reset when `IsRunning()` returns `false` with `err == nil`.

1c. Add `checkGeneratingSpecs()` — checks all specs in `generating` state:

```go
// checkGeneratingSpecs scans specsInProgressDir for specs in generating state and resets any
// whose generation container is no longer running.
func checkGeneratingSpecs(
    ctx context.Context,
    specsInProgressDir string,
    checker executor.ContainerChecker,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
    entries, err := os.ReadDir(specsInProgressDir)
    if err != nil {
        if os.IsNotExist(err) {
            return nil
        }
        return errors.Wrap(ctx, err, "read specs in-progress dir for health check")
    }
    for _, entry := range entries {
        if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
            continue
        }
        path := filepath.Join(specsInProgressDir, entry.Name())
        sf, err := spec.Load(ctx, path, currentDateTimeGetter)
        if err != nil || sf == nil {
            continue
        }
        if spec.Status(sf.Frontmatter.Status) != spec.StatusGenerating {
            continue
        }
        specBasename := strings.TrimSuffix(entry.Name(), ".md")
        containerName := "dark-factory-gen-" + specBasename
        running, err := checker.IsRunning(ctx, containerName)
        if err != nil {
            slog.Warn("health check: failed to check spec container, skipping",
                "file", entry.Name(), "container", containerName, "error", err)
            continue
        }
        if running {
            slog.Debug("health check: spec container running", "file", entry.Name(), "container", containerName)
            continue
        }
        slog.Warn("health check: spec generation container gone, resetting to approved",
            "file", entry.Name(), "container", containerName)
        sf.SetStatus(string(spec.StatusApproved))
        if err := sf.Save(ctx); err != nil {
            slog.Warn("health check: failed to save reset spec",
                "file", entry.Name(), "error", err)
        }
    }
    return nil
}
```

**Step 2 — Add a method to the `runner` struct and wire into `Run()`**

2a. Add a `runHealthCheckLoop` method to the `runner` struct in `runner.go`:

```go
// healthCheckLoop runs the periodic container health check loop.
func (r *runner) healthCheckLoop(ctx context.Context) error {
    return runHealthCheckLoop(
        ctx,
        30*time.Second,
        r.inProgressDir,
        r.specsInProgressDir,
        r.containerChecker,
        r.promptManager,
        r.notifier,
        r.projectName,
        r.currentDateTimeGetter,
    )
}
```

2b. In `Run()`, append `r.healthCheckLoop` to the existing `runners` slice before the `run.CancelOnFirstError` call. Do NOT replace the existing slice — it has conditional entries (server, reviewPoller, specWatcher) that must be preserved. Add this line after all existing `runners = append(...)` calls:

```go
runners = append(runners, r.healthCheckLoop)
```

**Step 3 — Create `pkg/runner/health_check_test.go`**

Create a test file `pkg/runner/health_check_test.go` with `package runner_test`.

3a. Test `checkExecutingPrompts`:
- No executing prompts in dir → no calls to checker, no notifications
- Prompt in `approved` state → skipped (not checked)
- Prompt in `executing` state, container running → no reset, no notification
- Prompt in `executing` state, container not running → `MarkApproved()` called, notification fired with `EventType = "stuck_container"`
- Prompt in `executing` state, `IsRunning()` returns error → warning logged, no reset, no notification (graceful degradation)
- Two executing prompts, one container gone, one running → only the dead one is reset

For these tests: create real temp directories and write `.md` files with appropriate YAML frontmatter. Use `mocks.ContainerChecker`, `mocks.Manager`, and `mocks.Notifier`.

For Manager.Load(), configure it to return a real `prompt.PromptFile` created with `prompt.NewPromptFile()` so that `MarkApproved()` and `Save()` work correctly against the temp files.

3b. Test `checkGeneratingSpecs`:
- No generating specs in dir → no calls to checker
- Spec in `approved` state → skipped
- Spec in `generating` state, container running → not reset
- Spec in `generating` state, container not running → spec file is reset to `approved`
- Spec in `generating` state, `IsRunning()` returns error → warning logged, not reset (graceful degradation)

For these tests: create real temp directories and write real `.md` files with appropriate YAML frontmatter. Use `mocks.ContainerChecker`. Load spec files directly using `spec.Load()` after the check to verify status.

3c. Test `runHealthCheckLoop`:
- Cancelling context stops the loop without error
- Loop calls `checkExecutingPrompts` and `checkGeneratingSpecs` at each tick (use a short interval like 50ms for testing and assert at least one tick occurs)

Use `mocks.ContainerChecker` and `mocks.Manager` with stub implementations for test-level control.

3d. Test that `runner.Run()` includes the health check loop — write an integration-style test that verifies the loop runs (use a very short interval isn't practical here, but verify that when context is cancelled cleanly, Run() returns nil). You can check by verifying `containerChecker` is not called during startup when no executing prompts or generating specs exist. This verifies the loop is wired but not intrusive.

**Step 4 — Write changelog entry**

Add to `CHANGELOG.md` under `## Unreleased` (create section if needed):
```
- feat: Periodic container health check loop detects disappeared executing prompt containers and generating spec containers at runtime, resetting them to approved within 30-60 seconds without requiring a daemon restart
```
</requirements>

<constraints>
- Reuse existing `containerChecker.IsRunning()` — no new Docker API calls or CLI invocations
- `IsRunning()` is safe to call concurrently with prompt processing (read-only)
- On Docker API error during a check, log a warning and skip that item — do NOT reset on check failure
- Only reset when `IsRunning()` returns `(false, nil)` — confirmed not running
- Health check interval is 30 seconds — hardcoded, not configurable (per spec non-goals)
- The health check loop must respect context cancellation — use `select` with `ctx.Done()` and `ticker.C`
- Do NOT use raw `go func()` — the loop is wired via `run.CancelOnFirstError` as a `run.Func`
- The loop must return `nil` (not an error) on clean context cancellation
- Checking loop errors are logged as warnings (not fatal) — use `slog.Warn`; the loop itself continues
- Must not interfere with the `resumeOrResetExecuting` startup logic or the specwatcher
- Health check race: if a prompt/spec completes normally between ticker firings, `IsRunning()` returns false but the status is already `completed` or `prompted` — the check skips non-executing/non-generating items, so no spurious reset
- Do NOT commit — dark-factory handles git
- All existing tests must pass
- New code must follow `github.com/bborbe/errors` for error wrapping (never `fmt.Errorf`)
- Coverage ≥ 80% for changed packages (`pkg/runner/`)
</constraints>

<verification>
Run `make precommit` — must pass.

Additional checks:
```bash
# Confirm health_check.go exists with the three functions
grep -n "runHealthCheckLoop\|checkExecutingPrompts\|checkGeneratingSpecs" pkg/runner/health_check.go

# Confirm health check loop is wired into Run()
grep -n "healthCheckLoop" pkg/runner/runner.go

# Confirm test file exists
ls pkg/runner/health_check_test.go

# Run targeted tests
go test -mod=vendor ./pkg/runner/... -v -count=1 2>&1 | tail -60

# Verify coverage
go test -mod=vendor -coverprofile=/tmp/cover.out ./pkg/runner/... && go tool cover -func=/tmp/cover.out | grep -E "health_check|total"
```
</verification>
