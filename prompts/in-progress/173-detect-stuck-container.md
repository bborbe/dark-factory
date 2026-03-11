---
status: approved
created: "2026-03-11T20:47:23Z"
queued: "2026-03-11T20:47:23Z"
---

<summary>
- Containers that completed work but won't exit are automatically stopped
- A goroutine scans the log file for the DARK-FACTORY-REPORT marker during execution
- After the report is detected, a grace period allows the container to exit naturally
- If the container is still running after the grace period, it is stopped via docker stop
- Normal container exits are unaffected — the watcher is a safety net only
</summary>

<objective>
Detect and stop Docker containers that have finished their work (completion report written) but fail to exit. This happens when the Claude agent uses blocking commands like `tail -f` that prevent the process from terminating. The container has already produced all its output — it just needs to be stopped.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/executor/executor.go` — the `Execute` method blocks on `e.commandRunner.Run(ctx, cmd)` at line ~140, waiting for the container to exit. The log file is written via `io.MultiWriter` at line ~136.
Read `pkg/report/suffix.go` — the `MarkerStart` and `MarkerEnd` constants define the completion report delimiters.
Read `pkg/report/suffix.go` to find the exact `MarkerEnd` string — use raw `strings.Contains` on log content, not `ParseFromLog` (avoids partial-write JSON parse failures).
</context>

<requirements>
1. In `pkg/executor/executor.go`, add a function `watchForCompletionReport(ctx context.Context, logFile string, containerName string, gracePeriod time.Duration) error` that:
   - Polls the log file every 10 seconds (using `os.ReadFile`, not `tail -f`)
   - Checks if the content contains `report.MarkerEnd` (the closing marker)
   - Once the marker is found, waits for `gracePeriod` (default: 2 minutes), respecting ctx cancellation during the wait
   - After the grace period, runs `docker stop <containerName>` to terminate the stuck container
   - Logs at Info level: `"stopping stuck container: completion report found but container still running"`
   - Returns nil when ctx is cancelled (normal exit — container finished before grace period)
   - Returns nil after stopping the container
2. In the `Execute` method, replace the direct `commandRunner.Run` call with `run.CancelOnFirstFinish` from `github.com/bborbe/run` (already a dependency at v1.9.4). Run both the container and the watcher in parallel — whichever finishes first cancels the other:
   ```go
   err = run.CancelOnFirstFinish(ctx,
       func(ctx context.Context) error {
           return e.commandRunner.Run(ctx, cmd)
       },
       func(ctx context.Context) error {
           return watchForCompletionReport(ctx, logFile, containerName, 2*time.Minute)
       },
   )
   ```
   This way: if the container exits normally, ctx cancels the watcher. If the watcher detects a stuck container and stops it, the container run returns and both complete.
   **Error semantics**: `CancelOnFirstFinish` returns the first non-nil error. After `docker stop`, `commandRunner.Run` will return an error (non-zero exit or killed). This is expected — the caller already handles non-zero exits. Do not suppress or wrap this error differently.
3. The grace period should be a constant, not configurable (keep it simple)
4. Import `github.com/bborbe/run` and `pkg/report` for the `MarkerEnd` constant
5. Add tests in `pkg/executor/executor_internal_test.go`:
   - Test: log file contains report marker → after grace period, docker stop is called
   - Test: context cancelled before grace period expires → no docker stop called
   - Test: log file never contains marker → no docker stop called
   Use a short grace period (100ms) in tests for speed.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- The watcher must not interfere with normal container exits — it is a safety net only
- Use `os.ReadFile` to poll, NOT `tail -f` or file watchers (keep it simple and reliable)
- The `docker stop` command has a default 10-second timeout before SIGKILL — this is fine
- Do not add a configurable timeout — hardcode 2 minutes grace period
</constraints>

<verification>
Run `make precommit` -- must pass.
</verification>
