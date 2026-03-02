<objective>
Add a dynamic completion report suffix that dark-factory appends to every prompt before passing it to the YOLO container.
This makes the output machine-parseable without requiring any YOLO CLAUDE.md configuration.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/processor/processor.go` — `processPrompt()` method at line ~172 gets content and passes to executor.
Read `pkg/prompt/prompt.go` — `Content()` at line ~484 strips frontmatter and returns body.
Read `pkg/executor/executor.go` — `Execute()` receives promptContent string.
</context>

<requirements>
1. Create `pkg/report/report.go` with the CompletionReport struct:

```go
package report

// CompletionReport is the structured output the AI agent must produce at the end of every prompt execution.
type CompletionReport struct {
    Status   string   `json:"status"`   // "success", "partial", "failed"
    Summary  string   `json:"summary"`  // one-line description of what was done
    Blockers []string `json:"blockers"` // why it's not success (empty on success)
}
```

2. Create `pkg/report/suffix.go` with a function that returns the suffix string:

```go
package report

// Suffix returns the markdown text that dark-factory appends to every prompt.
// It instructs the AI agent to output a structured completion report.
func Suffix() string {
    return `

---

## Completion Report (MANDATORY)

As your VERY LAST output, you MUST produce a completion report in this EXACT format.
The JSON must be on a SINGLE LINE between the markers.

<!-- DARK-FACTORY-REPORT
{"status":"success","summary":"Replaced splitFrontmatter with adrg/frontmatter library","blockers":[]}
DARK-FACTORY-REPORT -->

Field values:
- status: "success" = all requirements met, verification passed. "partial" = some work done but blockers remain. "failed" = could not complete the task.
- summary: One sentence describing what was accomplished.
- blockers: Array of strings explaining what prevented success. Empty array [] when status is "success".

This report is MANDATORY. Do not skip it.
`
}
```

3. In `pkg/processor/processor.go` — in `processPrompt()`, append the suffix to content before passing to executor:

```go
// After getting content (line ~174) and before executor.Execute (line ~224):
content = content + report.Suffix()
```

Add the import for `"github.com/bborbe/dark-factory/pkg/report"`.

4. Add unit tests in `pkg/report/report_test.go`:
   - Test that `Suffix()` contains the marker `DARK-FACTORY-REPORT`
   - Test that `CompletionReport` can be marshaled/unmarshaled via `encoding/json`
   - Test round-trip: marshal a CompletionReport, verify all fields survive

5. Add unit test in `pkg/processor/processor_test.go`:
   - Verify that the content passed to executor contains the completion report suffix
   - The mock executor should capture the promptContent argument and assert it ends with the suffix markers
</requirements>

<constraints>
- Do NOT modify `pkg/executor/executor.go` — the executor just passes content through
- Do NOT modify `pkg/prompt/prompt.go` — Content() stays as-is
- Do NOT modify any existing tests
- The suffix is a constant string — no dynamic fields, no template rendering
- Follow existing package patterns: see `pkg/prompt/` for style
- Use Ginkgo v2 + Gomega for tests, Counterfeiter for mocks
</constraints>

<verification>
Run: `make test`
Run: `go test -v ./pkg/report/...`
Run: `go test -v ./pkg/processor/...`
Run: `make precommit`
</verification>

<success_criteria>
- `CompletionReport` struct exists in `pkg/report/report.go`
- `Suffix()` function returns the instruction text with markers
- `processPrompt` appends suffix to content before executor call
- All tests pass
- `make precommit` passes
</success_criteria>
