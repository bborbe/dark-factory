---
spec: ["001"]
status: completed
summary: Added verification field to completion report with validation
container: dark-factory-064-add-verification-to-completion-report
dark-factory-version: v0.14.5
created: "2026-03-03T21:01:45Z"
queued: "2026-03-03T21:01:45Z"
started: "2026-03-03T21:01:45Z"
completed: "2026-03-03T21:09:57Z"
---
# Add verification field to completion report

## Goal

Add a `verification` field to the `CompletionReport` struct so dark-factory can reject `status: "success"` when the verification command had a non-zero exit code. This prevents YOLO from self-reporting success when `make precommit` failed.

## Current Behavior

`CompletionReport` has `status`, `summary`, and `blockers`. Dark-factory trusts YOLO's self-reported status. YOLO can claim success even when `make precommit` failed.

## Expected Behavior

`CompletionReport` includes an optional `verification` field with the command and exit code. When dark-factory sees `status: "success"` but `verification.exitCode != 0`, it overrides to `"partial"` and logs a warning.

## Implementation

### 1. Add `Verification` struct to `pkg/report/report.go`

```go
// Verification records the result of a verification command.
type Verification struct {
    Command  string `json:"command"`
    ExitCode int    `json:"exitCode"`
}

// CompletionReport is the structured output the AI agent must produce.
type CompletionReport struct {
    Status       string        `json:"status"`
    Summary      string        `json:"summary"`
    Blockers     []string      `json:"blockers"`
    Verification *Verification `json:"verification,omitempty"`
}
```

### 2. Add validation method to `CompletionReport`

Add a method that checks for inconsistency between status and verification:

```go
// ValidateConsistency checks if the reported status is consistent with verification results.
// Returns the corrected status and true if it was overridden.
func (r *CompletionReport) ValidateConsistency() (correctedStatus string, overridden bool) {
    if r.Status == "success" && r.Verification != nil && r.Verification.ExitCode != 0 {
        return "partial", true
    }
    return r.Status, false
}
```

### 3. Use `ValidateConsistency` in `pkg/processor/processor.go`

In the `validateCompletionReport` function, after parsing the report and before returning, call `ValidateConsistency`:

```go
correctedStatus, overridden := completionReport.ValidateConsistency()
if overridden {
    slog.Warn(
        "overriding self-reported status",
        "reported", completionReport.Status,
        "corrected", correctedStatus,
        "verificationCommand", completionReport.Verification.Command,
        "verificationExitCode", completionReport.Verification.ExitCode,
    )
    completionReport.Status = correctedStatus
}
```

### 4. Update `pkg/report/suffix.go`

Update the completion report template/suffix that gets appended to prompts. Add the `verification` field to the example JSON so YOLO knows to include it:

```json
{"status":"success|partial|failed","summary":"...","blockers":[],"verification":{"command":"make precommit","exitCode":0}}
```

### 5. Tests

In `pkg/report/report_test.go`:

- `ValidateConsistency` returns success unchanged when verification is nil (backwards compatible)
- `ValidateConsistency` returns success unchanged when verification.exitCode is 0
- `ValidateConsistency` overrides success to partial when verification.exitCode != 0
- `ValidateConsistency` does not override non-success statuses (failed stays failed)
- Parse a report JSON with verification field and verify it deserializes correctly
- Parse a report JSON without verification field (backwards compatible)

In `pkg/processor/processor_test.go`:

- Add test that processor rejects success when verification exit code is non-zero

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Coverage ≥80% for changed packages
- Must be backwards compatible — old reports without `verification` field must still work
- Follow existing patterns in `pkg/report/` exactly
