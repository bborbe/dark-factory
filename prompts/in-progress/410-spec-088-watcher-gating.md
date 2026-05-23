---
status: approved
spec: [088-disable-auto-prompt-generation]
created: "2026-05-24T00:00:00Z"
queued: "2026-05-23T22:30:51Z"
branch: dark-factory/disable-auto-prompt-generation
---

<summary>
- `NewSpecWatcher` signature updated to accept `disableAutoGenerate bool` as 5th parameter
- `specWatcher` struct gains `disableAutoGenerate bool` field
- `handleFileEvent` short-circuits before `generator.Generate()` when `disableAutoGenerate` is `true`; emits INFO log line: `spec approved — auto-generation disabled, run /dark-factory:generate-prompts-for-spec <path> manually`
- `scanExistingInProgress` honors the same flag (same helper, same gate)
- When `disableAutoGenerate` is `false` (default), behavior is byte-identical to existing code
- `CreateSpecWatcher` in `pkg/factory/factory.go` updated to pass `cfg.DisableAutoGeneratePrompts` through to `NewSpecWatcher`
- New unit tests in `pkg/specwatcher/watcher_test.go` verify gating behavior and skip log
- New unit tests in `main_internal_test.go` verify `--set disableAutoGeneratePrompts=true|false` parses correctly and rejects garbage values
- `make precommit` passes
</summary>

<objective>
Gate the two spec-watcher generation call sites (`handleFileEvent` at line ~154, `scanExistingInProgress` at line ~165) behind the `disableAutoGeneratePrompts` config flag. When the flag is `true`, the watcher logs a one-line INFO skip message and does NOT start the generator container.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Key files to read before making changes:
- `pkg/specwatcher/watcher.go` — full file. `NewSpecWatcher` (lines 32-44), `specWatcher` struct (lines 47-53), `handleFileEvent` (lines 132-161), `scanExistingInProgress` (lines 165-188). Note: `scanExistingInProgress` calls `handleFileEvent` at line 186, so only one gate is needed.
- `pkg/specwatcher/watcher_test.go` — full file. Existing test patterns use Counterfeiter mocks from `mocks.SpecGenerator`. `captureHandler` (lines 24-50) for log assertion.
- `pkg/factory/factory.go` — lines 683-694, `CreateSpecWatcher`. Pass `cfg.DisableAutoGeneratePrompts` to `NewSpecWatcher`.
- `main_internal_test.go` — existing `applySetOverrides` tests (around line 522) for `--set` parsing patterns.

The existing `hideGit`/`autoApprovePrompts` pattern in the watcher does NOT have a gate because those flags affect the container workspace shape, not the trigger decision. This flag is different — it is a direct gate on the `generator.Generate()` call.

The INFO log line must be: `spec approved — auto-generation disabled, run /dark-factory:generate-prompts-for-spec <spec-path> manually`. The message is the operator's recovery path.
</context>

<requirements>

### 1. Update `NewSpecWatcher` signature in `pkg/specwatcher/watcher.go`

Change the function signature from:

```go
func NewSpecWatcher(
    inProgressDir string,
    generator generator.SpecGenerator,
    debounce time.Duration,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) SpecWatcher
```

to:

```go
func NewSpecWatcher(
    inProgressDir string,
    generator generator.SpecGenerator,
    debounce time.Duration,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    disableAutoGenerate bool,
) SpecWatcher
```

### 2. Add `disableAutoGenerate` field to `specWatcher` struct in `pkg/specwatcher/watcher.go`

In the `specWatcher` struct (around line 47-53), add:

```go
disableAutoGenerate bool
```

### 3. Update `NewSpecWatcher` implementation to store the flag

Change the return from:

```go
return &specWatcher{
    inProgressDir:         inProgressDir,
    generator:             generator,
    debounce:              debounce,
    currentDateTimeGetter: currentDateTimeGetter,
}
```

to:

```go
return &specWatcher{
    inProgressDir:         inProgressDir,
    generator:             generator,
    debounce:              debounce,
    currentDateTimeGetter: currentDateTimeGetter,
    disableAutoGenerate:   disableAutoGenerate,
}
```

### 4. Add gating in `handleFileEvent` in `pkg/specwatcher/watcher.go`

In `handleFileEvent` (around lines 149-160), add the gate BEFORE the `w.generator.Generate(...)` call. The current code:

```go
slog.Info("spec file created in in-progress, triggering generation", "path", specPath)

w.mu.Lock()
defer w.mu.Unlock()

if err := w.generator.Generate(ctx, specPath); err != nil {
```

Change to:

```go
if w.disableAutoGenerate {
    slog.Info(
        "spec approved — auto-generation disabled, run /dark-factory:generate-prompts-for-spec <spec-path> manually",
        "path",
        specPath,
    )
    return
}

slog.Info("spec file created in in-progress, triggering generation", "path", specPath)

w.mu.Lock()
defer w.mu.Unlock()

if err := w.generator.Generate(ctx, specPath); err != nil {
```

Note: `scanExistingInProgress` (line 186) calls `handleFileEvent`, so the gate applies to both call sites automatically.

### 5. Update `CreateSpecWatcher` in `pkg/factory/factory.go` to pass the flag

In `pkg/factory/factory.go` (lines 683-694), change:

```go
func CreateSpecWatcher(
    cfg config.Config,
    gen generator.SpecGenerator,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) specwatcher.SpecWatcher {
    return specwatcher.NewSpecWatcher(
        cfg.Specs.InProgressDir,
        gen,
        time.Duration(cfg.DebounceMs)*time.Millisecond,
        currentDateTimeGetter,
    )
}
```

to:

```go
func CreateSpecWatcher(
    cfg config.Config,
    gen generator.SpecGenerator,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
) specwatcher.SpecWatcher {
    return specwatcher.NewSpecWatcher(
        cfg.Specs.InProgressDir,
        gen,
        time.Duration(cfg.DebounceMs)*time.Millisecond,
        currentDateTimeGetter,
        cfg.DisableAutoGeneratePrompts,
    )
}
```

### 6. Add new watcher tests in `pkg/specwatcher/watcher_test.go`

Add tests for the gating behavior. Add the following test cases to the existing `Describe("SpecWatcher", ...)` block:

```go
It("does NOT call generator when disableAutoGenerate is true on new file event", func() {
    gen := &mocks.SpecGenerator{}
    gen.GenerateReturns(nil)

    w := specwatcher.NewSpecWatcher(
        inProgressDir,
        gen,
        200*time.Millisecond,
        libtime.NewCurrentDateTime(),
        true, // disableAutoGenerate = true
    )

    go func() {
        _ = w.Watch(ctx)
    }()

    time.Sleep(100 * time.Millisecond)

    specFile := filepath.Join(inProgressDir, "gated-spec.md")
    content := "---\nstatus: approved\n---\n# Gated Spec\n"
    err := os.WriteFile(specFile, []byte(content), 0600)
    Expect(err).NotTo(HaveOccurred())

    // Generator should NOT be called within 2 seconds
    Consistently(func() int {
        return gen.GenerateCallCount()
    }, 2*time.Second, 50*time.Millisecond).Should(Equal(0))

    cancel()
})

It("logs INFO message when disableAutoGenerate is true", func() {
    handler := &captureHandler{}
    origLogger := slog.Default()
    slog.SetDefault(slog.New(handler))
    defer slog.SetDefault(origLogger)

    gen := &mocks.SpecGenerator{}

    w := specwatcher.NewSpecWatcher(
        inProgressDir,
        gen,
        200*time.Millisecond,
        libtime.NewCurrentDateTime(),
        true, // disableAutoGenerate = true
    )

    go func() {
        _ = w.Watch(ctx)
    }()

    time.Sleep(100 * time.Millisecond)

    specFile := filepath.Join(inProgressDir, "log-spec.md")
    content := "---\nstatus: approved\n---\n# Log Spec\n"
    err := os.WriteFile(specFile, []byte(content), 0600)
    Expect(err).NotTo(HaveOccurred())

    Eventually(func() []string {
        return handler.Messages()
    }, 2*time.Second, 50*time.Millisecond).Should(
        ContainElement(ContainSubstring("auto-generation disabled")),
    )

    cancel()
})

It("does NOT call generator when disableAutoGenerate is true for pre-existing spec on startup", func() {
    gen := &mocks.SpecGenerator{}
    gen.GenerateReturns(nil)

    // Create spec BEFORE starting the watcher.
    specFile := filepath.Join(inProgressDir, "pre-existing-gated.md")
    content := "---\nstatus: approved\n---\n# Pre-existing Gated Spec\n"
    err := os.WriteFile(specFile, []byte(content), 0600)
    Expect(err).NotTo(HaveOccurred())

    w := specwatcher.NewSpecWatcher(
        inProgressDir,
        gen,
        200*time.Millisecond,
        libtime.NewCurrentDateTime(),
        true, // disableAutoGenerate = true
    )

    go func() {
        _ = w.Watch(ctx)
    }()

    // Generator should NOT be called on startup scan
    Consistently(func() int {
        return gen.GenerateCallCount()
    }, 2*time.Second, 50*time.Millisecond).Should(Equal(0))

    cancel()
})

It("calls generator when disableAutoGenerate is false (default behavior)", func() {
    gen := &mocks.SpecGenerator{}
    gen.GenerateReturns(nil)

    w := specwatcher.NewSpecWatcher(
        inProgressDir,
        gen,
        200*time.Millisecond,
        libtime.NewCurrentDateTime(),
        false, // disableAutoGenerate = false (default)
    )

    go func() {
        _ = w.Watch(ctx)
    }()

    time.Sleep(100 * time.Millisecond)

    specFile := filepath.Join(inProgressDir, "enabled-spec.md")
    content := "---\nstatus: approved\n---\n# Enabled Spec\n"
    err := os.WriteFile(specFile, []byte(content), 0600)
    Expect(err).NotTo(HaveOccurred())

    Eventually(func() int {
        return gen.GenerateCallCount()
    }, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 1))

    cancel()
})
```

_(CLI / global-layering tests for `--set`, `computeFieldSources`, and `applyGlobalOverrides` live in rung 1 — they test code added in `1-spec-088-config-field-plumbing.md`. This rung covers watcher gating only.)_

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must continue to pass without modification (the new test cases are additions, not modifications)
- The gate must be BEFORE the mutex lock, not inside it
- When `disableAutoGenerate` is `true`, the spec file remains in `specs/in-progress/` with `status: approved` — no movement, no deletion, no change to any watcher behavior except the generator call
- The INFO log message must contain both "auto-generation disabled" and the spec path
- `scanExistingInProgress` gates via `handleFileEvent` — no separate gate needed
</constraints>

<verification>
```bash
# Watcher gating tests
go test ./pkg/specwatcher/... -v

# Main internal tests
go test . -run "ApplyGlobal|SetOverrides|computeFieldSources" -v

# Verify no regression on default behavior
go test ./pkg/specwatcher/... -v

# Final validation
make precommit
```
</verification>