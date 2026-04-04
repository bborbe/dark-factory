---
status: draft
created: "2026-04-04T14:31:56Z"
---

<summary>
- "prompt blocked" log message now includes which specific prompt numbers are missing from completed/
- New helper function `FindMissingCompleted` returns the list of missing prompt numbers
- Failed prompts in in-progress/ are reported with their status so operators know what to fix
- Debug log level shows all completed numbers found during the check
- Blocked message is logged at most once per prompt until state changes (no more spam every 5 seconds)
</summary>

<objective>
When `dark-factory daemon` or `dark-factory run` encounters a blocked prompt, the log message `"prompt blocked" reason="previous prompt not completed"` gives no indication of WHICH previous prompt is missing or WHY. Operators must manually inspect `prompts/completed/` and `prompts/in-progress/` to diagnose. Improve the diagnostic output so the blocked reason includes the missing prompt numbers and their current status (e.g., "failed", "executing", "not found").
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these files before making changes:
- `pkg/processor/processor.go` — `processQueue()` at line ~384 logs the blocked message. This is where the improved diagnostics must be added.
- `pkg/prompt/prompt.go` — `AllPreviousCompleted()` at line ~1217 checks completed directory. Add a companion function `FindMissingCompleted()` that returns the missing numbers.
- `pkg/prompt/prompt.go` — `extractNumberFromFilename()` at line ~1205 extracts number from filename.

The current blocked log looks like:
```
level=INFO msg="prompt blocked" file=084-path-validation-and-permissions.md reason="previous prompt not completed"
```

This repeats every 5 seconds with zero additional context. The operator has no idea if prompt 001, 042, or 083 is the blocker, or whether it's failed, executing, or missing entirely.
</context>

<requirements>
**Step 1 — Add `FindMissingCompleted` to `pkg/prompt/prompt.go`**

Add a new exported function next to `AllPreviousCompleted`:

```go
// FindMissingCompleted returns prompt numbers less than n that are NOT in the completed directory.
func FindMissingCompleted(ctx context.Context, completedDir string, n int) []int
```

- Reuse the same `os.ReadDir` + `extractNumberFromFilename` pattern from `AllPreviousCompleted`
- Return a sorted slice of missing numbers
- Return nil if all are completed

Add a corresponding method on the manager interface and implementation:
```go
FindMissingCompleted(ctx context.Context, n int) []int
```

**Step 2 — Add status lookup for missing prompts in `pkg/prompt/prompt.go`**

Add a new exported function:

```go
// FindPromptStatus looks up a prompt by number in the given directory and returns its frontmatter status.
// Returns empty string if not found.
func FindPromptStatus(ctx context.Context, dir string, number int) string
```

- Scan the directory for a file matching the number prefix
- Read just the frontmatter to extract the status field
- Used by the processor to report why a prompt is blocking (failed, executing, etc.)

Add a corresponding manager method:
```go
FindPromptStatusInProgress(ctx context.Context, number int) string
```

**Step 3 — Improve the blocked log message in `pkg/processor/processor.go`**

Replace the current blocked log at line ~385-393 with:

```go
if !p.promptManager.AllPreviousCompleted(ctx, pr.Number()) {
    missing := p.promptManager.FindMissingCompleted(ctx, pr.Number())
    // Build diagnostic details for each missing prompt
    var details []string
    for _, num := range missing {
        status := p.promptManager.FindPromptStatusInProgress(ctx, num)
        if status != "" {
            details = append(details, fmt.Sprintf("%03d(%s)", num, status))
        } else {
            details = append(details, fmt.Sprintf("%03d(not found)", num))
        }
    }
    slog.Info(
        "prompt blocked",
        "file", filepath.Base(pr.Path),
        "reason", "previous prompt not completed",
        "missing", strings.Join(details, ", "),
    )
    return nil
}
```

Now the log shows:
```
level=INFO msg="prompt blocked" file=084-path-validation-and-permissions.md reason="previous prompt not completed" missing="083(failed)"
```

**Step 4 — Deduplicate repeated blocked messages**

Add a field to the processor struct to track the last blocked message:
```go
lastBlockedMsg string
```

Only log the blocked message if it differs from the last one. Reset `lastBlockedMsg` when the processor successfully picks up a prompt or the queue changes.

This eliminates the spam of identical messages every 5 seconds while still logging when the situation changes.

**Step 5 — Tests**

- Test `FindMissingCompleted` with: all completed, some missing, none completed, empty dir
- Test `FindPromptStatus` with: found prompt, missing prompt
- Test the deduplication: same blocked state logs once, changed state logs again
- Follow existing test patterns in `pkg/prompt/prompt_test.go` and `pkg/processor/processor_test.go`

Run `make precommit` — must pass with no lint issues.
</requirements>
