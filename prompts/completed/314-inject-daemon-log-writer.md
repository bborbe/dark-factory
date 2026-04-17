---
status: completed
summary: Made daemon log writer injectable via io.Writer — moved os.Create from runner.Run() to factory.CreateRunner(), updated NewRunner signature, and updated all test call sites to pass nil so no .dark-factory.log is created during tests.
container: dark-factory-314-inject-daemon-log-writer
dark-factory-version: v0.122.0-6-g6b02e84
created: "2026-04-17T12:52:26Z"
queued: "2026-04-17T12:52:26Z"
started: "2026-04-17T12:52:44Z"
completed: "2026-04-17T13:01:25Z"
---

<summary>
- Daemon log writer is injectable instead of hardcoded file creation
- Runner accepts an optional io.Writer for daemon log output
- Tests pass an in-memory writer instead of creating .dark-factory.log on disk
- No stale .dark-factory.log files left in pkg/runner/ after test runs
- Daemon and one-shot modes both still write logs to disk in production
- Production behavior unchanged
</summary>

<objective>
Make the daemon log writer injectable so tests don't create `.dark-factory.log` on the filesystem. Currently `runner.Run()` calls `os.Create(".dark-factory.log")` directly, which leaves a stale file in the test working directory (`pkg/runner/`). Move file creation to the factory and pass an `io.Writer` into the runner.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Key files:
- `pkg/runner/runner.go` — `Run()` method at ~line 131 creates `.dark-factory.log` via `os.Create`
- `pkg/runner/runner.go` — `NewRunner` constructor and `runner` struct
- `pkg/runner/oneshot.go` — one-shot runner (does NOT create log file today, no change needed unless it also does)
- `pkg/factory/factory.go` — `CreateRunner` wires `NewRunner`
- `pkg/runner/runner_test.go` or `pkg/runner/lifecycle_test.go` — existing tests
</context>

<requirements>

## 1. Add `logWriter` field to `runner` struct

In `pkg/runner/runner.go`, add an `io.Writer` field to the `runner` struct:

```go
// Before:
startupLogger         func()
hideGit               bool

// After:
startupLogger         func()
hideGit               bool
logWriter             io.Writer
```

## 2. Add `logWriter` parameter to `NewRunner`

Add an `io.Writer` parameter to `NewRunner` (after `hideGit`):

```go
hideGit bool,
logWriter io.Writer,
```

Wire it in the struct literal:
```go
hideGit:    hideGit,
logWriter:  logWriter,
```

## 3. Replace `os.Create` in `Run()` with the injected writer

In `runner.Run()`, replace the `os.Create(".dark-factory.log")` block (~line 144-154) with:

```go
// Before:
if logFile, err := os.Create(".dark-factory.log"); err != nil {
    slog.Warn("failed to create daemon log file, continuing without", "error", err)
} else {
    defer logFile.Close()
    level := slog.LevelInfo
    if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
        level = slog.LevelDebug
    }
    w := io.MultiWriter(os.Stderr, logFile)
    slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})))
}

// After:
if r.logWriter != nil {
    level := slog.LevelInfo
    if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
        level = slog.LevelDebug
    }
    w := io.MultiWriter(os.Stderr, r.logWriter)
    slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})))
}
```

Also add cleanup: if `r.logWriter` implements `io.Closer`, close it at the end of `Run()` via defer:

```go
// Add right after the logWriter nil check block:
if closer, ok := r.logWriter.(io.Closer); ok {
    defer closer.Close()
}
```

Remove `"os"` from imports if no longer used (but it's likely still used for `os.Stat` in the hideGit check).

## 4. Create the log file in factory's `CreateRunner`

In `pkg/factory/factory.go`, in the `CreateRunner` function, create the log file and pass it to `NewRunner`:

```go
// Before calling runner.NewRunner, add:
var logWriter io.Writer
if logFile, err := os.Create(".dark-factory.log"); err != nil {
    slog.Warn("failed to create daemon log file, continuing without", "error", err)
} else {
    logWriter = logFile
}

// Pass logWriter (which may be nil) as the logWriter parameter to NewRunner
```

The runner closes the writer at the end of `Run()` via the `io.Closer` check in requirement 3.

## 5. Update all `NewRunner` call sites

Grep for all `NewRunner(` calls and add the `logWriter` argument:

```bash
grep -rn "NewRunner(" pkg/ --include='*.go'
```

- Factory: pass the `logFile` created above
- Tests: pass `nil` (no log file needed) or `&bytes.Buffer{}` if test wants to inspect output

## 6. Update runner tests

In test files that construct a runner via `NewRunner`, add `nil` as the `logWriter` argument. The main place is the `newTestRunner` helper in `pkg/runner/runner_test.go` (~line 67) — updating it covers most tests. Also grep for any direct `runner.NewRunner(` calls in test files and add the `nil` argument there too.

## 7. Clean up stale file

Delete `pkg/runner/.dark-factory.log` if it exists (test artifact from the old implementation).

## 8. Run `make precommit`

```bash
cd /workspace && make precommit
```

Verify no `.dark-factory.log` is created in `pkg/runner/` after tests run:
```bash
ls pkg/runner/.dark-factory.log 2>&1  # should say "No such file"
```

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Use `errors.Wrap(ctx, err, ...)` for error wrapping (not `fmt.Errorf`)
- Production behavior must be identical — daemon still writes to `.dark-factory.log`
- Tests must NOT create any files on disk
</constraints>

<verification>
`make precommit` in `/workspace` must pass.

Post-check:
```bash
ls pkg/runner/.dark-factory.log 2>&1  # must not exist
```
</verification>
