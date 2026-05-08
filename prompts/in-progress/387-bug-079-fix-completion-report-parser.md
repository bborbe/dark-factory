---
status: committing
spec: [079-bug-completion-report-parser-tail-boundary]
summary: Fixed ParseFromLog to select last complete marker pair in tail window and export ErrStartWithoutEnd sentinel so validator can distinguish actionable boundary failures from other parse errors
container: dark-factory-387-bug-079-fix-completion-report-parser
dark-factory-version: v0.154.0
created: "2026-05-08T09:00:00Z"
queued: "2026-05-08T09:17:32Z"
started: "2026-05-08T09:17:34Z"
branch: dark-factory/bug-completion-report-parser-tail-boundary
---

<summary>
- `ParseFromLog` is changed to select the last complete start/end marker pair in the 4096-byte tail window, not the first start + first end independently
- A new exported sentinel `ErrStartWithoutEnd` is added to `pkg/report` so callers can distinguish "start marker present but no end follows" (actionable) from JSON unmarshal errors (non-actionable)
- When a start marker is found in the tail but no end marker follows it, `ParseFromLog` wraps `ErrStartWithoutEnd` instead of returning a generic error — the existing error message text is preserved so the existing parse test still passes
- `Validator.Validate` uses `stderrors.Is` to detect `ErrStartWithoutEnd` and propagates it directly instead of downgrading it to a debug-level "no report"; all other parse errors (e.g. malformed JSON) continue to be downgraded for backwards compatibility
- Seven new unit tests cover: last-block-wins, the exact production failure layout (9235-byte log), orphaned end marker, start-without-end, end-without-start, and zero-markers scenarios
- One new validator test confirms that a start-without-end error propagates (not downgrades) to the caller
- All existing tests in `pkg/report/parse_test.go` and `pkg/completionreport/validator_test.go` continue to pass unchanged
- A CHANGELOG entry documents the fix under `## Unreleased`
</summary>

<objective>
Fix the completion report parser so it selects the last complete `<!-- DARK-FACTORY-REPORT … DARK-FACTORY-REPORT -->` marker pair in the tail window, eliminating the orphaned-end-marker boundary artifact that caused the daemon to commit a prompt whose agent-reported status was `failed`. Export a sentinel error to let the validator distinguish "start without matching end" (must escalate to failure) from other parse errors (may be downgraded for backwards compatibility).
</objective>

<context>
Read `CLAUDE.md` for project conventions (errors wrapping, Ginkgo/Gomega, Counterfeiter).
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Files to read in full before editing:
- `pkg/report/parse.go` — the parser to fix (~82 lines)
- `pkg/report/parse_test.go` — existing tests to preserve; shows the Ginkgo structure
- `pkg/report/suffix.go` — `MarkerStart` / `MarkerEnd` constants (DO NOT change)
- `pkg/report/report.go` — `CompletionReport` struct
- `pkg/completionreport/validator.go` — the validator to fix (~95 lines)
- `pkg/completionreport/validator_test.go` — existing validator tests to preserve

**Production failure context** (dark-factory v0.154.0, prompt `005-update-build-golang-1.26.3.md`):
- Log file was 9235 bytes; the tail window is the last 4096 bytes (tail starts at byte 5139)
- 1st `<!-- DARK-FACTORY-REPORT` at byte 5122 — just OUTSIDE the tail (17 bytes before the window start)
- 1st `DARK-FACTORY-REPORT -->` at byte 5693 — inside the tail (orphaned end marker)
- 2nd `<!-- DARK-FACTORY-REPORT` at byte 7371 — inside the tail
- 2nd `DARK-FACTORY-REPORT -->` at byte 7942 — inside the tail
- Both JSON blocks contained `"status":"failed"`
- Old parser: `strings.Index(tail, MarkerStart)` returned the 2nd start (offset 2232 in tail); `strings.Index(tail, MarkerEnd)` returned the ORPHANED 1st end (offset 554 in tail); the `endIdx <= startIdx` guard fired; error was returned; validator downgraded it to "no report"; daemon committed the failed prompt
</context>

<requirements>

## 1. Export `ErrStartWithoutEnd` sentinel in `pkg/report/parse.go`

Add the `stderrors` import alias (standard pattern in this project — see `pkg/queuescanner/scanner.go`):

```go
import (
    stderrors "errors"
    // ... other imports unchanged
)
```

Add the exported sentinel immediately after the `tailBytes` constant:

```go
// ErrStartWithoutEnd is returned by ParseFromLog when a start marker is found in the tail
// but no matching end marker follows it. This signals an actionable parse failure —
// the agent began a report but the tail window was cut before the closing marker.
var ErrStartWithoutEnd = stderrors.New("found start marker but no valid end marker")
```

## 2. Fix the marker search algorithm in `ParseFromLog`

Replace the current `strings.Index` / `endIdx <= startIdx` block with a two-step search that finds the **last** start marker, then searches for an end marker only in the text after that start marker.

Full replacement for the marker-search section (beginning after `tail := string(buf[:n])`):

```go
// Find the last start marker in the tail.
startIdx := strings.LastIndex(tail, MarkerStart)
if startIdx == -1 {
    // No report found — not an error, old prompts won't have one.
    return nil, nil //nolint:nilnil
}

// Search for an end marker only in the text that follows the last start marker.
remaining := tail[startIdx+len(MarkerStart):]
relEndIdx := strings.Index(remaining, MarkerEnd)
if relEndIdx == -1 {
    return nil, errors.Wrap(ctx, ErrStartWithoutEnd, "parse report tail boundary")
}

// Extract JSON between the markers.
jsonStr := strings.TrimSpace(remaining[:relEndIdx])

// Unmarshal.
var report CompletionReport
if err := json.Unmarshal([]byte(jsonStr), &report); err != nil {
    return nil, errors.Wrap(ctx, err, "unmarshal completion report JSON")
}
return &report, nil
```

**Remove** the old lines `startIdx := strings.Index(...)`, `endIdx := strings.Index(...)`,
the `endIdx == -1 || endIdx <= startIdx` guard, and the `jsonStart := startIdx + len(MarkerStart)` line.
**Do not change** the file-open, stat, seek, or read sections above this block.

Why this works for the production scenario:
- The tail contains: `[orphaned MarkerEnd] … [MarkerStart] … [JSON] … [MarkerEnd]`
- `strings.LastIndex` skips the orphaned end and finds the real second start marker
- `strings.Index` in `remaining` finds the correct end marker immediately after
- The correct `status: failed` JSON is extracted and returned

## 3. Update `pkg/completionreport/validator.go` to propagate `ErrStartWithoutEnd`

Add the `stderrors` import to `validator.go`:

```go
import (
    "context"
    stderrors "errors"
    "log/slog"

    "github.com/bborbe/errors"

    "github.com/bborbe/dark-factory/pkg/report"
)
```

In `Validate`, replace the current error-handling block for `report.ParseFromLog`:

```go
// BEFORE:
completionReport, err := report.ParseFromLog(ctx, logFile)
if err != nil {
    slog.Debug("failed to parse completion report", "error", err)
    // Parse error — downgrade to "no report" and fall through to critical failure scan.
    completionReport = nil
}

// AFTER:
completionReport, err := report.ParseFromLog(ctx, logFile)
if err != nil {
    if stderrors.Is(err, report.ErrStartWithoutEnd) {
        // Start marker found but no end marker in the tail: the agent began a report
        // but the tail window was cut before the closing delimiter.
        // This is an actionable failure — do not downgrade to "no report".
        return nil, errors.Wrap(ctx, err, "parse completion report")
    }
    // Other parse errors (e.g. both markers present but JSON is malformed):
    // downgrade to "no report" for backwards compatibility.
    slog.Debug("failed to parse completion report", "error", err)
    completionReport = nil
}
```

No other changes to `Validate` — the `completionReport == nil` path, critical failure scan,
consistency check, and non-success handling all stay exactly as they are.

## 4. Add new test cases to `pkg/report/parse_test.go`

Add a new `Context("last-block-wins and tail-boundary cases", ...)` block inside the
existing `Describe("ParseFromLog", ...)`. Use the same `BeforeEach` / `AfterEach` / `logFile`
variables already declared at the top of that `Describe`.

Add ALL of the following `It` blocks:

### 4a. Two complete blocks — last block's status wins

```go
It("returns last block when log contains two complete report blocks", func() {
    content := `output line 1
output line 2

` + report.MarkerStart + `
{"status":"success","summary":"first attempt","blockers":[]}
` + report.MarkerEnd + `

More output after first report.

` + report.MarkerStart + `
{"status":"failed","summary":"second attempt","blockers":["compile error"]}
` + report.MarkerEnd + `
`
    Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())

    result, err := report.ParseFromLog(context.Background(), logFile)
    Expect(err).NotTo(HaveOccurred())
    Expect(result).NotTo(BeNil())
    Expect(result.Status).To(Equal("failed"))
    Expect(result.Summary).To(Equal("second attempt"))
})
```

### 4b. Production reproduction: 9235-byte log, two failed blocks, first start outside tail

This test mirrors the exact byte layout of the production failure described in the spec.
Include a Go comment block at the top of the `It` reproducing the byte-offset table from the spec (Reproduction → marker offsets) so future readers know why the magic numbers exist.
The file is 9235 bytes total; the tail window starts at byte 5139 (9235 − 4096).
Block1's start marker begins at byte 5122 (17 bytes before the tail window — outside the tail).
Block1's end marker lands at byte 5693 (inside the tail — the "orphan").
Block2's start marker is at byte 7371 and end marker at byte 7942 (both inside the tail).

Construction formula (the sizes below are exact):
- `preamble`: 5122 bytes
- `block1 = MarkerStart + "\n" + json1 + "\n" + MarkerEnd + "\n"` where `len(json1) == 545` → `len(block1) == 595`
- `filler`: 1654 bytes  (= 7371 − 5122 − 595)
- `block2 = MarkerStart + "\n" + json2 + "\n" + MarkerEnd + "\n"` where `len(json2) == 545` → `len(block2) == 595`
- `trailing`: 1269 bytes  (= 9235 − 7371 − 595)
- Total: 5122 + 595 + 1654 + 595 + 1269 = 9235 ✓

Derivation of json1/json2 length:
- `len(MarkerStart)` = 24, `len(MarkerEnd)` = 23
- `json_len = 5693 − 5122 − 24 − 2 = 545`  (5693 is where MarkerEnd starts; subtract MarkerStart len and 2 newlines)

Use a closure to build a valid 545-byte failed-report JSON by padding:

```go
It("production reproduction: 9235-byte log with orphaned end marker in tail window", func() {
    makeFailedJSON := func(wantLen int) string {
        // Base JSON for a failed report (~116 chars; compute via len() at runtime)
        base := `{"status":"failed","blockers":[],"summary":"make precommit failed","verification":{"command":"make precommit","exitCode":1}}`
        // Append a pad field to reach wantLen.
        // `,"_p":"` = 6 chars, closing `"` = 1 char, total overhead = 7 + padLen
        // base ends with '}'; replace trailing '}' with `,"_p":"XXX..."}`
        padLen := wantLen - len(base) - 8 // 8 = len(`,"_p":""}`)
        Expect(padLen).To(BeNumerically(">=", 0),
            "base JSON is already %d bytes, cannot pad to %d", len(base), wantLen)
        return base[:len(base)-1] + `,"_p":"` + strings.Repeat("x", padLen) + `"}`
    }

    const (
        wantTotal     = 9235
        tailBytesSize = 4096 // must match tailBytes in parse.go
        block1Start   = 5122
        block1End     = 5693 // byte where block1's MarkerEnd begins
        block2Start   = 7371
    )
    tailWindowStart := wantTotal - tailBytesSize // = 5139
    Expect(block1Start).To(BeNumerically("<", tailWindowStart),
        "block1 start must be before the tail window")
    Expect(block1End).To(BeNumerically(">=", tailWindowStart),
        "block1 end must be inside the tail window (orphaned)")

    // json length = block1End - block1Start - len(MarkerStart) - 2 newlines
    jsonLen := block1End - block1Start - len(report.MarkerStart) - 2
    Expect(jsonLen).To(Equal(545))

    json1 := makeFailedJSON(jsonLen)
    Expect(len(json1)).To(Equal(jsonLen))
    json2 := makeFailedJSON(jsonLen)

    block1 := report.MarkerStart + "\n" + json1 + "\n" + report.MarkerEnd + "\n"
    block2 := report.MarkerStart + "\n" + json2 + "\n" + report.MarkerEnd + "\n"
    Expect(len(block1)).To(Equal(595))

    filler    := strings.Repeat("f", block2Start-(block1Start+len(block1)))
    trailing  := strings.Repeat("t", wantTotal-block2Start-len(block2))
    preamble  := strings.Repeat("a", block1Start)

    content := preamble + block1 + filler + block2 + trailing
    Expect(len(content)).To(Equal(wantTotal))

    Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())

    result, err := report.ParseFromLog(context.Background(), logFile)
    Expect(err).NotTo(HaveOccurred())
    Expect(result).NotTo(BeNil())
    Expect(result.Status).To(Equal("failed"))
})
```

**Important**: the `strings` import is already used in `parse_test.go`; add it only if absent.
The `report.MarkerStart` and `report.MarkerEnd` constants are exported from `pkg/report/suffix.go`.

### 4c. Orphaned end marker followed by complete pair — parser returns the complete pair

```go
It("ignores orphaned end marker when a complete pair follows it in the tail", func() {
    content := report.MarkerEnd + `

Some output between markers.

` + report.MarkerStart + `
{"status":"success","summary":"recovered","blockers":[]}
` + report.MarkerEnd + `
`
    Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())

    result, err := report.ParseFromLog(context.Background(), logFile)
    Expect(err).NotTo(HaveOccurred())
    Expect(result).NotTo(BeNil())
    Expect(result.Status).To(Equal("success"))
    Expect(result.Summary).To(Equal("recovered"))
})
```

### 4d. Only start marker, no end — parser returns wrapped ErrStartWithoutEnd

```go
It("returns ErrStartWithoutEnd when start marker is present but end marker is missing", func() {
    content := `starting session...

` + report.MarkerStart + `
{"status":"failed","summary":"truncated","blockers":[]}
`
    // NOTE: no closing MarkerEnd
    Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())

    result, err := report.ParseFromLog(context.Background(), logFile)
    Expect(err).To(HaveOccurred())
    Expect(stderrors.Is(err, report.ErrStartWithoutEnd)).To(BeTrue())
    Expect(result).To(BeNil())
})
```

Add `stderrors "errors"` to the import block of `parse_test.go` (alongside the existing imports).

### 4e. Only end marker, no start — parser returns (nil, nil)

```go
It("returns (nil, nil) when only an end marker is present (no start marker)", func() {
    content := `some output
` + report.MarkerEnd + `
`
    Expect(os.WriteFile(logFile, []byte(content), 0600)).To(Succeed())

    result, err := report.ParseFromLog(context.Background(), logFile)
    Expect(err).NotTo(HaveOccurred())
    Expect(result).To(BeNil())
})
```

## 5. Add validator propagation tests to `pkg/completionreport/validator_test.go`

Add TWO new `It` blocks inside `Describe("Validator", ...)`. Do NOT modify any existing test.
`pkg/report` is already imported in `validator_test.go` — use `report.MarkerStart` / `report.MarkerEnd` directly.

### 5a. Start-without-end propagates as failure

```go
It("propagates error when log has start marker but no end marker", func() {
    writeLog(`Starting session...

` + report.MarkerStart + `
{"status":"failed","summary":"truncated","blockers":[]}
`)
    // NOTE: no closing MarkerEnd
    r, err := validator.Validate(ctx, logFile)
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("parse report tail boundary"))
    Expect(r).To(BeNil())
})
```

### 5b. Production reproduction at validator boundary — failed status escalates

This mirrors the production failure end-to-end: the validator (not just the parser) must return a non-nil error for the same 9235-byte log layout. The test reuses the same byte-layout construction as test 4b. Extract the JSON-padding helper and constants into a small unexported package-test helper if they would otherwise be duplicated, OR inline them — agent's choice, both are fine.

```go
It("propagates failure when production-shaped log has orphaned end marker in tail", func() {
    // Reuse the 9235-byte construction from pkg/report/parse_test.go test 4b.
    // Both report blocks contain "status":"failed".
    content := buildProductionFailureLog() // helper as per test 4b construction
    writeLog(content)

    r, err := validator.Validate(ctx, logFile)
    Expect(err).To(HaveOccurred())
    Expect(r).To(BeNil())
    // The validator returns a non-nil error reflecting the parsed status:failed,
    // NOT a downgraded "no report" path.
})
```

If extracting the helper across packages adds churn, inline the construction directly here using the same formula documented in test 4b. The acceptance criterion is that the validator returns `(nil, non-nil error)` against this byte-exact input, confirming end-to-end escalation.

## 6. Add CHANGELOG entry

Add to `## Unreleased` in `CHANGELOG.md` (create the section if absent):

```markdown
- fix: ParseFromLog selects last complete marker pair in tail window, preventing orphaned end-marker boundary artifact from silently swallowing agent-reported failures
```

## 7. Verification commands after each step

Run `make test` after each changed file to catch compilation errors early:
```
cd /workspace && make test   # after pkg/report/parse.go
cd /workspace && make test   # after pkg/completionreport/validator.go
cd /workspace && make test   # after all test files
```

Run `make precommit` once at the very end.

</requirements>

<constraints>
- Public function signature `report.ParseFromLog(ctx, logFile) (*CompletionReport, error)` MUST NOT change
- `MarkerStart` and `MarkerEnd` constants in `pkg/report/suffix.go` MUST NOT change
- Logs with no start marker in the tail MUST still return `(nil, nil)` — backwards compatible
- Logs with no start marker AND no end marker MUST still return `(nil, nil)`
- Validator behavior for genuinely missing reports MUST stay the same: fall through to `ScanForCriticalFailures`
- `ErrStartWithoutEnd` MUST be exported (capital `E`) so the validator package can reference it
- The existing test `"returns error when end marker is missing"` in `parse_test.go` checks `ContainSubstring("no valid end marker")` — the new error text `errors.Wrap(ctx, ErrStartWithoutEnd, "parse report tail boundary")` produces a message containing "no valid end marker" (from the sentinel text), so this test continues to pass without modification
- The existing validator test `"returns (nil, nil) for malformed JSON completion report"` MUST still pass — malformed-JSON errors are NOT `ErrStartWithoutEnd` so they continue to be downgraded
- Do NOT change `pkg/report/suffix.go`, `pkg/report/report.go`, `pkg/report/failure.go`, or any file outside `pkg/report/` and `pkg/completionreport/`
- Do NOT commit — dark-factory handles git
- Wrap all non-nil errors with `errors.Wrap` / `errors.Wrapf` / `errors.Errorf` from `github.com/bborbe/errors`; never `fmt.Errorf`, never bare `return err`
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Existing tests in `pkg/report/parse_test.go` and `pkg/completionreport/validator_test.go` must all still pass
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -n "LastIndex" pkg/report/parse.go` — must show `strings.LastIndex` for the start marker search
2. `grep -n "ErrStartWithoutEnd" pkg/report/parse.go pkg/completionreport/validator.go` — sentinel defined in parse.go, referenced in validator.go
3. `grep -n "stderrors.Is.*ErrStartWithoutEnd" pkg/completionreport/validator.go` — propagation guard present
4. `go test -v -run "ParseFromLog" ./pkg/report/...` — all tests pass; new tests are listed
5. `go test -v -run "Validator" ./pkg/completionreport/...` — all tests pass; new propagation test is listed

Reproduction replay (per spec section "Verification"):
1. The test added in requirement 4b constructs the 9235-byte production log and calls `report.ParseFromLog` directly — it must return `status: failed`, not nil, not an error.
2. The test added in requirement 5b runs the same byte-layout through `Validator.Validate` and asserts `(nil, non-nil error)` — end-to-end escalation confirmed.
</verification>
