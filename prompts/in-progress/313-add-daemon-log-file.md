---
status: approved
created: "2026-04-17T10:32:25Z"
queued: "2026-04-17T10:33:55Z"
---

<summary>
- Dark-factory daemon writes structured log output to .dark-factory.log alongside .dark-factory.lock
- Log file is truncated on each daemon start so it does not grow unbounded
- Daemon logs to both stderr (existing behavior) and the log file simultaneously
- Log file uses the same slog format as stderr output
- The dark-factory status command shows the log file path in its output
</summary>

<objective>
Add a daemon log file at `.dark-factory.log` so operators can inspect daemon output after the fact without needing to capture stderr at launch time. The file is truncated on each daemon restart to avoid unbounded growth.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/runner/runner.go` — find the `Run` method where the daemon main loop starts and where `startupLogger` is called. Insert log file setup between `slog.Info("acquired lock", ...)` and `if r.startupLogger != nil`.
Read `pkg/status/formatter.go` — find the `Format` method that builds the status output. Look for the `Last log:` line (~line 85) and `formatWarnings` (~line 95).
Read `pkg/status/status.go` — find the `Status` struct to add a new field.
Read `cmd/dark-factory/main.go` — find the `slog.SetDefault` call (~line 65) to understand the current handler options (log level).
The daemon currently logs via `slog` to stderr only. The log file should receive the same structured output.
</context>

<requirements>
1. In the `Run` method of `pkg/runner/runner.go`, after acquiring the lock and before the startup logger call, open `.dark-factory.log` for writing with `os.Create` (truncates on each start). Defer closing the file handle.

2. Configure `slog` to write to both stderr and the log file using `io.MultiWriter(os.Stderr, logFile)`. Create a new `slog.TextHandler` with the same handler options as the existing default (preserve log level from `main.go`). Call `slog.SetDefault` with the new handler. This must happen early in `Run`, before any log output after lock acquisition.

3. Add a `DaemonLogFile string` field to the `Status` struct in `pkg/status/status.go`. Set it to `.dark-factory.log` during status collection. In `pkg/status/formatter.go`, add a display line `Daemon log: .dark-factory.log` near the `Last log:` line in the `Format` method.

4. Verify `.dark-factory.log` is in `.gitignore` (it already is).

5. If the log file cannot be created (permission error, read-only filesystem), log a warning to stderr via `slog.Warn` and continue without the log file — do not abort the daemon.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Do NOT change the log format — use the same slog text handler
- Do NOT add log rotation or size limits — truncate-on-start is sufficient
- Do NOT add a config option for the log file path — it is always `.dark-factory.log`
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
