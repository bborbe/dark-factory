---
status: completed
summary: defaultCommandRunner.Run() now uses cmd.Start()+cmd.Wait() with a goroutine that sends SIGINT then SIGKILL on context cancellation; three new Ginkgo tests cover normal exit, non-zero exit, and context-cancellation termination
container: dark-factory-259-fix-command-runner-ctx-cancellation
dark-factory-version: v0.103.0
created: "2026-04-06T12:00:00Z"
queued: "2026-04-06T13:55:33Z"
started: "2026-04-06T13:58:05Z"
completed: "2026-04-06T14:09:29Z"
---

<summary>
- Docker container processes are killed when the parent context is cancelled
- The command runner now starts the process and watches for context cancellation in a separate goroutine
- When context is cancelled while a command is running, the process receives SIGINT then SIGKILL after 10 seconds
- Callers no longer rely solely on docker stop/kill for cleanup — context cancellation propagates to the process
- Existing tests continue to pass, new tests verify cancellation behavior
</summary>

<objective>
Fix `defaultCommandRunner.Run()` in `pkg/executor/command.go` to respect context cancellation. Currently it calls `cmd.Run()` which blocks until the process exits and ignores `ctx`. This means when `timeoutKiller` or `watchForCompletionReport` cancel the context (via `run.CancelOnFirstFinish`), the docker process doesn't receive any signal — the system relies entirely on a separate `docker stop` command. Making the runner context-aware adds defense-in-depth.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these files before making changes:
- `pkg/executor/command.go` — the `commandRunner` interface and `defaultCommandRunner` struct (entire file, ~23 lines)
- `pkg/executor/executor.go` — see how `commandRunner` is used in `buildRunFuncs` (~line 167-194), `timeoutKiller` (~line 220-250), `watchForCompletionReport` (~line 255-296), `StopAndRemoveContainer` (~line 483-491), `removeContainerIfExists` (~line 493-502)
- `pkg/executor/executor_internal_test.go` — see `fakeCommandRunner` (~line 23-50) and `multiFailRunner` (~line 52-73) test doubles; these do NOT need to change (they don't block, so ctx cancellation is not relevant for fakes)

Follow error wrapping conventions: use `errors.Wrap(ctx, err, "message")` from `github.com/bborbe/errors` — never bare `return err` in production code.
</context>

<requirements>
1. **Replace `cmd.Run()` with `cmd.Start()` + context-aware wait** in `defaultCommandRunner.Run()` in `pkg/executor/command.go`:
   - Call `cmd.Start()`. If it returns an error, wrap and return it.
   - Launch a goroutine that waits on `<-ctx.Done()`. When fired:
     - Call `cmd.Process.Signal(os.Interrupt)` (sends SIGINT). If signal fails (process already exited), ignore.
     - Start a 10-second timer. If `cmd.Process` is still running after 10s, call `cmd.Process.Kill()`.
   - Call `cmd.Wait()` in the main goroutine. Wrap its error with `errors.Wrap` before returning.
   - Ensure the signal goroutine doesn't leak: use a channel or context derived from the function scope that closes when `cmd.Wait()` returns.

2. **Important edge case**: When the context is already cancelled before `cmd.Start()`, the function should still attempt to start and immediately signal. Do NOT add a `ctx.Err()` pre-check — let `cmd.Start()` run so the caller gets consistent behavior.

3. **Add `"os"` to imports** in `command.go` (needed for `os.Interrupt`).

4. **Add `"time"` to imports** in `command.go` (needed for the 10s kill timer).

5. **Do NOT change the `commandRunner` interface signature** — `Run(ctx context.Context, cmd *exec.Cmd) error` stays the same.

6. **Do NOT change `fakeCommandRunner` or `multiFailRunner`** in the test file — they are non-blocking test doubles that don't need context awareness.

7. **Add Ginkgo/Gomega unit tests** in `pkg/executor/executor_internal_test.go` for `defaultCommandRunner` inside a new `Describe("defaultCommandRunner")` block:
   - Test: context cancelled while command is running causes process termination. Use `exec.Command("sleep", "60")` as the long-running process. Cancel ctx after 100ms. Use `Eventually(func() error { return runErr }).WithTimeout(2 * time.Second).Should(HaveOccurred())` to verify it returns promptly.
   - Test: command that exits normally returns nil error. Use `exec.Command("true")`. `Expect(err).To(BeNil())`.
   - Test: command that exits with non-zero returns error. Use `exec.Command("false")`. `Expect(err).To(HaveOccurred())`.

8. **Update CHANGELOG.md**: Add under `## Unreleased` (create section if needed, above `## v0.104.0`):
   - `fix: defaultCommandRunner now respects context cancellation — sends SIGINT/SIGKILL to child process when ctx is cancelled`
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Do NOT change the commandRunner interface
- Do NOT change fakeCommandRunner or multiFailRunner test doubles
- Follow project conventions from CLAUDE.md (doc comments on exports, structured logging, error wrapping)
</constraints>

<verification>
Run `make precommit` — must pass.
Run `go test ./pkg/executor/ -run "defaultCommandRunner" -v -count=1` — new tests must pass.
</verification>
