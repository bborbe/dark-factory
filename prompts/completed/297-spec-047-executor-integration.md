---
status: completed
spec: [047-go-stream-formatter]
summary: 'Wired pkg/formatter.Formatter into executor.go so every prompt run (Execute and Reattach) produces two log files: raw JSONL (container stdout verbatim) and human-readable formatted log; added YOLO_OUTPUT=json env var to docker command; bumped DefaultContainerImage to v0.6.0; updated NewDockerExecutor signature, export_test.go, executor_test.go, and factory.go; added streaming pipeline unit tests (9a-9e); extracted runWithFormatterPipeline helper to satisfy funlen lint limit.'
container: dark-factory-297-spec-047-executor-integration
dark-factory-version: v0.111.2
created: "2026-04-16T17:22:00Z"
queued: "2026-04-16T17:27:46Z"
started: "2026-04-16T17:51:10Z"
completed: "2026-04-16T18:13:06Z"
branch: dark-factory/go-stream-formatter
---

<summary>
- The container image version used by dark-factory is bumped to the first release that supports raw JSON passthrough
- The container is told to emit raw newline-delimited JSON via a fixed environment variable, so stdout carries structured messages instead of pre-formatted text
- Every prompt run produces TWO log files alongside each other: one containing the verbatim raw JSON, one containing a human-readable formatted rendering
- The raw log path is derived automatically from the formatted log path (same directory, same prefix, different extension)
- The raw log file is opened (or fails fast with a clear error naming the path) before the container starts
- Terminal output for the operator stays the same: formatted lines go to stdout live as the run progresses
- Reattaching to a running container produces the same two-file output as the primary run path
- The executor accepts an injected formatter dependency so tests can stub it
- A changelog entry under Unreleased documents the new dual-log behavior and the image bump
- Executor unit tests assert both files are created, the raw file is verbatim container stdout, and the formatted file is readable
</summary>

<objective>
Wire the `pkg/formatter.Formatter` (created in prompt 1) into `pkg/executor/executor.go` so that every prompt run — both primary `Execute` and `Reattach` — produces two log files: a raw `.jsonl` file containing the container's unmodified stdout and a `.log` file containing human-readable formatted output. The container is told to emit raw stream-json via `YOLO_OUTPUT=json`. The `DefaultContainerImage` is bumped to a version that supports this mode. Prompt 1 must be merged first.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Files to read in full before editing:
- `pkg/executor/executor.go` — the full file; focus on:
  - `NewDockerExecutor` (line ~48)
  - `dockerExecutor` struct fields (line ~78)
  - `Execute` (line ~93) — `prepareLogFile`, `cmd.Stdout/Stderr` setup, `buildRunFuncs` call
  - `Reattach` (line ~226) — same patterns
  - `buildDockerCommand` (line ~422) — env var block where `YOLO_OUTPUT=json` must be added
  - `prepareLogFile` (line ~358)
- `pkg/executor/export_test.go` — exported test helpers; understand pattern before adding new ones
- `pkg/executor/executor_test.go` — understand how executor tests are structured (mocked commandRunner)
- `pkg/factory/factory.go` — `createDockerExecutor` helper; add `formatter.NewFormatter()` argument
- `pkg/const.go` — `DefaultContainerImage` constant
- `pkg/formatter/formatter.go` — the `Formatter` interface from prompt 1
- `mocks/formatter.go` — the Counterfeiter fake from prompt 1
</context>

<requirements>

## 1. Bump `DefaultContainerImage` in `pkg/const.go`

Change the constant to `v0.6.0`:

```go
const DefaultContainerImage = "docker.io/bborbe/claude-yolo:v0.6.0"
```

This is the minimum claude-yolo version that supports `YOLO_OUTPUT=json` raw passthrough mode.

## 2. Add `YOLO_OUTPUT=json` to `buildDockerCommand`

In the env var block inside `buildDockerCommand`, add `YOLO_OUTPUT=json` immediately after the existing `ANTHROPIC_MODEL` line:

```go
args = append(args,
    "-e", "YOLO_PROMPT_FILE=/tmp/prompt.md",
    "-e", "ANTHROPIC_MODEL="+e.model,
    "-e", "YOLO_OUTPUT=json",          // ← add this line
)
```

**Do not change any other line in `buildDockerCommand`.**

## 3. Add `formatter` field to `dockerExecutor`

Add a `formatter formatter.Formatter` field to the `dockerExecutor` struct. It must be the LAST new field added, keeping existing field order stable:

```go
type dockerExecutor struct {
    containerImage        string
    projectName           string
    model                 string
    netrcFile             string
    gitconfigFile         string
    env                   map[string]string
    extraMounts           []config.ExtraMount
    claudeDir             string
    commandRunner         commandRunner
    maxPromptDuration     time.Duration
    currentDateTimeGetter libtime.CurrentDateTimeGetter
    hideGitDir            bool
    formatter             formatter.Formatter  // ← add this field
}
```

## 4. Update `NewDockerExecutor` signature

Add `fmtr formatter.Formatter` as the last parameter and set the struct field:

```go
func NewDockerExecutor(
    containerImage string,
    projectName string,
    model string,
    netrcFile string,
    gitconfigFile string,
    env map[string]string,
    extraMounts []config.ExtraMount,
    claudeDir string,
    maxPromptDuration time.Duration,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    worktreeMode bool,
    fmtr formatter.Formatter,
) Executor {
    return &dockerExecutor{
        // ... all existing fields unchanged ...
        formatter: fmtr,
    }
}
```

### 4a. Update `NewDockerExecutorWithRunnerForTest` in `pkg/executor/export_test.go`

**CRITICAL:** `pkg/executor/export_test.go` exposes a test constructor `NewDockerExecutorWithRunnerForTest` that directly builds a `dockerExecutor` literal. Adding the `formatter` struct field WITHOUT updating this helper causes the formatter to be nil, and `e.formatter.ProcessStream(...)` panics on the first run.

Update the helper to accept and set the formatter. Preferred signature (append as last parameter, matching `NewDockerExecutor`):

```go
func NewDockerExecutorWithRunnerForTest(
    // ... existing parameters unchanged ...
    fmtr formatter.Formatter,
) Executor {
    return &dockerExecutor{
        // ... all existing fields unchanged ...
        formatter: fmtr,
    }
}
```

Then update every caller of `NewDockerExecutorWithRunnerForTest` in `pkg/executor/executor_test.go` (lines 1164 and 1324 in the current tree) to pass a `formatter.NewFormatter()` or a counterfeiter `FakeFormatter` stub. For tests that don't care about formatter behavior, pass a `FakeFormatter` whose `ProcessStream` simply drains the reader to `io.Discard` and returns nil — this avoids real formatting in tests that only care about command-runner behavior.

## 5. Add `prepareRawLogFile` helper

Add a new function adjacent to `prepareLogFile`:

```go
// prepareRawLogFile opens the raw JSONL log file for writing.
// The raw log path is the formatted log path with the extension replaced by ".jsonl".
// Returns a non-nil error naming the raw log path if the file cannot be opened.
func prepareRawLogFile(ctx context.Context, rawLogFile string) (*os.File, error) {
    logDir := filepath.Dir(rawLogFile)
    if err := os.MkdirAll(logDir, 0750); err != nil {
        return nil, errors.Wrapf(ctx, err, "create log directory for raw log %s", rawLogFile)
    }
    // #nosec G304 -- rawLogFile is derived from prompt filename, not user input
    f, err := os.OpenFile(rawLogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
    if err != nil {
        return nil, errors.Wrapf(ctx, err, "open raw log file %s", rawLogFile)
    }
    return f, nil
}
```

Add a `rawLogPath` helper to derive the JSONL path:

```go
// rawLogPath returns the raw JSONL log path corresponding to the given formatted log path.
// Example: "prompts/log/042.log" → "prompts/log/042.jsonl"
func rawLogPath(logFile string) string {
    ext := filepath.Ext(logFile)
    if ext == "" {
        return logFile + ".jsonl"
    }
    return strings.TrimSuffix(logFile, ext) + ".jsonl"
}
```

## 6. Update `Execute` to use the formatter pipeline

Replace the stdout setup section in `Execute` with the formatter pipeline. The change is **localized to the block between `prepareLogFile` and `cmd.Stdout =`**. Everything else in `Execute` — auth check, temp file, docker command build, `buildRunFuncs` — remains unchanged.

```go
func (e *dockerExecutor) Execute(
    ctx context.Context,
    promptContent string,
    logFile string,
    containerName string,
) error {
    // ... existing: projectRoot, home, prepareLogFile, createPromptTempFile, removeContainerIfExists,
    //     extractPromptBaseName, validateClaudeAuth, buildDockerCommand ... (all unchanged)

    // Open formatted log file (unchanged)
    logFileHandle, err := prepareLogFile(ctx, logFile)
    if err != nil {
        return errors.Wrap(ctx, err, "prepare log file")
    }
    defer logFileHandle.Close()

    // Open raw JSONL log file — fail fast before starting the container
    rawFile := rawLogPath(logFile)
    rawFileHandle, err := prepareRawLogFile(ctx, rawFile)
    if err != nil {
        return err // error already names the raw log path
    }
    defer rawFileHandle.Close()

    // ... (existing: createPromptTempFile, removeContainerIfExists, etc.) ...

    // Build docker command (unchanged)
    cmd := e.buildDockerCommand(...)

    // Set up streaming pipeline:
    //   cmd.Stdout → pipe → formatter → rawFileHandle (raw) + MultiWriter(os.Stdout, logFileHandle) (formatted)
    //   cmd.Stderr → MultiWriter(os.Stderr, logFileHandle)  (unchanged)
    pr, pw := io.Pipe()
    cmd.Stdout = pw
    cmd.Stderr = io.MultiWriter(os.Stderr, logFileHandle)

    formattedWriter := io.MultiWriter(os.Stdout, logFileHandle)

    // Run formatter in a goroutine; it exits when pr reaches EOF (pw.Close() called below).
    fmtDone := make(chan error, 1)
    go func() {
        fmtDone <- e.formatter.ProcessStream(ctx, pr, rawFileHandle, formattedWriter)
    }()

    // Wrap the commandRunner so it closes pw (and thus signals EOF to the formatter) on exit.
    origFuncs := e.buildRunFuncs(cmd, logFile, containerName)
    wrappedFuncs := wrapFirstFuncWithPipeClose(origFuncs, pw)

    runErr := run.CancelOnFirstFinish(ctx, wrappedFuncs...)

    // Wait for formatter to drain remaining buffered data after the pipe is closed.
    if fmtErr := <-fmtDone; fmtErr != nil {
        slog.Warn("formatter error", "error", fmtErr)
    }

    if runErr != nil {
        return errors.Wrap(ctx, runErr, "docker run failed")
    }
    return nil
}
```

Add the `wrapFirstFuncWithPipeClose` helper:

```go
// wrapFirstFuncWithPipeClose wraps the first run.Func in fns so that pw is closed
// when that function returns (regardless of error). The remaining functions are returned unchanged.
// The first function is always the commandRunner.Run wrapper from buildRunFuncsWithTimeout.
func wrapFirstFuncWithPipeClose(fns []run.Func, pw *io.PipeWriter) []run.Func {
    if len(fns) == 0 {
        return fns
    }
    wrapped := make([]run.Func, len(fns))
    copy(wrapped, fns)
    original := fns[0]
    wrapped[0] = func(ctx context.Context) error {
        defer pw.Close()
        return original(ctx)
    }
    return wrapped
}
```

**Note on ordering:** `prepareLogFile` and `createPromptTempFile` are called before `buildDockerCommand`. The raw file must be opened before `buildDockerCommand` is called too, so both file handles are ready before the docker command is built and run. Adjust the `Execute` body to match: open `logFileHandle` → open `rawFileHandle` → create temp file → … → build command → set up pipe → run.

## 7. Update `Reattach` with the same pipeline

Apply the identical pattern to `Reattach`. Full method body with error handling and defer closes:

```go
func (e *dockerExecutor) Reattach(
    ctx context.Context,
    logFile string,
    containerName string,
    maxPromptDuration time.Duration,
) error {
    // ... existing preamble (e.g. context checks) unchanged ...

    logFileHandle, err := prepareLogFile(ctx, logFile)
    if err != nil {
        return errors.Wrap(ctx, err, "prepare log file")
    }
    defer logFileHandle.Close()

    rawFile := rawLogPath(logFile)
    rawFileHandle, err := prepareRawLogFile(ctx, rawFile)
    if err != nil {
        return err // error already names the raw log path
    }
    defer rawFileHandle.Close()

    // Build docker logs command (unchanged from current Reattach):
    //   cmd := exec.CommandContext(ctx, "docker", "logs", "--follow", containerName)
    cmd := exec.CommandContext(ctx, "docker", "logs", "--follow", containerName)

    // Set up streaming pipeline.
    pr, pw := io.Pipe()
    cmd.Stdout = pw
    cmd.Stderr = io.MultiWriter(os.Stderr, logFileHandle)

    formattedWriter := io.MultiWriter(os.Stdout, logFileHandle)

    fmtDone := make(chan error, 1)
    go func() {
        fmtDone <- e.formatter.ProcessStream(ctx, pr, rawFileHandle, formattedWriter)
    }()

    origFuncs := e.buildRunFuncsWithTimeout(cmd, logFile, containerName, maxPromptDuration)
    wrappedFuncs := wrapFirstFuncWithPipeClose(origFuncs, pw)

    runErr := run.CancelOnFirstFinish(ctx, wrappedFuncs...)

    if fmtErr := <-fmtDone; fmtErr != nil {
        slog.Warn("formatter error on reattach", "error", fmtErr)
    }

    if runErr != nil {
        return errors.Wrap(ctx, runErr, "reattach failed")
    }
    return nil
}
```

**Note:** The `cmd` variable is the existing `docker logs --follow <container>` command currently constructed at the top of `Reattach` (around line 240). Preserve that construction exactly; only the stdout/stderr wiring changes.

### 7a. Pipe drain invariant (for both Execute and Reattach)

The formatter goroutine relies on `pw.Close()` to signal EOF. Because `wrapFirstFuncWithPipeClose` uses `defer pw.Close()` on the first run.Func (the commandRunner wrapper), the pipe closes on BOTH success and cancellation paths of the runner func — so `<-fmtDone` will not hang when `run.CancelOnFirstFinish` cancels the runner mid-stream. Document this with a code comment above `wrapFirstFuncWithPipeClose` so future maintainers don't break it.

## 8. Update `pkg/factory/factory.go`

In the `createDockerExecutor` helper function, add the `formatter.NewFormatter()` argument as the last parameter to `executor.NewDockerExecutor`:

```go
import "github.com/bborbe/dark-factory/pkg/formatter"

func createDockerExecutor(
    // ... existing parameters unchanged ...
    worktreeMode bool,
) executor.Executor {
    return executor.NewDockerExecutor(
        containerImage, projectName, model, netrcFile,
        gitconfigFile, env, extraMounts, claudeDir, maxPromptDuration,
        currentDateTimeGetter,
        worktreeMode,
        formatter.NewFormatter(),
    )
}
```

Also update the direct `executor.NewDockerExecutor(...)` call in `CreateSpecGenerator` (which does NOT use the `createDockerExecutor` helper) to pass `formatter.NewFormatter()` as the last argument.

## 9. Unit tests for the executor streaming pipeline

Add a `Describe("Execute streaming pipeline", ...)` block to the executor test file.

**Note:** Check how the existing tests mock `commandRunner` — typically via `export_test.go` and a fake `commandRunner`. Reuse that pattern.

### 9a. JSONL file is created alongside formatted log

Use the existing fake `commandRunner`. Configure it to write 3 stream-json lines to stdout:
```json
{"type":"system","subtype":"init","session_id":"test-123"}
{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}
{"type":"result","result":"success","duration_ms":100}
```
Also write a DARK-FACTORY-REPORT JSON block to stdout (the watcher requires it to find the report).

Assert:
- The formatted log file (`.log`) exists and is non-empty
- The JSONL log file (`.jsonl`, same prefix) exists and contains exactly the 3 raw lines
- Each line in the JSONL file is valid JSON
- The formatted log contains "hello" (formatted text) and "Session started"

**9b. Raw JSONL preserves non-JSON lines verbatim**

Configure commandRunner stdout to emit:
```
not-json-at-all
{"type":"result","result":"success","duration_ms":1}
```
Assert:
- JSONL file contains both lines verbatim
- Formatted log contains "not-json-at-all" (verbatim passthrough)
- `Execute` returns nil (no error from non-JSON line)

**9c. Raw log file open failure returns error before container starts**

**Do NOT use `chmod 0500`** — Docker containers (and many CI environments) run as root, where chmod-based read-only restrictions are bypassed. Instead, arrange the failure by passing a formatted log path whose parent cannot be created: use a path that would require creating a directory under a file (e.g. create a regular file at `$tmp/blocker`, then pass `$tmp/blocker/sub/log.log` as the formatted log path). `prepareRawLogFile` will fail at `os.MkdirAll` because a directory cannot be made under a regular file.

Assert:
- `Execute` returns a non-nil error
- The error message mentions the raw log path (derived via `rawLogPath`)
- The container is NOT started (verify via the fake `commandRunner`: `Run` was not called)

**9d. Reattach also produces JSONL file**

Same as 9a but call `Reattach` instead of `Execute`. Assert both `.log` and `.jsonl` are created.

**9e. YOLO_OUTPUT=json is in docker args**

Call `BuildDockerCommandForTest` (or the equivalent export-test helper) and assert the returned args contain BOTH `"-e"` immediately followed by `"YOLO_OUTPUT=json"` in the args slice — not just "somewhere in the merged string". This guards against placement regressions.

## 10. Add CHANGELOG entry

Add a bullet under `## Unreleased` in `CHANGELOG.md` describing the change:

```markdown
## Unreleased

- Executor writes two log files per prompt run: raw JSONL (container stdout verbatim) and human-readable formatted log. Container image bumped to the first claude-yolo version supporting `YOLO_OUTPUT=json` passthrough.
```

Match the existing changelog voice and formatting (check the most recent entry for style).

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- **FREEZE `pkg/formatter/`** — no changes permitted to the formatter package (it was delivered by prompt 1).
- **FREEZE `buildDockerCommand` body** except for the single `"-e", "YOLO_OUTPUT=json"` line added in requirement 2.
- The `Executor` interface signature (`Execute`, `Reattach`) must NOT change — the interface is unchanged; only the implementation changes.
- `watchForCompletionReport` reads the formatted `.log` file for the DARK-FACTORY-REPORT block — ensure the formatter writes the raw result/report lines through to the formatted log without swallowing them.
- Container stderr must still reach `os.Stderr` and the formatted log exactly as today.
- Wrap all non-nil errors with `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`.
- No new runtime dependencies — packages only from Go stdlib or what is already in `go.mod`. The repo is non-vendored.
- The formatter goroutine must be waited on (`<-fmtDone`) before `Execute`/`Reattach` return, so the log files are fully flushed before callers close them.
- Do not touch `go.mod` / `go.sum`.
- Existing tests must still pass.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional checks:
1. `grep -n "YOLO_OUTPUT" pkg/executor/executor.go` — exactly ONE occurrence inside `buildDockerCommand`.
2. `grep -n "v0.6.0" pkg/const.go` — must show `DefaultContainerImage` at `v0.6.0`.
3. `grep -n "rawLogPath\|prepareRawLogFile" pkg/executor/executor.go` — both helpers present.
4. `grep -n "formatter" pkg/executor/executor.go` — field, import, and goroutine present.
5. `grep -n "formatter.NewFormatter" pkg/factory/factory.go` — two occurrences (createDockerExecutor + CreateSpecGenerator).
6. `grep -A1 "## Unreleased" CHANGELOG.md` — shows the new entry describing dual-log behavior.
7. `grep -n "formatter" pkg/executor/export_test.go` — `NewDockerExecutorWithRunnerForTest` accepts a `formatter.Formatter` parameter.

Operator smoke test (after running a real prompt through dark-factory):
```bash
ls prompts/log/           # shows NNN.log and NNN.jsonl
head prompts/log/NNN.jsonl | jq .   # raw messages parse as JSON
head prompts/log/NNN.log             # formatted, human-readable
```
</verification>
