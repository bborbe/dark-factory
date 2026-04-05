---
status: approved
created: "2026-04-04T15:30:00Z"
queued: "2026-04-05T22:23:55Z"
---

<summary>
- Daemon stops when a post-execution git failure occurs after a prompt was already moved to completed
- Uncommitted code changes from successful container runs are no longer silently lost
- Human intervention is required before the daemon processes additional prompts after a git infrastructure failure
- Existing behavior for normal prompt failures (file still in in-progress/) is unchanged
- New tests verify both the stop-on-missing and continue-on-present code paths
</summary>

<objective>
When `processPrompt` returns an error but the prompt file has already been moved to `completed/` (meaning the container succeeded but `git commit` or a later step failed), the daemon must stop instead of continuing. The current code calls `handlePromptFailure` which cannot find the file in `in-progress/` and then silently continues to the next prompt, risking loss of uncommitted code changes.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these files before making changes:
- `pkg/processor/processor.go` — focus on:
  - `processExistingQueued` method (~line 357) — the main loop that picks up and processes prompts
  - The error handling block after `p.processPrompt(ctx, pr)` (~line 416-427) — this is the fix site
  - `handlePromptFailure` method (~line 528) — loads prompt from path, will fail if file was moved
  - `moveToCompletedAndCommit` method (~line 1003) — moves prompt to completed/ then commits; if commit fails, the file is already in completed/
  - `handlePostExecution` method (~line 937) — calls `moveToCompletedAndCommit`
- `pkg/processor/processor_test.go` — existing test patterns using Ginkgo/Gomega
- `pkg/processor/processor_internal_test.go` — internal test patterns
</context>

<requirements>
1. **Add post-execution failure detection in `processExistingQueued`** in `pkg/processor/processor.go`.

In the `processExistingQueued` method, find the error handling block after the `p.processPrompt(ctx, pr)` call. The current code is:

```go
if err := p.processPrompt(ctx, pr); err != nil {
    if ctx.Err() != nil {
        slog.Info(
            "daemon shutting down, prompt stays executing",
            "file",
            filepath.Base(pr.Path),
        )
        return errors.Wrap(ctx, err, "prompt failed")
    }
    p.handlePromptFailure(ctx, pr.Path, err)
    continue // re-queued or permanently failed — process next prompt
}
```

Replace with:

```go
if err := p.processPrompt(ctx, pr); err != nil {
    if ctx.Err() != nil {
        slog.Info(
            "daemon shutting down, prompt stays executing",
            "file",
            filepath.Base(pr.Path),
        )
        return errors.Wrap(ctx, err, "prompt failed")
    }
    // If the prompt file no longer exists at its in-progress path, it was already
    // moved to completed/ — meaning the container succeeded but a post-execution
    // step (git commit, release, etc.) failed. This is a git infrastructure failure
    // that requires human intervention. Do NOT continue processing — uncommitted
    // code changes would be overwritten by the next prompt's git fetch/merge.
    if _, statErr := os.Stat(pr.Path); os.IsNotExist(statErr) {
        slog.Error(
            "post-execution failure, prompt already moved to completed — stopping daemon",
            "file", filepath.Base(pr.Path),
            "error", err,
        )
        return errors.Wrap(ctx, err, "post-execution git failure, manual intervention required")
    }
    p.handlePromptFailure(ctx, pr.Path, err)
    continue // re-queued or permanently failed — process next prompt
}
```

Ensure `os` is imported (it likely already is — verify).

2. **Add tests** in `pkg/processor/processor_test.go` or `pkg/processor/processor_internal_test.go` (whichever is more appropriate given the test patterns already in the file — the method under test is exported via the `Processor` interface if available, otherwise use internal tests).

Add a new `Describe("processExistingQueued post-execution failure")` block (or add `It` blocks to an existing describe for `processExistingQueued`) with two test cases:

**Test case A: prompt file gone after error — daemon stops**
- Set up a prompt with a valid `pr.Path` pointing to a file that does NOT exist on disk (simulating it was already moved to completed/)
- Mock `processPrompt` to return an error (or set up conditions so it returns an error after the file is gone)
- Call `processExistingQueued` (or the relevant method)
- Assert it returns a non-nil error containing "post-execution git failure"

**Test case B: prompt file still exists after error — handlePromptFailure called, loop continues**
- Set up a prompt with a valid `pr.Path` pointing to a file that DOES exist on disk
- Mock `processPrompt` to return an error
- Call `processExistingQueued`
- Assert `handlePromptFailure` was invoked (the prompt file gets re-queued or marked failed)
- Assert the method does NOT return an error (it continues the loop)

Follow the existing test patterns in the file — use the same mock setup, test helpers, and assertion style already established. Study how other `processExistingQueued` tests set up the prompt queue and mock dependencies.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrap(ctx, err, "message")` — never `fmt.Errorf`
- Follow existing test patterns: Ginkgo v2 / Gomega
- Do not change the signature of `processPrompt`, `handlePromptFailure`, or `moveToCompletedAndCommit`
- The fix must be in the `processExistingQueued` method only — do not change post-execution logic
</constraints>

<verification>
```bash
# Verify the os.IsNotExist check is present
grep -n 'os.IsNotExist' pkg/processor/processor.go

# Verify the return on post-execution failure
grep -A2 'post-execution git failure' pkg/processor/processor.go

# Run full precommit
make precommit
```
All must pass with no errors.
</verification>
