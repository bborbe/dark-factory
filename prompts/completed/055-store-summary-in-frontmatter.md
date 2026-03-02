---
status: completed
container: workspace-055-store-summary-in-frontmatter
dark-factory-version: dev
created: "2026-03-02T22:41:26Z"
queued: "2026-03-02T22:41:26Z"
started: "2026-03-02T22:47:22Z"
completed: "2026-03-02T22:50:29Z"
---
<objective>
Store the completion report summary in the prompt's frontmatter when execution finishes.
After a successful prompt, the completed file should contain `summary: "..."` in its frontmatter.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/processor/processor.go` — `processPrompt` and `validateCompletionReport`.
Read `pkg/prompt/prompt.go` — `Frontmatter` struct, `MarkCompleted`, `MarkFailed`, `Save`.
Read `pkg/report/report.go` — `CompletionReport` struct.
Read `pkg/report/parse.go` — `ParseFromLog`.
Read `pkg/processor/processor_test.go` — existing tests.
</context>

<requirements>

## 1. Add Summary field to Frontmatter

In `pkg/prompt/prompt.go`, add a `Summary` field to the `Frontmatter` struct:

```go
type Frontmatter struct {
    Status             string `yaml:"status"`
    Summary            string `yaml:"summary,omitempty"`
    Container          string `yaml:"container,omitempty"`
    DarkFactoryVersion string `yaml:"dark-factory-version,omitempty"`
    Created            string `yaml:"created,omitempty"`
    Queued             string `yaml:"queued,omitempty"`
    Started            string `yaml:"started,omitempty"`
    Completed          string `yaml:"completed,omitempty"`
}
```

Place `Summary` right after `Status` so it's visible when scanning frontmatter.

## 2. Add SetSummary method to PromptFile

```go
func (pf *PromptFile) SetSummary(summary string) {
    pf.Frontmatter.Summary = summary
}
```

## 3. Change validateCompletionReport to return the summary

Current signature returns only `error`. Change it to also return the summary string:

```go
func validateCompletionReport(ctx context.Context, logFile string) (string, error)
```

- On success: return `(completionReport.Summary, nil)`
- On failure: return `("", error)`
- On no report / parse error: return `("", nil)`

## 4. Update processPrompt to store summary

In `processPrompt`, after `validateCompletionReport` succeeds:

```go
summary, err := validateCompletionReport(ctx, logFile)
if err != nil {
    return err
}

// Store summary in frontmatter before moving to completed
if summary != "" {
    pf.SetSummary(summary)
    if err := pf.Save(); err != nil {
        return errors.Wrap(ctx, err, "save summary")
    }
}
```

This must happen BEFORE `MoveToCompleted` so the summary is saved to the file in queueDir, then moved.

## 5. Update tests

Update existing tests in `pkg/processor/processor_test.go` that call or mock `validateCompletionReport` behavior to account for the summary return value.

Add a test that verifies the summary is stored in the completed prompt file's frontmatter after successful execution with a completion report containing a summary.

</requirements>

<constraints>
- Do NOT change any business logic beyond adding summary storage
- Do NOT modify function signatures except `validateCompletionReport`
- Keep backwards compatible — missing summary in report or log means empty summary (no error)
- Use Ginkgo v2 + Gomega for tests
- Summary field must be `omitempty` so old prompts without summary don't get an empty field
</constraints>

<verification>
Run: `make test`
Run: `make precommit`
</verification>

<success_criteria>
- Completed prompt files contain `summary: "..."` in frontmatter when completion report has a summary
- No summary field when completion report is missing or has empty summary
- All existing tests pass
- `make precommit` passes
</success_criteria>
