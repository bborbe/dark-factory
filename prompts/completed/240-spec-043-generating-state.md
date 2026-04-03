---
status: completed
spec: ["043"]
summary: 'Added generating lifecycle state to spec: StatusGenerating constant, Generating timestamp field, generator sets spec to generating before Execute() and resets to approved on non-cancellation failure, and daemon startup resets orphaned generating specs via resumeOrResetGenerating.'
container: dark-factory-240-spec-043-generating-state
dark-factory-version: v0.89.1-dirty
created: "2026-04-03T09:30:00Z"
queued: "2026-04-03T09:50:42Z"
started: "2026-04-03T09:50:44Z"
completed: "2026-04-03T10:11:49Z"
---

<summary>
- Specs gain a `generating` lifecycle state that marks when a generation container is actively running
- The generator sets the spec to `generating` (with a timestamp) immediately before launching the Docker container
- On successful generation the spec transitions to `prompted` as before; on non-cancellation failure it resets to `approved` so the daemon retries automatically
- On daemon startup, specs left in `generating` state are checked: if the container is gone the spec resets to `approved`; if the container is still alive it is left alone (handled by reattach on next scan)
- Warning is logged whenever a spec is reset from `generating` to `approved`
- No existing behavior is changed for specs in `approved`, `prompted`, `verifying`, or `completed` states
</summary>

<objective>
Add a `generating` status to the spec lifecycle so the runtime health check loop (added in the next prompt) can identify which specs have an active generation container and need monitoring. The generator sets the spec to `generating` before launching Execute() and resets it to `approved` on failure, ensuring the spec is always retried cleanly.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these files before making changes:
- `pkg/spec/spec.go` — `Status` constants (~line 28), `Frontmatter` struct (~line 47), `SetStatus()` (~line 121), `stampOnce()` (~line 79). Add `StatusGenerating` here and `Generating string` to Frontmatter.
- `pkg/generator/generator.go` — `Generate()` (~line 84). This is where spec status transitions happen. The spec status must change to `generating` before `Execute()` is called at line ~113.
- `pkg/runner/runner.go` — `Run()` (~line 113). The startup sequence calls `resumeOrResetExecuting` for prompts. A similar startup reset for specs in `generating` state must be added here.
- `pkg/runner/lifecycle.go` — `resumeOrResetExecuting()` (~line 103) and `resumeOrResetExecutingEntry()` (~line 133). Use these as a model for the new `resumeOrResetGenerating()` function.
- `pkg/runner/runner_test.go` — see `Describe("resumeOrResetExecuting", ...)` at line ~715. Follow the same test style for the new startup reset.
- `mocks/` — check for `ContainerChecker` mock (used in runner tests).
- `pkg/generator/generator_test.go` — add tests for state transitions here.
</context>

<requirements>
**Step 1 — Add `StatusGenerating` to `pkg/spec/spec.go`**

1a. Add a new constant after `StatusApproved`:
```go
// StatusGenerating indicates the spec is currently being processed to generate prompts.
StatusGenerating Status = "generating"
```

1b. Add `Generating string \`yaml:"generating,omitempty"\`` to `Frontmatter`, after the `Approved` field:
```go
type Frontmatter struct {
    Status     string   `yaml:"status"`
    Tags       []string `yaml:"tags,omitempty"`
    Approved   string   `yaml:"approved,omitempty"`
    Generating string   `yaml:"generating,omitempty"`
    Prompted   string   `yaml:"prompted,omitempty"`
    Verifying  string   `yaml:"verifying,omitempty"`
    Completed  string   `yaml:"completed,omitempty"`
    Branch     string   `yaml:"branch,omitempty"`
    Issue      string   `yaml:"issue,omitempty"`
}
```

1c. Add a case to `SetStatus()` for `StatusGenerating`:
```go
case StatusGenerating:
    s.stampOnce(&s.Frontmatter.Generating)
```

**Step 2 — Update `pkg/generator/generator.go` to set/clear `generating` state**

2a. In `Generate()`, after loading the spec path for generation but BEFORE calling `g.executor.Execute()`, load the spec file and set its status to `generating`:

```go
// Mark spec as generating before launching the container
sf, err := spec.Load(ctx, specPath, g.currentDateTimeGetter)
if err != nil {
    return errors.Wrap(ctx, err, "load spec file before generation")
}
sf.SetStatus(string(spec.StatusGenerating))
if err := sf.Save(ctx); err != nil {
    return errors.Wrap(ctx, err, "save spec file as generating")
}
```

Place this block BEFORE the `beforeFiles` snapshot (before step e at line ~106).

2b. After `g.executor.Execute()` returns an error (non-cancellation), reset the spec back to `approved` before returning:

```go
if err := g.executor.Execute(ctx, promptContent, logFile, containerName); err != nil {
    // Reset spec to approved so it will be retried, unless cancelled
    if ctx.Err() == nil && !errors.Is(err, context.Canceled) {
        if resetErr := resetSpecToApproved(ctx, specPath, g.currentDateTimeGetter); resetErr != nil {
            slog.Warn("failed to reset spec to approved after generation failure",
                "spec", specBasename, "error", resetErr)
        }
    }
    return errors.Wrap(ctx, err, "execute spec generator")
}
```

2c. Add a package-level helper in `generator.go`:
```go
// resetSpecToApproved reloads the spec file and resets its status to approved.
func resetSpecToApproved(ctx context.Context, specPath string, currentDateTimeGetter libtime.CurrentDateTimeGetter) error {
    sf, err := spec.Load(ctx, specPath, currentDateTimeGetter)
    if err != nil {
        return errors.Wrap(ctx, err, "load spec for reset")
    }
    sf.SetStatus(string(spec.StatusApproved))
    return errors.Wrap(ctx, sf.Save(ctx), "save spec after reset")
}
```

2d. The `reattachAndFinalize()` path already handles the case when the container is running on restart — no changes needed there. The spec was already set to `generating` before the daemon was restarted, so the health check loop (next prompt) will handle dead containers on restart.

**Step 3 — Add startup reset for `generating` specs in `pkg/runner/`**

3a. Add a new function `resumeOrResetGenerating()` in `pkg/runner/lifecycle.go` (following the exact pattern of `resumeOrResetExecuting()`):

```go
// resumeOrResetGenerating scans specsInProgressDir for specs with "generating" status.
// If the generation container is still running, the spec is left as-is (the specwatcher
// will reattach on next scan). If the container is gone, the spec is reset to approved.
func resumeOrResetGenerating(
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
        return errors.Wrap(ctx, err, "read specs in-progress dir")
    }
    for _, entry := range entries {
        if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
            continue
        }
        if err := resumeOrResetGeneratingEntry(ctx, specsInProgressDir, entry.Name(), checker, currentDateTimeGetter); err != nil {
            return err
        }
    }
    return nil
}

// resumeOrResetGeneratingEntry handles a single spec file: checks container liveness and resets if gone.
func resumeOrResetGeneratingEntry(
    ctx context.Context,
    specsInProgressDir string,
    name string,
    checker executor.ContainerChecker,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
    path := filepath.Join(specsInProgressDir, name)
    sf, err := spec.Load(ctx, path, currentDateTimeGetter)
    if err != nil || sf == nil {
        return nil
    }
    if spec.Status(sf.Frontmatter.Status) != spec.StatusGenerating {
        return nil
    }
    // Derive container name from spec filename (dark-factory-gen-<basename>)
    specBasename := strings.TrimSuffix(name, ".md")
    containerName := "dark-factory-gen-" + specBasename

    running, err := checker.IsRunning(ctx, containerName)
    if err != nil {
        slog.Warn("failed to check spec generation container liveness, resetting spec",
            "file", name, "container", containerName, "error", err)
        running = false
    }
    if running {
        slog.Info("spec generation container still running, leaving as generating",
            "file", name, "container", containerName)
        return nil
    }
    slog.Warn("spec generation container not found, resetting spec to approved",
        "file", name, "container", containerName)
    sf.SetStatus(string(spec.StatusApproved))
    return errors.Wrap(ctx, sf.Save(ctx), "reset generating spec to approved")
}
```

Note: You will need to add `"github.com/bborbe/dark-factory/pkg/spec"` to the imports in `lifecycle.go`.

3b. Add `resumeOrResetGenerating` import to `lifecycle.go`. The function needs:
- `"github.com/bborbe/dark-factory/pkg/spec"` — for `spec.Load`, `spec.Status`, `spec.StatusGenerating`, `spec.StatusApproved`
- `libtime "github.com/bborbe/time"` — already imported

3c. Add `currentDateTimeGetter libtime.CurrentDateTimeGetter` to the `runner` struct if not already present — it IS already present in the struct.

3d. Add a method `resumeOrResetGenerating()` to the `runner` struct in `runner.go`:
```go
// resumeOrResetGenerating selectively resumes or resets generating specs based on container liveness.
func (r *runner) resumeOrResetGenerating(ctx context.Context) error {
    return resumeOrResetGenerating(
        ctx,
        r.specsInProgressDir,
        r.containerChecker,
        r.currentDateTimeGetter,
    )
}
```

3e. Call `r.resumeOrResetGenerating(ctx)` in `runner.Run()` AFTER the existing `r.resumeOrResetExecuting(ctx)` call and BEFORE `r.processor.ResumeExecuting(ctx)`:
```go
// Selectively resume or reset executing prompts based on container liveness
if err := r.resumeOrResetExecuting(ctx); err != nil {
    return errors.Wrap(ctx, err, "resume or reset executing prompts")
}

// Reset any specs left in generating state if their container is gone
if err := r.resumeOrResetGenerating(ctx); err != nil {
    return errors.Wrap(ctx, err, "resume or reset generating specs")
}
```

**Step 4 — Add tests**

4a. In `pkg/generator/generator_test.go`, add tests for state transitions:
- When `Generate()` is called on an `approved` spec, the spec transitions to `generating` before Execute() runs
- When Execute() succeeds and produces prompt files, the spec ends up in `prompted` state
- When Execute() returns a non-cancellation error, the spec is reset to `approved`
- When Execute() is cancelled (ctx.Err() != nil), the spec is NOT reset (stays `generating`)

Use a `mocks.Executor` for the executor and verify spec file status by reading the file from disk. Create a real temporary directory with a real spec file for these tests.

4b. In `pkg/runner/runner_test.go`, add tests under a new `Describe("resumeOrResetGenerating", ...)` block (following the pattern of `Describe("resumeOrResetExecuting", ...)`):
- No-op when specs in-progress dir has no spec files
- Spec in `generating` state with running container → left as `generating`
- Spec in `generating` state with dead container → reset to `approved`, warning logged
- Spec in `approved` state → left as-is (not generating, skip)
- Use real temp files with appropriate YAML frontmatter; mock `containerChecker`

4c. Add tests to `pkg/spec/spec_test.go` (or the existing test file):
- `SetStatus("generating")` sets `Status` to `"generating"` and stamps `Generating` timestamp
- `SetStatus("generating")` does NOT overwrite an existing `Generating` timestamp
- Loading a spec YAML with `generating: "2026-04-03T09:00:00Z"` correctly parses the field
</requirements>

<constraints>
- Reuse existing `containerChecker.IsRunning()` — no new Docker API calls or CLI invocations
- `containerChecker.IsRunning()` must be safe to call concurrently with prompt processing — it is read-only and stateless
- Generation containers follow the `dark-factory-gen-*` naming convention — derive from spec filename, do NOT add a `container` field to spec frontmatter
- Must not interfere with normal prompt execution or the existing `ResumeExecuting` startup logic
- On context cancellation during Execute(), do NOT reset spec to approved (caller is shutting down; daemon restart will detect and reset)
- The `reattachAndFinalize` path in `generator.go` is unchanged — it already handles the container-still-running case on restart
- All existing tests must pass
- New code must follow `github.com/bborbe/errors` for error wrapping (never `fmt.Errorf`)
- Do NOT commit — dark-factory handles git
- Coverage ≥ 80% for changed packages
- Run `go generate -mod=vendor ./pkg/spec/...` if counterfeiter directives exist and mocks need regeneration
</constraints>

<verification>
Run `make precommit` — must pass.

Additional checks:
```bash
# Confirm StatusGenerating constant exists
grep -n "StatusGenerating\|generating" pkg/spec/spec.go

# Confirm Generating timestamp field in Frontmatter
grep -n "Generating" pkg/spec/spec.go

# Confirm generator sets generating before Execute
grep -n "StatusGenerating\|generating" pkg/generator/generator.go

# Confirm startup reset is called in runner
grep -n "resumeOrResetGenerating" pkg/runner/runner.go pkg/runner/lifecycle.go

# Run targeted tests
go test -mod=vendor ./pkg/spec/... ./pkg/generator/... ./pkg/runner/... -v -count=1 2>&1 | tail -50
```
</verification>
