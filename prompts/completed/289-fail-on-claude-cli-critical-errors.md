---
status: completed
summary: Added ScanForCriticalFailures to detect claude CLI auth/API errors in container logs and refactored validateCompletionReport to return (*CompletionReport, error), preventing auth-failed prompts from being silently marked completed
container: dark-factory-289-fail-on-claude-cli-critical-errors
dark-factory-version: v0.110.1
created: "2026-04-15T21:00:00Z"
queued: "2026-04-15T19:51:07Z"
started: "2026-04-16T06:03:50Z"
completed: "2026-04-16T06:14:27Z"
---

<summary>
- Dark-factory stops marking prompts as completed when the claude CLI itself failed (auth error, API error)
- Detection scans the container log for known critical-failure patterns (e.g. `Failed to authenticate`, `API Error: 401/403/429/5xx`, `"type":"authentication_error"`)
- A prompt with no completion report AND a critical failure is marked failed, not completed — subsequent prompts block as intended by "stop on failure"
- A prompt with no completion report AND no detected failure still succeeds silently (backwards compatible)
- `validateCompletionReport` is refactored to return `(*CompletionReport, error)` instead of `(string, error)` — no more ambiguous `("", nil)` that conflates "success with empty summary", "no report found", and "parse error"
- Caller reads `Summary` from the returned report object
- Full unit-test coverage for critical-failure detection and the new validator signature
</summary>

<objective>
Close the root cause of the billomat incident where 21 prompts (010-030) were marked `completed` after the claude CLI inside the container hit `401 authentication_error`. The current code in `pkg/processor/processor.go` treats a missing completion report as success; after this change, a missing report combined with a detected claude-CLI critical failure fails the prompt instead.
</objective>

<context>
Read /workspace/CLAUDE.md for project conventions (errors wrapping, Ginkgo/Gomega, Counterfeiter).

Read the relevant source to understand the current flow:
- `pkg/report/parse.go` — `ParseFromLog` reads the last 4096 bytes and extracts the `<!-- DARK-FACTORY-REPORT ... DARK-FACTORY-REPORT -->` block.
- `pkg/report/report.go` — the `CompletionReport` struct definition.
- `pkg/report/suffix.go` — the `MarkerStart` / `MarkerEnd` constants and the `Suffix()` agents append to every prompt.
- `pkg/processor/processor.go` around the `validateCompletionReport` function (approx. line 1638) and its call site in `handlePostExecution` (approx. line 1060).

Bug reproduction: A real log produced during a broken-auth batch run in a sibling project contained only these four lines:

```
Starting headless session...
[18:31:29] Failed to authenticate. API Error: 401 {"type":"error","error":{"type":"authentication_error","message":"Invalid authentication credentials"},"request_id":"req_..."}

[18:31:29] --- DONE ---
Failed to authenticate. API Error: 401 {"type":"error","error":{"type":"authentication_error","message":"Invalid authentication credentials"},"request_id":"req_..."}
```

The container exited with status 0 (claude CLI does not exit non-zero on auth error). `ParseFromLog` found no `<!-- DARK-FACTORY-REPORT` marker and returned `(nil, nil)`. The current `validateCompletionReport` at `pkg/processor/processor.go:1645-1648` reads:

```go
if completionReport == nil {
    // No report found — backwards compatible
    return "", nil
}
```

So the prompt was moved to `completed/` and the daemon proceeded to the next prompt, which failed the same way, and so on.

The fix must:
1. Detect claude-CLI-level critical failures in the log even when no completion report is present.
2. Replace the `(string, error)` signature — where `("", nil)` ambiguously meant both "no report" and "success with no summary" — with `(*CompletionReport, error)`.
</context>

<requirements>

1. **Add `pkg/report/failure.go`** with a new exported function:

   ```go
   // ScanForCriticalFailures reads the log file and returns a non-empty reason when the log contains
   // a pattern indicating the claude CLI itself failed (as opposed to the prompt task failing).
   // Returns "" when no critical failure is detected.
   // The returned error is reserved for I/O errors (open/read); a detected failure is signalled via the string.
   func ScanForCriticalFailures(ctx context.Context, logFile string) (string, error)
   ```

   Implementation rules:
   - Open the log file. On missing file, return `("", nil)` — no file means no evidence, not an error from this function's perspective. Any other I/O error is wrapped and returned.
   - Read up to the first 64 KiB of the file (auth errors appear near the top, not the tail). Use a bounded read — do not load the whole file unconditionally.
   - Lowercase the read bytes once, then check for the following patterns (case-insensitive):
     - `failed to authenticate`
     - `"type":"authentication_error"`
     - `api error: 401`
     - `api error: 403`
     - `api error: 429`
     - `api error: 500`
     - `api error: 502`
     - `api error: 503`
     - `api error: 504`
   - Return the **first** matched pattern (original casing) as the reason string, for better log readability. Do not return the lowercased form.
   - If none match, return `("", nil)`.

   Wrap all non-nil I/O errors with `errors.Wrap(ctx, err, "…")` from `github.com/bborbe/errors` (never `fmt.Errorf`, never bare `return err`).

2. **Refactor `validateCompletionReport`** in `pkg/processor/processor.go` to the new signature and behaviour:

   ```go
   // validateCompletionReport parses the completion report from the log and detects claude-CLI-level failures.
   // Returns (report, nil) when a report is present and indicates success.
   // Returns (nil, nil) when no report is present AND no critical failure is detected in the log
   // (backwards compatible — old prompts without reports are treated as successful).
   // Returns (nil, error) when:
   //   - the log shows a claude-CLI critical failure (auth error, API error) even without a report
   //   - a parseable report indicates non-success status (after consistency check)
   //   - the report exists but is malformed
   func validateCompletionReport(ctx context.Context, logFile string) (*report.CompletionReport, error)
   ```

   Behaviour:
   - Call `report.ParseFromLog`. On parse error, log at debug and treat the report as absent (do not fail the prompt on a corrupt tail).
   - When the report is absent, call `report.ScanForCriticalFailures(ctx, logFile)`. If it returns a non-empty reason, return `(nil, errors.Errorf(ctx, "claude CLI critical failure: %s", reason))`. If it returns empty, return `(nil, nil)`.
   - When the report is present, run the existing `ValidateConsistency` + non-success handling. On success, return `(report, nil)`. On non-success, return `(nil, errors.Errorf(ctx, "completion report status: %s", correctedStatus))` — keep the existing failure log lines (`slog.Info "completion report indicates failure"`, `slog.Info "blockers reported"`).

3. **Update the caller** in `handlePostExecution` (same file, around line 1060):

   ```go
   // BEFORE
   summary, err := validateCompletionReport(ctx, logFile)
   if err != nil {
       p.notifyFromReport(ctx, logFile, promptPath)
       return errors.Wrap(ctx, err, "validate completion report")
   }
   if summary != "" {
       pf.SetSummary(summary)
       if err := pf.Save(ctx); err != nil {
           return errors.Wrap(ctx, err, "save summary")
       }
   }
   ```

   ```go
   // AFTER
   completionReport, err := validateCompletionReport(ctx, logFile)
   if err != nil {
       p.notifyFromReport(ctx, logFile, promptPath)
       return errors.Wrap(ctx, err, "validate completion report")
   }
   if completionReport != nil && completionReport.Summary != "" {
       pf.SetSummary(completionReport.Summary)
       if err := pf.Save(ctx); err != nil {
           return errors.Wrap(ctx, err, "save summary")
       }
   }
   ```

   Do not change any other behaviour in `handlePostExecution`. The verification gate, clone vs direct workflow, and move-to-completed steps stay the same.

4. **Tests — `pkg/report/failure_test.go`** (Ginkgo `_ = Describe("ScanForCriticalFailures", …)`, following the pattern in `pkg/report/parse_test.go`). Cover at minimum:
   - log does not exist → `("", nil)`
   - empty log → `("", nil)`
   - log contains only `Starting headless session...` → `("", nil)`
   - log contains `[18:31:29] Failed to authenticate. API Error: 401 {"type":"error","error":{"type":"authentication_error",...` → returns the reason string, nil error
   - log contains `API Error: 500 Internal Server Error` → returns reason, nil error
   - log contains `API Error: 429 Too Many Requests` → returns reason, nil error
   - log is larger than 64 KiB and the match is in the first 64 KiB → detected
   - log is larger than 64 KiB and the ONLY match is past 64 KiB → not detected (documents the cap)
   - uppercase `FAILED TO AUTHENTICATE` → detected (case-insensitive)
   - unrelated text containing the word "authenticate" (e.g. `rewriting authenticate handler`) → `("", nil)` — only the exact patterns listed in requirement 1 match

5. **Tests — `pkg/processor/processor_internal_test.go`** (new `Describe("validateCompletionReport", …)` or extend the existing internal test suite). Cover:
   - log with a valid success report → returns non-nil `*CompletionReport`, nil error, Summary populated
   - log with a `partial` status report and a failing verification exit code → non-nil error (consistency check fires)
   - log with a `failed` status report → non-nil error
   - log with no report and no critical failure pattern → returns `(nil, nil)`
   - log with no report but containing `Failed to authenticate. API Error: 401` → returns `(nil, non-nil error)` whose message contains `claude CLI critical failure`
   - log with no report but containing `API Error: 500` → returns `(nil, non-nil error)`
   - log with a malformed JSON completion report → returns `(nil, nil)` (parse error downgraded to "no report" so we then run the critical-failure scan)
   - log that is malformed JSON AND contains an auth-error pattern → returns `(nil, non-nil error)` (critical failure still caught)

   Use a `t.TempDir()` (or `GinkgoT().TempDir()`) to write synthetic log files per test case. No Counterfeiter mocks required for these tests — they are pure file-reading logic.

6. **Keep all other behaviour untouched**. Do NOT change:
   - `pkg/report/parse.go` — `ParseFromLog` stays as-is (tail-only scan for the report marker).
   - `pkg/report/suffix.go` — the agent instructions stay identical.
   - The retry logic in `handlePromptFailure` — auth errors will consume a retry slot; that is acceptable for this fix.
   - `validateClaudeAuth` in `pkg/executor/executor.go` — out of scope for this prompt.

7. **Run verification**: `make precommit` must pass. This runs the full test suite and linters.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Do NOT add any new exported names to `pkg/report` beyond `ScanForCriticalFailures`.
- Wrap all non-nil errors with `errors.Wrap` / `errors.Wrapf` / `errors.Errorf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`.
- Keep error messages lowercase and context-free (no file names, no "%v").
- No `fmt.Errorf`, no `errors.New`, no `pkg/errors` in changed files.
- Do not touch `go.mod` / `go.sum` / `vendor/`.
- Do not add a feature flag — this is strictly a correctness fix; old behaviour was buggy and should not be opt-in.
- The `(nil, nil)` return from `validateCompletionReport` is intentional and documented — it represents the "no report, no failure" backwards-compatible case. Do not try to remove it by introducing a sentinel `CompletionReport`.
- Existing tests must still pass.
</constraints>

<verification>
Run `make precommit` in the repo root — must exit 0. The Ginkgo suites in requirements 4 and 5 are the authoritative gate; `make precommit` executes them.
</verification>
