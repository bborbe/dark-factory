---
spec: ["001"]
status: completed
container: dark-factory-050-completion-report-parse-log
dark-factory-version: dev
created: "2026-03-02T20:40:34Z"
queued: "2026-03-02T20:40:34Z"
started: "2026-03-02T20:40:34Z"
completed: "2026-03-02T20:58:51Z"
---
<objective>
Parse the completion report JSON from the log file after YOLO container exits.
dark-factory reads the last lines of the log, extracts the JSON between markers,
and uses the result to determine if the prompt truly succeeded.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/report/report.go` — CompletionReport struct (created by previous prompt).
Read `pkg/report/suffix.go` — the markers `DARK-FACTORY-REPORT` (created by previous prompt).
Read `pkg/processor/processor.go` — `processPrompt()` calls executor, then commits on success.
Log files are at `prompts/log/NNN-name.log`.
</context>

<requirements>
1. Add `pkg/report/parse.go` with a function to extract the report from log content:

```go
package report

// ParseFromLog reads the last N bytes of a log file and extracts the CompletionReport.
// Returns nil if no report found (graceful — old prompts won't have one).
func ParseFromLog(logFile string) (*CompletionReport, error)
```

Implementation:
- Read the last 4096 bytes of the log file (the report is near the end)
- Find `<!-- DARK-FACTORY-REPORT` and `DARK-FACTORY-REPORT -->` markers
- Extract the single JSON line between them
- Unmarshal into `CompletionReport`
- Return `nil, nil` if markers not found (no report = old prompt, not an error)
- Return `nil, err` if markers found but JSON is malformed

2. Add constants for the markers in `pkg/report/suffix.go`:

```go
const (
    MarkerStart = "<!-- DARK-FACTORY-REPORT"
    MarkerEnd   = "DARK-FACTORY-REPORT -->"
)
```

Use these constants in both `Suffix()` and `ParseFromLog()`.

3. In `pkg/processor/processor.go` — after `executor.Execute()` succeeds (line ~224), parse the log:

```go
// After executor.Execute succeeds, before MoveToCompleted:
completionReport, err := report.ParseFromLog(logFile)
if err != nil {
    log.Printf("dark-factory: failed to parse completion report: %v", err)
    // Continue — don't fail the prompt just because report parsing failed
}
if completionReport != nil {
    log.Printf("dark-factory: completion report: status=%s summary=%s", completionReport.Status, completionReport.Summary)
    if completionReport.Status != "success" {
        // Report says not success — treat as failure
        log.Printf("dark-factory: completion report status is %q, treating as failed", completionReport.Status)
        if len(completionReport.Blockers) > 0 {
            log.Printf("dark-factory: blockers: %v", completionReport.Blockers)
        }
        return fmt.Errorf("completion report status: %s", completionReport.Status)
    }
}
// If no report found (nil), continue as before — backwards compatible
```

4. Add unit tests in `pkg/report/parse_test.go` (Ginkgo v2 + Gomega):

   a. Success: log ends with valid report → parses correctly
   b. No report: log without markers → returns nil, nil
   c. Malformed JSON: markers present but invalid JSON → returns nil, error
   d. Partial status: `{"status":"partial","summary":"half done","blockers":["make precommit fails"]}` → parses correctly
   e. Report not at very end: report followed by "Type /exit" text → still parses correctly
   f. Large log: 100KB of output before the report → still finds it in last 4096 bytes

5. Add integration test in `pkg/processor/processor_test.go`:
   - Mock executor writes a log file with a completion report
   - Verify processor handles `status: "success"` → continues to commit
   - Verify processor handles `status: "failed"` → returns error
</requirements>

<constraints>
- Do NOT modify `pkg/executor/executor.go`
- Do NOT modify `pkg/report/report.go` (CompletionReport struct)
- Do NOT break existing tests
- Missing report = backwards compatible (proceed as before, just log a note)
- Only `status != "success"` causes failure — "partial" and "failed" both fail
- Read from END of file, not beginning (logs can be huge)
</constraints>

<verification>
Run: `make test`
Run: `go test -v ./pkg/report/...`
Run: `go test -v ./pkg/processor/...`
Run: `make precommit`
</verification>

<success_criteria>
- `ParseFromLog` extracts report from log file tail
- Processor fails prompt when report status != "success"
- Processor continues normally when no report found (backwards compatible)
- All tests pass
- `make precommit` passes
</success_criteria>
