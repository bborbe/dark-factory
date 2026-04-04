---
status: draft
spec: ["044"]
created: "2026-04-04T20:50:00Z"
---

<summary>
- Failed prompts (any reason, including timeout) are auto-retried up to `autoRetryLimit` times when configured
- Retry attempts and failure reasons are tracked in prompt metadata between retries
- After exhausting retries, the prompt is marked as permanently failed and the daemon continues
- The daemon does NOT stop on a `permanently_failed` prompt; it skips it and processes the next one
- `dark-factory prompt requeue <file>` and `dark-factory prompt retry` reset `retryCount` to 0 in addition to re-queuing
- `permanently_failed` prompts are re-queued by `retry` (same as `failed`), refreshing the retry budget
- When `autoRetryLimit` is 0 (default), prompts fail immediately without retry — same as current behavior
- A notification fires when a prompt is permanently failed (new event type `prompt_permanently_failed`)
- All existing status consumers (list, requeue, prompt_complete) handle `permanently_failed` consistently
- The daemon in one-shot mode (`ProcessQueue`) stops and returns error on first failure (no retry in one-shot)
</summary>

<objective>
Wire auto-retry and permanent failure into the processor: after a prompt fails, check `autoRetryLimit`, re-queue if budget remains, otherwise mark permanently failed and let the daemon continue. When `autoRetryLimit` is 0 (default), behavior is unchanged. Update requeue/retry commands to reset the retry counter so manual re-queuing always gets a fresh budget.
</objective>

<context>
Read CLAUDE.md for project conventions.

**This prompt builds on Prompts 1 and 2 (spec-044-model, spec-044-executor-timeout)**: `PermanentlyFailedPromptStatus`, `MarkPermanentlyFailed`, `SetLastFailReason`, `LastFailReason` frontmatter field, and the new config fields `AutoRetry`/`AutoRetryLimit` all exist before this prompt runs.

Read these files before making any changes:
- `pkg/processor/processor.go` — `NewProcessor`, `processor` struct, `Process`, `ProcessQueue`, `processExistingQueued`, `handlePromptFailure`, `shouldSkipPrompt`
- `pkg/prompt/prompt.go` — `MarkFailed`, `MarkApproved`, `MarkPermanentlyFailed` (from Prompt 1), `RetryCount`, `Frontmatter.RetryCount`
- `pkg/cmd/requeue.go` — `requeueFile`, `requeueFailed` — currently call `MarkApproved()` without resetting `retryCount`
- `pkg/factory/factory.go` — `CreateProcessor` signature, where it is called
- `pkg/notifier/notifier.go` — `Event` struct and existing event types
- `pkg/cmd/list.go` — status filtering for `--failed` flag
- `mocks/processor.go`, `mocks/prompt-manager.go` — counterfeiter-generated mocks used in tests
</context>

<requirements>
**Step 1: Add `autoRetry` and `autoRetryLimit` fields to `processor` struct in `pkg/processor/processor.go`**

Add to the struct:
```go
autoRetry      bool
autoRetryLimit int
```

Update `NewProcessor` to accept them (add as the last two parameters before or after `containerLock`/`containerChecker` — follow alphabetical or logical grouping consistent with the existing parameter list):
```go
func NewProcessor(
    // ... existing params ...
    autoRetry      bool,
    autoRetryLimit int,
) Processor {
    return &processor{
        // ... existing fields ...
        autoRetry:      autoRetry,
        autoRetryLimit: autoRetryLimit,
    }
}
```

**Step 2: Rewrite `handlePromptFailure` in `pkg/processor/processor.go`**

The current implementation unconditionally calls `pf.MarkFailed()`. Replace it with retry-aware logic:

```go
// handlePromptFailure decides whether to retry, permanently fail, or fail the prompt.
// Re-queuing increments retryCount and calls MarkApproved; exhausted retries call MarkPermanentlyFailed.
func (p *processor) handlePromptFailure(ctx context.Context, path string, err error) {
    slog.Error("prompt failed", "file", filepath.Base(path), "error", err)

    pf, loadErr := p.promptManager.Load(ctx, path)
    if loadErr != nil {
        slog.Error("failed to load prompt for failure handling", "error", loadErr)
        return
    }

    reason := err.Error()
    pf.SetLastFailReason(reason)

    if p.autoRetry && p.autoRetryLimit > 0 && pf.RetryCount() < p.autoRetryLimit {
        // Re-queue with incremented retry count
        pf.Frontmatter.RetryCount++
        pf.MarkApproved()
        if saveErr := pf.Save(ctx); saveErr != nil {
            slog.Error("failed to save prompt for retry", "error", saveErr)
            // Treat as permanent failure — fall through to MarkPermanentlyFailed
            pf.MarkPermanentlyFailed(reason)
            if saveErr2 := pf.Save(ctx); saveErr2 != nil {
                slog.Error("failed to save permanently failed prompt", "error", saveErr2)
            }
            p.notifyPermanentFailure(ctx, path, reason)
            return
        }
        slog.Info("prompt re-queued for retry",
            "file", filepath.Base(path),
            "retryCount", pf.RetryCount(),
            "autoRetryLimit", p.autoRetryLimit)
        return
    }

    if p.autoRetry {
        // Retries exhausted — permanently fail
        pf.MarkPermanentlyFailed(reason)
        if saveErr := pf.Save(ctx); saveErr != nil {
            slog.Error("failed to save permanently failed prompt", "error", saveErr)
        }
        p.notifyPermanentFailure(ctx, path, reason)
        return
    }

    // autoRetry disabled — standard failure
    pf.MarkFailed()
    if saveErr := pf.Save(ctx); saveErr != nil {
        slog.Error("failed to set failed status", "error", saveErr)
    }
    _ = p.notifier.Notify(ctx, notifier.Event{
        ProjectName: p.projectName,
        EventType:   "prompt_failed",
        PromptName:  filepath.Base(path),
    })
}

// notifyPermanentFailure fires a notification for a permanently failed prompt.
func (p *processor) notifyPermanentFailure(ctx context.Context, path string, reason string) {
    _ = p.notifier.Notify(ctx, notifier.Event{
        ProjectName: p.projectName,
        EventType:   "prompt_permanently_failed",
        PromptName:  filepath.Base(path),
    })
}
```

**Step 3: Fix `processExistingQueued` flow control for retry/permanent failure**

In `processExistingQueued` (line 351), after the `processPrompt` call (line 407), the current code is:
```go
if err := p.processPrompt(ctx, pr); err != nil {
    if ctx.Err() != nil {
        // ... shutdown case — return error (keep as-is)
    }
    p.handlePromptFailure(ctx, pr.Path, err)
    return errors.Wrap(ctx, err, "prompt failed")  // ← THIS MUST CHANGE
}
```

The `return` on line 417 exits the loop after ANY failure, which prevents the daemon from continuing to the next prompt after a re-queue or permanent failure. Replace the `return` with `continue` so the loop processes the next queued prompt:

```go
if err := p.processPrompt(ctx, pr); err != nil {
    if ctx.Err() != nil {
        slog.Info("daemon shutting down, prompt stays executing",
            "file", filepath.Base(pr.Path))
        return errors.Wrap(ctx, err, "prompt failed")
    }
    p.handlePromptFailure(ctx, pr.Path, err)
    continue // re-queued or permanently failed — process next prompt
}
```

This is critical: without this change, the daemon stops processing after the first failure even when auto-retry re-queues the prompt.

**Step 3b: Add `permanently_failed` to skip logic in `processExistingQueued`**

Find `shouldSkipPrompt` in `processor.go` (or the place inside `processExistingQueued` where failed prompts are skipped). Add `PermanentlyFailedPromptStatus` to the skip list next to the existing `FailedPromptStatus` skip:
```go
if pr.Status == prompt.PermanentlyFailedPromptStatus {
    slog.Debug("skipping permanently failed prompt", "file", filepath.Base(pr.Path))
    p.skippedPrompts[filepath.Base(pr.Path)] = modTime
    continue
}
```

**Step 4: Update `pkg/factory/factory.go` — thread `autoRetry` and `autoRetryLimit` into `CreateProcessor`**

Add two parameters to `CreateProcessor` (line 467 in `pkg/factory/factory.go`):
```go
func CreateProcessor(
    // ... existing params ...
    autoRetry      bool,   // NEW
    autoRetryLimit int,    // NEW
) processor.Processor {
```

Pass them through to `processor.NewProcessor`.

`CreateProcessor` is called from two locations — update both:
- `CreateRunner` (line 264): pass `cfg.AutoRetry, cfg.AutoRetryLimit`
- `CreateOneShotRunner` (line 331): pass `cfg.AutoRetry, cfg.AutoRetryLimit`

**Step 5: Update `pkg/cmd/requeue.go` — reset `retryCount` on re-queue**

In `requeueFile`: after calling `pf.MarkApproved()`, reset the retry counter:
```go
pf.MarkApproved()
pf.Frontmatter.RetryCount = 0  // reset auto-retry budget on manual re-queue
```

In `requeueFailed`: update the status check to also include `PermanentlyFailedPromptStatus` and reset `retryCount`:
```go
if pf.Frontmatter.Status == string(prompt.FailedPromptStatus) ||
    pf.Frontmatter.Status == string(prompt.PermanentlyFailedPromptStatus) {
    pf.MarkApproved()
    pf.Frontmatter.RetryCount = 0  // reset auto-retry budget on manual re-queue
    if err := pf.Save(ctx); err != nil {
        return errors.Wrap(ctx, err, "save prompt")
    }
    fmt.Printf("requeued: %s\n", entry.Name())
    requeued++
}
```

**Step 6: Update `pkg/cmd/list.go` — include `permanently_failed` in failed filter**

Find where `prompt.FailedPromptStatus` is used for filtering (around line 105). Add `PermanentlyFailedPromptStatus` to the same filter so `dark-factory prompt list --failed` shows both:

Look at the `filterPromptsByStatus` call or equivalent and include the new status.

**Step 7: Add tests**

Add tests in `pkg/processor/processor_test.go` using the existing Ginkgo v2 / Gomega patterns. Read the existing test file to understand how `processor` is constructed in tests (what mocks are used).

1. **Auto-retry: first failure re-queues with incremented retryCount**
   - Set `autoRetry: true`, `autoRetryLimit: 3`
   - Simulate a prompt failure (`executor.Execute` returns error)
   - Assert prompt status is `approved` (re-queued) and `retryCount` is 1

2. **Auto-retry: exhausted retries → permanently_failed**
   - Set `autoRetry: true`, `autoRetryLimit: 2`
   - Load a prompt with `retryCount: 2` already set
   - Trigger `handlePromptFailure`
   - Assert status is `permanently_failed` and notifier received `prompt_permanently_failed` event

3. **Auto-retry disabled → standard failed status**
   - Set `autoRetry: false`
   - Trigger `handlePromptFailure`
   - Assert status is `failed` and notifier received `prompt_failed` event

4. **`permanently_failed` prompt is skipped in `processExistingQueued`**
   - List contains a `permanently_failed` prompt
   - Assert `Execute` is NOT called

Add tests in `pkg/cmd/requeue_test.go`:

5. **requeueFile resets retryCount**
   - Create a prompt file with `retryCount: 3, status: failed`
   - Call `requeueFile`
   - Assert resulting frontmatter has `retryCount: 0` and `status: approved`

6. **requeueFailed includes permanently_failed prompts**
   - Create a prompt file with `status: permanently_failed`
   - Call `requeueFailed`
   - Assert it is re-queued with `retryCount: 0`
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- The `failed` status must still exist and work as before when `autoRetry` is false
- Daemon mode (`Process`) must NOT stop when a prompt becomes `permanently_failed` — the warn-and-continue pattern from `processExistingQueued` already handles this; `handlePromptFailure` must not return an error
- One-shot mode (`ProcessQueue`) uses the same `processExistingQueued` loop. With the `continue` fix in Step 3, re-queued prompts are picked up on the next loop iteration. The retry budget limits total attempts regardless of mode.
- `autoRetry: false` must behave identically to the current behavior (no code-path changes for the disabled case beyond the existing `handlePromptFailure` logic)
- `autoRetryLimit: 0` with `autoRetry: true` means no auto-retries (same as `autoRetry: false`) — the condition `autoRetryLimit > 0 && retryCount < autoRetryLimit` already handles this
- Use `errors.Wrap(ctx, err, "message")` — never `fmt.Errorf`
- Counterfeiter mocks in `mocks/` must be regenerated if interface signatures change: run `go generate ./...` or manually update affected mock files. Check if `Processor` interface changes (it should not in this prompt)
- The `prompt_permanently_failed` event type is new — add it to the comment on the `Event` struct in `pkg/notifier/notifier.go`
</constraints>

<verification>
```bash
# Confirm auto-retry fields in processor struct
grep -n "autoRetry\|autoRetryLimit" pkg/processor/processor.go

# Confirm permanently_failed is skipped in processExistingQueued
grep -n "PermanentlyFailed\|permanently_failed" pkg/processor/processor.go

# Confirm retryCount reset in requeue
grep -n "RetryCount = 0\|retryCount.*0" pkg/cmd/requeue.go

# Confirm new event type in notifier
grep -n "permanently_failed" pkg/notifier/notifier.go

make precommit
```
Must pass with no errors.
</verification>
