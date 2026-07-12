---
status: approved
spec: [104-local-execution-backend]
created: "2026-07-12T19:00:00Z"
queued: "2026-07-12T19:08:22Z"
---

<summary>

- Adds a second execution backend that runs the LLM step (`claude`) as a plain local subprocess in the current working directory instead of launching a Docker container.
- The new executor reuses the exact same prompt-temp-file handoff and the same stdout formatter/raw-log pipeline the docker executor uses — only the process being launched changes (`claude ...` instead of `docker run ... claude`).
- If `claude` is not found on `PATH`, the local backend fails immediately with a clear, actionable error naming the missing binary — it never silently falls back to Docker.
- Because a local subprocess dies with the dark-factory process, the local backend cannot re-attach after a restart: its reattach path returns a typed "not supported" signal so the caller knows to re-run the prompt (per-prompt commits make re-running safe).
- Stopping a runaway local execution terminates the whole child process group (SIGTERM, then SIGKILL after a grace period) — the local analog of docker stop/kill.
- This prompt adds the executor and its unit tests only. It does NOT wire the backend into the factory (prompt 3) — so nothing changes at runtime yet.

</summary>

<objective>
Add `pkg/executor/local_subprocess.go` implementing the existing neutral `Executor` interface (plus `ExecutionChecker` and `ExecutionStopper` local variants) by running `claude` as a local subprocess in cwd, reusing the docker executor's temp-file + formatter/raw-log pipeline. Fail closed when `claude` is absent from `PATH`; return a typed `ErrReattachUnsupported` from `Reattach`; kill the child process group on stop. Unit tests cover all four behaviors.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read the parent spec end-to-end:
- `/workspace/specs/in-progress/104-local-execution-backend.md` — Desired Behavior 3, 5; Constraints; Security section; Failure Modes rows "claude not on PATH", "pod restarts mid-local-execution", "child subprocess hangs"; Acceptance Criteria 4, 7, 10.

Read these coding-plugin docs (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — public interface + private struct + `New*` constructor; error wrapping with `errors.Wrapf(ctx, err, ...)`; sentinel errors via `stderrors` alias.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — sentinel-error idiom (`var ErrX = stderrors.New(...)`), never `fmt.Errorf`, never `context.Background()` in pkg/.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega suite, counterfeiter mocks, external `_test` packages, coverage ≥80%, error-path testing.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-concurrency-patterns.md` — `run.CancelOnFirstFinish`, caller-owned channels.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-security-linting.md` — `#nosec G204` with a reason on `exec.Command` built from internal (non-user) input.
- `/workspace/docs/execution-backends.md` — the "Adding a Backend" blueprint: this file is the new-file half of the ≤3-file plan.

Read these files END-TO-END before implementing (the local executor is a sibling of the docker one and MUST reuse its helpers):
- `/workspace/pkg/executor/executor.go` — the whole file. CRITICAL reuse points, all in `package executor` so the new file can call them directly:
  - The `Executor` interface (line ~31): `Execute(ctx, promptContent, logFile, executionID string) error`; `Reattach(ctx, logFile, executionID string, maxPromptDuration time.Duration) error`; `StopAndRemoveContainer(ctx, executionID string)`. Implement these three exactly.
  - `prepareLogFile(ctx, logFile)`, `prepareRawLogFile(ctx, rawLogPath(logFile))`, `rawLogPath(logFile)` — reuse verbatim for the same two-log-file contract.
  - `createPromptTempFile(ctx, promptContent)` returns `(path string, cleanup func(), error)` — reuse verbatim for the prompt handoff. NOTE: it chmods the temp dir/file to 0755/0644 for the container user; that is harmless locally, so reuse as-is.
  - `runWithFormatterPipeline(...)` is a METHOD on `*dockerExecutor` (line ~136). Do NOT call it from the local struct. Instead replicate its body as a small package-level helper OR a method on the local struct — see requirement 3 for the exact shape. It wires `cmd.Stdout` through `io.Pipe` into `e.formatter.ProcessStream(...)`, runs the funcs via `run.CancelOnFirstFinish` wrapped by `wrapFirstFuncWithPipeClose`, and drains `fmtDone`.
  - `wrapFirstFuncWithPipeClose(fns []run.Func, pw *io.PipeWriter)` — package-level, reuse verbatim.
  - `formatter.Formatter` (from `github.com/bborbe/dark-factory/pkg/formatter`) has `ProcessStream(ctx, r io.Reader, rawWriter io.Writer, formattedWriter io.Writer) error` — confirm the exact signature by reading its usage at executor.go line ~149.
  - `commandRunner` interface (in `/workspace/pkg/executor/command.go`) with `defaultCommandRunner` — reuse `&defaultCommandRunner{}` for the local run so context-cancellation SIGINT→SIGKILL escalation is inherited.
- `/workspace/pkg/executor/command.go` — the whole file: `commandRunner.Run(ctx, cmd)` and `defaultCommandRunner` (already handles ctx-cancel → `os.Interrupt` → 10s → `Kill`).
- `/workspace/pkg/executor/checker.go` — `ExecutionChecker` interface: `IsRunning(ctx, executionID) (bool, error)` and `WaitUntilRunning(ctx, executionID, timeout) error`; the docker impl and its `NewDockerExecutionChecker` constructor. Model the local checker's constructor name `NewLocalSubprocessExecutionChecker`.
- `/workspace/pkg/executor/stopper.go` — `ExecutionStopper` interface: `StopContainer(ctx, executionID) error`; `NewDockerExecutionStopper` constructor. Model `NewLocalSubprocessExecutionStopper`.
- `/workspace/pkg/cmd/healthcheck/probes.go` (lines ~222–245) — the `claudeProbe.Run` comment documents the EXACT claude argv the claude-yolo image's `entrypoint.sh` runs in production: `claude --dangerously-skip-permissions --model "$ANTHROPIC_MODEL" --output-format stream-json --verbose`, reading the prompt from the mounted file, with `YOLO_OUTPUT=json` selecting stream-json JSONL that the formatter parses. The local executor MUST reproduce this same argv so the formatter parses identical output. See requirement 2 for the exact argv.
- `/workspace/pkg/claudeargv/envoverlay.go` — documents that `entrypoint.sh` maps `ANTHROPIC_MODEL`→`--model`, `YOLO_OUTPUT=json`→`--output-format stream-json --verbose`, and reads the prompt file. There is NO local claude-argv builder today; build the argv directly in the local executor per requirement 2 (do not invent a new claudeargv API).

Verified facts (do not re-derive):
- The neutral interface method is named `StopAndRemoveContainer` and the `ExecutionStopper` method is `StopContainer` — these names are fixed by the existing interfaces; keep them (the local impl satisfies the SAME interfaces).
- `run.CancelOnFirstFinish` and `run.Func` come from `github.com/bborbe/run` (already imported in executor.go).
- `libtime.CurrentDateTimeGetter` from `github.com/bborbe/time` is the time-injection type used by the docker executor's timeout logic.
- The prompt content is written to a temp file by `createPromptTempFile`; for local execution that file is on the real filesystem and readable by the current process — no mount needed.

Open question (surface in `## Improvements`, category PROMPT — do NOT block on it): the docker executor pipes the prompt via a mounted file that `entrypoint.sh` reads. Locally there is no entrypoint.sh, so the executor must pass the prompt to `claude` directly. Use `claude ... -p "$(contents)"` is WRONG for large prompts (argv length limit) — instead pass the prompt file on stdin: read the temp file and set `cmd.Stdin` to the file handle, invoking `claude --dangerously-skip-permissions --model <model> --output-format stream-json --verbose --print` reading the prompt from stdin. If reading the actual claude-yolo `entrypoint.sh` (referenced in probes.go) reveals a different invocation (e.g. it passes the file path as a positional arg), MATCH that instead and note the correction in `## Improvements`. Default to the stdin approach specified in requirement 2 if the entrypoint is not available in the container.

</context>

<requirements>

## 1. Create `pkg/executor/local_subprocess.go` with the local executor

Create `/workspace/pkg/executor/local_subprocess.go`, `package executor`, with the standard BSD license header (copy from `executor.go`). Define:

```go
// localSubprocessExecutor implements Executor by running claude as a local
// subprocess in the current working directory (already the checked-out repo).
// No docker run, no bind mounts. Intended ONLY for already-isolated callers
// (see docs/execution-backends.md) — it runs claude with the full credentials
// and filesystem of the dark-factory process.
type localSubprocessExecutor struct {
	model                 string
	maxPromptDuration     time.Duration
	currentDateTimeGetter libtime.CurrentDateTimeGetter
	formatter             formatter.Formatter
	commandRunner         commandRunner
}

// NewLocalSubprocessExecutor creates an Executor that runs claude locally.
func NewLocalSubprocessExecutor(
	model string,
	maxPromptDuration time.Duration,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	fmtr formatter.Formatter,
) Executor {
	return &localSubprocessExecutor{
		model:                 model,
		maxPromptDuration:     maxPromptDuration,
		currentDateTimeGetter: currentDateTimeGetter,
		formatter:             fmtr,
		commandRunner:         &defaultCommandRunner{},
	}
}
```

Note the constructor signature intentionally OMITS `launchpolicy.Policy` (the docker `NewDockerExecutor` takes it, but local needs no image/mounts/hide-git). Prompt 3 will construct this at the factory switch.

## 2. Implement `Execute` — subprocess + fail-closed on missing claude

Add a sentinel and the claude-lookup at the top of `Execute`:

```go
// ErrClaudeNotFound signals that the `claude` binary is not on PATH.
// backend: local requires claude in the environment; the local executor
// NEVER falls back to docker.
var ErrClaudeNotFound = stderrors.New("claude not found on PATH")
```
(import `stderrors "errors"` per the sentinel idiom in checker.go.)

`Execute(ctx, promptContent, logFile, executionID string) error` MUST:
1. Resolve the binary: `claudePath, err := exec.LookPath("claude")`. If err != nil, return a wrapped, actionable error whose message contains the literal `claude not found on PATH`: `return errors.Wrapf(ctx, ErrClaudeNotFound, "backend: local requires claude in the environment; %v", err)`. This is the fail-closed path (Failure Mode row 2, AC 6/10-`TestLocalMissingClaudeFailsClosed`). Do this BEFORE creating any log/temp file so a missing claude fails fast.
2. Open the two log files: `prepareLogFile(ctx, logFile)` and `prepareRawLogFile(ctx, rawLogPath(logFile))` (reuse the executor.go helpers). `defer` closing both.
3. Create the prompt temp file: `promptFilePath, cleanup, err := createPromptTempFile(ctx, promptContent)`; `defer cleanup()`.
4. Build the claude command (argv reproduces the claude-yolo entrypoint's production invocation documented in probes.go):
   ```go
   // #nosec G204 -- claudePath is resolved from PATH; model is validated config; args are static flags, not user input
   cmd := exec.CommandContext(ctx, claudePath,
       "--dangerously-skip-permissions",
       "--model", e.model,
       "--output-format", "stream-json",
       "--verbose",
       "--print",
   )
   promptFile, err := os.Open(promptFilePath) // #nosec G304 -- path from createPromptTempFile, not user input
   if err != nil { return errors.Wrap(ctx, err, "open prompt file for stdin") }
   defer promptFile.Close()
   cmd.Stdin = promptFile
   cmd.Dir = ""  // inherit cwd (already the checked-out repo)
   ```
   If `e.model` is empty, OMIT the `--model`/value pair (claude falls back to its default) — build the args slice conditionally.
5. Run through the formatter/raw-log pipeline (requirement 3). On failure wrap: `return errors.Wrap(ctx, runErr, "local claude run failed")`.
6. Emit `log.From(ctx).Debug("local claude execution prepared", "model", e.model, "prompt_file", promptFilePath, "execution_id", executionID)`.

Do NOT invoke `docker` anywhere in this file. Do NOT call `validateClaudeAuth` (that checks the container config dir; local execution uses the ambient `CLAUDE_CONFIG_DIR`/auth of the process — auth failures surface from claude's own exit).

## 3. Formatter/raw-log pipeline + timeout for the local run

Add a method `func (e *localSubprocessExecutor) runWithFormatterPipeline(ctx, cmd *exec.Cmd, rawWriter, logWriter io.Writer, runFuncs []run.Func, fmtErrMsg string) error` that replicates the body of `dockerExecutor.runWithFormatterPipeline` (executor.go line ~136): `io.Pipe`, `cmd.Stdout = pw`, `cmd.Stderr = io.MultiWriter(os.Stderr, logWriter)`, goroutine calling `e.formatter.ProcessStream(ctx, pr, rawWriter, io.MultiWriter(os.Stdout, logWriter))`, then `run.CancelOnFirstFinish(ctx, wrapFirstFuncWithPipeClose(runFuncs, pw)...)`, drain `<-fmtDone`, return runErr. (This duplication is intentional and small; do NOT refactor the docker method to share — keep the docker path byte-for-byte unchanged per the spec Constraint.)

Build the run funcs: a slice with the single command func `func(ctx context.Context) error { return e.commandRunner.Run(ctx, cmd) }`. If `e.maxPromptDuration > 0`, append a timeout func that, on deadline, terminates the child process group (call the same kill logic as `StopAndRemoveContainer` — see requirement 5). Reuse `waitUntilDeadline(ctx, e.currentDateTimeGetter, deadline, 30*time.Second)` from executor.go for the deadline wait. On timeout, return `errors.Errorf(ctx, "prompt timed out after %s", e.maxPromptDuration)`. (This satisfies Failure Mode row "child subprocess hangs".)

## 4. Implement `Reattach` — return the not-supported sentinel

Add:
```go
// ErrReattachUnsupported signals that the local backend cannot reattach to a
// prior execution: a local subprocess dies with the dark-factory process, so
// there is nothing to reattach to. The caller must recover by re-running the
// prompt (safe because execution commits per prompt).
var ErrReattachUnsupported = stderrors.New("reattach unsupported for local backend")
```
`Reattach(ctx, logFile, executionID string, maxPromptDuration time.Duration) error` MUST return `errors.Wrap(ctx, ErrReattachUnsupported, "local backend does not support reattach")`. It MUST NOT silently succeed and MUST NOT spawn anything.

## 5. Implement `StopAndRemoveContainer` — kill the child process group

The local executor must be able to terminate a running child (the timeout path and cancellation path both call this). Track the running child so stop can reach it:
- Add a mutex-guarded `runningCmd *exec.Cmd` (or its `*os.Process`) field on the struct, set when `Execute` starts the process and cleared on exit. Alternatively (simpler and preferred): give the timeout func in requirement 3 a direct closure over the `cmd`, and implement `StopAndRemoveContainer` as a best-effort process-group signal. Pick ONE approach and implement it fully — the required observable behavior is: on stop, the child (and its group) receives SIGTERM, then SIGKILL after a grace period.
- To reach the process GROUP, set `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}` when building the command in `Execute`, and signal the negative PID: `syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)`, wait a grace period (e.g. 10s or until exit), then `syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)`. Guard against `cmd.Process == nil`. Log via `log.From(ctx)`; errors are best-effort (the interface method returns nothing).
- `StopAndRemoveContainer(ctx, executionID string)` sends SIGTERM→grace→SIGKILL to the tracked child's process group. If no child is tracked (nothing running), it is a no-op.

This is Unix-specific; the file targets linux (the agent pod). Guard the build if needed with `//go:build linux` at the top of `local_subprocess.go` OR use `syscall` directly (which compiles on linux/darwin). Prefer plain `syscall` usage so the file compiles on the CI/dev macOS host too; if `syscall.SysProcAttr{Setpgid: true}` is not portable in a way that breaks `make precommit` on the build host, add `//go:build unix` and note it in `## Improvements`.

## 6. Local `ExecutionChecker` and `ExecutionStopper`

Add in the same file:
- `localSubprocessExecutionChecker` implementing `ExecutionChecker` with `NewLocalSubprocessExecutionChecker(currentDateTimeGetter libtime.CurrentDateTimeGetter) ExecutionChecker`. Semantics: a local subprocess never survives a dark-factory restart and is not inspectable across restarts. `IsRunning(ctx, executionID) (bool, error)` returns `(false, nil)` always. `WaitUntilRunning(ctx, executionID, timeout) error` returns `nil` immediately (the local `Execute` blocks in-process, so "wait until running" is trivially satisfied). Add a doc comment explaining WHY each returns as it does.
- `localSubprocessExecutionStopper` implementing `ExecutionStopper` with `NewLocalSubprocessExecutionStopper() ExecutionStopper`. `StopContainer(ctx, executionID) error` — because the local backend's cancellation is driven through the executor's `StopAndRemoveContainer` (which holds the child handle), a standalone stopper keyed only by `executionID` cannot reach the child. Return `nil` (no-op) and document that local cancellation flows through `StopAndRemoveContainer` / context cancellation instead. Do NOT return an error (the docker stopper's contract is best-effort/non-fatal).

## 7. Unit tests (AC 10)

Create `/workspace/pkg/executor/local_subprocess_test.go`, `package executor_test` (external test package). Use the Ginkgo suite already present (`executor_suite_test.go`). Cover:

- **`TestLocalExecute` equivalent**: run `Execute` with a stub `claude` on PATH. Approach: create a temp dir containing an executable script named `claude` that captures its own `os.Args` (write them to a sentinel file — e.g. `printf '%s\n' "$@" > "$ARGS_FILE"`) AND emits a minimal stream-json line to stdout and exits 0; prepend that dir to `PATH` for the test (restore after). Assert: `Execute` returns nil; the formatted log file and the `.jsonl` raw log file exist and are non-empty; assert NO `docker` binary was invoked (the test's fake PATH has no `docker`, or assert the stub-claude ran by having it write a sentinel file). CRITICAL: read the captured-args sentinel and assert the invocation includes the production flags sourced from `pkg/cmd/healthcheck/probes.go` / `pkg/claudeargv` — `--dangerously-skip-permissions`, `--model` (with the configured model value), `--output-format stream-json`, and `--verbose` — not merely that stream-json was emitted. This proves the local executor reproduces the claude-yolo entrypoint's production argv, so the formatter parses identical output.
- **`TestLocalMissingClaudeFailsClosed`**: set `PATH` to an empty temp dir (no `claude`); assert `Execute` returns an error and `err.Error()` contains `claude not found on PATH`; assert `errors.Is(err, executor.ErrClaudeNotFound)` (export the sentinel — it is package-level, so the external test references it via `executor.ErrClaudeNotFound`; if it is unexported, add it to `export_test.go`). Assert NO log/temp files were created for this failure path if feasible (fail-fast).
- **`TestReattachUnsupported`**: `Reattach(...)` returns an error with `errors.Is(err, executor.ErrReattachUnsupported)` true.
- **`TestLocalStopKillsProcessGroup`**: start `Execute` with a stub `claude` that sleeps (blocks), in a goroutine; once the child is running, call `StopAndRemoveContainer` and assert the child is terminated within the grace window (e.g. `Execute` returns within a few seconds, or the sleep script did not run to completion — write a sentinel on natural exit and assert it is absent). Keep the test deterministic and short (< 15s). If a full process-group test is flaky on the build host, at minimum assert that `StopAndRemoveContainer` on a running child causes `Execute` to return before the stub's natural sleep duration.
- Cover the checker/stopper: `IsRunning` returns `(false, nil)`; `WaitUntilRunning` returns nil; `StopContainer` returns nil.

Coverage for the new file MUST be ≥80% for the changed/added code paths.

## 8. CHANGELOG

Append ONE bullet to `## Unreleased` in `/workspace/CHANGELOG.md`:
```
- feat: add localSubprocessExecutor (pkg/executor/local_subprocess.go) running claude as a local subprocess in cwd; fails closed when claude is absent from PATH, Reattach returns ErrReattachUnsupported, stop kills the child process group (spec 104 prompt 2)
```

## Improvements

Record these in the completion report's `## Improvements` section (category PROMPT — do NOT block on them):

- **Cross-reference:** the `ErrReattachUnsupported` sentinel this prompt returns from `Reattach` is CONSUMED by prompt 3's `pkg/promptresumer` re-queue branch (`errors.Is(reattachErr, executor.ErrReattachUnsupported)` → `MarkApproved` → re-run). Prompt 2 ONLY returns the sentinel; it does not implement the restart-recovery itself. The two prompts are coupled through this sentinel.
- **Open question (`--print`/stdin):** keep the open-question note from `<context>` here — locally there is no `entrypoint.sh`, so the executor passes the prompt on stdin with `--print`. If reading the actual claude-yolo `entrypoint.sh` reveals a different invocation (e.g. the prompt file as a positional arg), MATCH that instead and note the correction here.

</requirements>

<constraints>

- Reuse the existing neutral `Executor`/`ExecutionChecker`/`ExecutionStopper` interfaces — introduce NO new interface (spec Constraint). The local structs satisfy the SAME interfaces the docker structs do.
- Do NOT modify `dockerExecutor` or any existing docker code path — the default (`backend: docker`) behavior must stay byte-for-byte identical (spec Constraint). Reuse shared helpers by calling them, not by refactoring them.
- The local executor MUST NEVER invoke `docker` and MUST NEVER fall back to docker on a missing `claude` — it fails closed (Desired Behavior 5, Security section).
- No docker daemon may be required for the local path to run.
- Wrap all errors with `errors.Wrapf(ctx, err, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf`, never `context.Background()` in pkg/. Sentinels use `stderrors "errors"`.
- `#nosec G204`/`G304` annotations MUST carry a reason (project rule); the claude path is PATH-resolved and args are static — say so.
- This prompt does NOT wire the executor into the factory — prompt 3 does. Nothing changes at runtime after this prompt alone.
- The `hotpath-execution-naming-check` gate must still pass. `pkg/executor` is a DOCKER-flavored package where container vocabulary is allowed, so the interface method name `StopAndRemoveContainer` stays — do NOT rename it and do NOT leak container tokens into neutral packages (this prompt touches only `pkg/executor`).
- `make precommit` passes; counterfeiter mocks regenerate cleanly (`go generate ./...`).
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.

</constraints>

<verification>

```bash
cd /workspace

# build compiles (no factory wiring yet — executor package must compile standalone)
go build -mod=mod ./pkg/executor/...
# expected: exit 0

# unit tests for the local executor
go test -mod=mod ./pkg/executor/...
# expected: PASS (TestLocalExecute, TestLocalMissingClaudeFailsClosed, TestReattachUnsupported, TestLocalStopKillsProcessGroup)

# coverage on the executor package
go test -coverprofile=/tmp/cover.out -mod=mod ./pkg/executor/... && go tool cover -func=/tmp/cover.out | grep local_subprocess
# expected: added code paths ≥80%

# no docker invocation in the local file
grep -n '"docker"' pkg/executor/local_subprocess.go; echo "grep_exit=$?"
# expected: grep_exit=1 (no matches — local backend never calls docker)

# fail-closed error string present
grep -n 'claude not found on PATH' pkg/executor/local_subprocess.go
# expected: >= 1 line

# naming gate still green
make hotpath-execution-naming-check; echo "exit=$?"
# expected: exit=0

# changelog entry
grep -n 'spec 104 prompt 2' CHANGELOG.md
# expected: >= 1 line

make precommit
# expected: exit 0
```

</verification>
