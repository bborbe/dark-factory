---
status: completed
summary: Removed permanently_failed prompt status — exhausted retries now mark prompts failed, notifyPermanentFailure replaced with notifyFailed, all references removed from source and tests
container: dark-factory-256-remove-permanently-failed-status
dark-factory-version: v0.102.1-dirty
created: "2026-04-06T08:15:16Z"
queued: "2026-04-06T08:38:28Z"
started: "2026-04-06T08:54:28Z"
completed: "2026-04-06T09:09:33Z"
---

<summary>
- The `permanently_failed` prompt status is removed entirely — all failure cases use `failed`
- When auto-retries are exhausted, prompts are marked `failed` (not `permanently_failed`) with lastFailReason and retryCount preserved for diagnostics
- The daemon continues running after exhausted retries — the prompt stays failed, user can requeue to reset retryCount and try again
- The `prompt_permanently_failed` notification event is removed — all failures fire `prompt_failed`
- Requeue and list commands work with only `failed` status (no dual-status checks needed)
- Manual completion of failed prompts still works (only checks `failed`, not `permanently_failed`)
- Tests are updated to reflect the simplified failure model
- Documentation no longer references `permanently_failed`
</summary>

<objective>
Remove the `permanently_failed` prompt status to simplify the failure model. When retries are exhausted, the prompt is simply marked `failed` with `lastFailReason` and `retryCount` preserved. The user can `requeue` to reset retryCount and try again. This eliminates a status that adds complexity without meaningful benefit.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these files before making any changes:
- `pkg/prompt/prompt.go` — `PermanentlyFailedPromptStatus` constant (~line 73-74), `AvailablePromptStatuses` (~line 78-89), `MarkPermanentlyFailed` method (~line 373-378), `ListQueued` skip list (~line 694-699)
- `pkg/processor/processor.go` — `handlePromptFailure` method (~line 563-618), `notifyPermanentFailure` helper (~line 620-627), `autoSetQueuedStatus` switch (~line 493-501)
- `pkg/cmd/requeue.go` — `requeueFailed` method (~line 91-119)
- `pkg/cmd/list.go` — `failedOnly` filter (~line 104-109)
- `pkg/cmd/prompt_complete.go` — allowed source statuses switch (~line 79-92)
- `pkg/notifier/notifier.go` — `Event` struct comment (~line 12)
- `docs/configuration.md` — autoRetryLimit docs (~line 228, 236)
- `pkg/prompt/prompt_test.go` — tests for `PermanentlyFailedPromptStatus` and `MarkPermanentlyFailed` (~line 2675-2695)
- `pkg/processor/processor_test.go` — "exhausted retries" test (~line 6405), "skips permanently_failed" test (~line 6491)
- `pkg/cmd/requeue_test.go` — "permanently_failed prompts" test (~line 184)
</context>

<requirements>
1. **Remove `PermanentlyFailedPromptStatus` from `pkg/prompt/prompt.go`**

   - Delete the `PermanentlyFailedPromptStatus` constant (line with `PermanentlyFailedPromptStatus PromptStatus = "permanently_failed"`) and its preceding comment line.
   - Remove `PermanentlyFailedPromptStatus` from the `AvailablePromptStatuses` slice.
   - Delete the entire `MarkPermanentlyFailed` method (the method that sets status to `permanently_failed`, sets `Completed`, and sets `LastFailReason`).
   - Keep `SetLastFailReason` method — still needed.
   - Keep `LastFailReason` in the `Frontmatter` struct — still needed for diagnostics.
   - In `ListQueued`, remove the `PermanentlyFailedPromptStatus` line from the status skip block. `FailedPromptStatus` already covers all failure cases.

2. **Rewrite `handlePromptFailure` in `pkg/processor/processor.go`**

   Replace the current three-branch logic (retry / permanently-fail / standard-fail) with two branches (retry / fail):

   ```go
   // handlePromptFailure decides whether to retry or fail the prompt.
   // Re-queuing increments retryCount and calls MarkApproved; exhausted retries call MarkFailed.
   func (p *processor) handlePromptFailure(ctx context.Context, path string, err error) {
       slog.Error("prompt failed", "file", filepath.Base(path), "error", err)

       pf, loadErr := p.promptManager.Load(ctx, path)
       if loadErr != nil {
           slog.Error("failed to load prompt for failure handling", "error", loadErr)
           return
       }

       reason := err.Error()
       pf.SetLastFailReason(reason)

       if p.autoRetryLimit > 0 && pf.RetryCount() < p.autoRetryLimit {
           // Re-queue with incremented retry count
           pf.Frontmatter.RetryCount++
           pf.MarkApproved()
           if saveErr := pf.Save(ctx); saveErr != nil {
               slog.Error("failed to save prompt for retry", "error", saveErr)
               // Fall through to MarkFailed
               pf.MarkFailed()
               if saveErr2 := pf.Save(ctx); saveErr2 != nil {
                   slog.Error("failed to save failed prompt", "error", saveErr2)
               }
               p.notifyFailed(ctx, path)
               return
           }
           slog.Info("prompt re-queued for retry",
               "file", filepath.Base(path),
               "retryCount", pf.RetryCount(),
               "autoRetryLimit", p.autoRetryLimit)
           return
       }

       // Retries exhausted or autoRetryLimit == 0 — mark failed
       pf.MarkFailed()
       if saveErr := pf.Save(ctx); saveErr != nil {
           slog.Error("failed to set failed status", "error", saveErr)
       }
       p.notifyFailed(ctx, path)
   }

   // notifyFailed fires a notification for a failed prompt.
   func (p *processor) notifyFailed(ctx context.Context, path string) {
       _ = p.notifier.Notify(ctx, notifier.Event{
           ProjectName: p.projectName,
           EventType:   "prompt_failed",
           PromptName:  filepath.Base(path),
       })
   }
   ```

   Key changes vs current code:
   - The save-error fallback (line ~583-589) now calls `pf.MarkFailed()` instead of `pf.MarkPermanentlyFailed(reason)` and calls `p.notifyFailed` instead of `p.notifyPermanentFailure`.
   - The "retries exhausted" branch (line ~598-606) is merged with the "autoRetryLimit == 0" branch (line ~608-617) into one unified `pf.MarkFailed()` path.
   - The `notifyPermanentFailure` helper method (line ~620-627) is replaced with `notifyFailed` which fires `"prompt_failed"` instead of `"prompt_permanently_failed"`.
   - Delete the old `notifyPermanentFailure` method entirely.

3. **Remove `PermanentlyFailedPromptStatus` from `autoSetQueuedStatus` in `pkg/processor/processor.go`**

   In the `autoSetQueuedStatus` method, remove `prompt.PermanentlyFailedPromptStatus` from the switch-case that lists statuses that should NOT be overridden. `prompt.FailedPromptStatus` already covers failed prompts.

4. **Simplify `requeueFailed` in `pkg/cmd/requeue.go`**

   In `requeueFailed`, remove the `PermanentlyFailedPromptStatus` check. Change:
   ```go
   if pf.Frontmatter.Status == string(prompt.FailedPromptStatus) ||
       pf.Frontmatter.Status == string(prompt.PermanentlyFailedPromptStatus) {
   ```
   To:
   ```go
   if pf.Frontmatter.Status == string(prompt.FailedPromptStatus) {
   ```

5. **Simplify failed filter in `pkg/cmd/list.go`**

   In the `failedOnly` case, remove `PermanentlyFailedPromptStatus`. Change:
   ```go
   case failedOnly:
       entries = filterPromptsByStatus(
           entries,
           string(prompt.FailedPromptStatus),
           string(prompt.PermanentlyFailedPromptStatus),
       )
   ```
   To:
   ```go
   case failedOnly:
       entries = filterPromptsByStatus(
           entries,
           string(prompt.FailedPromptStatus),
       )
   ```

6. **Remove `PermanentlyFailedPromptStatus` from `pkg/cmd/prompt_complete.go`**

   In the switch-case for allowed source statuses, remove `prompt.PermanentlyFailedPromptStatus`. Change:
   ```go
   case prompt.PendingVerificationPromptStatus,
       prompt.FailedPromptStatus,
       prompt.PermanentlyFailedPromptStatus,
       prompt.InReviewPromptStatus,
       prompt.ExecutingPromptStatus:
   ```
   To:
   ```go
   case prompt.PendingVerificationPromptStatus,
       prompt.FailedPromptStatus,
       prompt.InReviewPromptStatus,
       prompt.ExecutingPromptStatus:
   ```

7. **Update `pkg/notifier/notifier.go`**

   In the `Event` struct comment, remove `"prompt_permanently_failed"` from the event type list. Change:
   ```go
   EventType   string // "prompt_failed", "prompt_permanently_failed", "prompt_partial", "spec_verifying", "review_limit", "stuck_container"
   ```
   To:
   ```go
   EventType   string // "prompt_failed", "prompt_partial", "spec_verifying", "review_limit", "stuck_container"
   ```

8. **Update tests in `pkg/prompt/prompt_test.go`**

   - Delete the `Describe("PermanentlyFailedPromptStatus")` block (~line 2675-2679) that validates the status.
   - Delete the `Describe("PromptFile.MarkPermanentlyFailed")` block (~line 2682-2695) that tests `MarkPermanentlyFailed`.

9. **Update tests in `pkg/processor/processor_test.go`**

   - In the "marks permanently_failed when retries exhausted" test (~line 6405): rename test to "marks failed when retries exhausted". Change assertions from `permanently_failed` to `failed` and from `prompt_permanently_failed` event to `prompt_failed` event. Specifically:
     - Change the assertion `Expect(string(content)).To(ContainSubstring("status: permanently_failed"))` to `Expect(string(content)).To(ContainSubstring("status: failed"))`.
     - Change the event check from `evt.EventType == "prompt_permanently_failed"` to `evt.EventType == "prompt_failed"`.
     - Update the failure message from `"expected prompt_permanently_failed notification"` to `"expected prompt_failed notification"`.
   - In the "skips permanently_failed prompts" test (~line 6491): remove or rewrite this test. Since `permanently_failed` no longer exists, the test is obsolete. The "skips failed prompts" behavior is already covered by existing tests for `FailedPromptStatus`. Delete the entire test entry.

10. **Update tests in `pkg/cmd/requeue_test.go`**

    - In the "requeueFailed includes permanently_failed prompts and resets retryCount" test (~line 184): remove this test entirely. The `permanently_failed` status no longer exists, and requeue of `failed` prompts is already tested.

11. **Update `docs/configuration.md`**

    - At ~line 228, change "marking them `permanently_failed`" to "marking them `failed`".
    - At ~line 236, change "the prompt transitions to `permanently_failed` and stops being retried" to "the prompt transitions to `failed` and stops being retried automatically".
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass (after the test updates described above)
- The `failed` status must still work as before when `autoRetryLimit` is 0
- Daemon mode (`Process`) must NOT stop when retries are exhausted — the `continue` in `processExistingQueued` already handles this
- `autoRetryLimit: 0` (default) must behave identically to current behavior — standard `failed` status, no retry logic
- Use `errors.Wrap(ctx, err, "message")` — never `fmt.Errorf`
- Keep `SetLastFailReason`, `LastFailReason`, `RetryCount` — these are still needed for diagnostics
- Keep `retryCount` reset logic in `requeueFile` and `requeueFailed` — still needed so manual requeue refreshes the retry budget
- Do NOT change the `handlePromptFailure` function signature
- Do NOT change the `processExistingQueued` flow control (the `continue` after `handlePromptFailure`)
</constraints>

<verification>
```bash
# Verify permanently_failed is completely gone from non-test source
grep -rn 'PermanentlyFailed\|permanently_failed\|MarkPermanentlyFailed\|notifyPermanentFailure' pkg/ --include='*.go' | grep -v '_test.go'
# Expected: no output

# Verify permanently_failed is gone from test files
grep -rn 'PermanentlyFailed\|permanently_failed\|MarkPermanentlyFailed\|notifyPermanentFailure\|prompt_permanently_failed' pkg/ --include='*_test.go'
# Expected: no output

# Verify docs updated
grep -n 'permanently_failed' docs/configuration.md
# Expected: no output

# Verify MarkFailed still exists
grep -n 'func.*MarkFailed' pkg/prompt/prompt.go
# Expected: MarkFailed method present

# Verify notifyFailed helper exists
grep -n 'func.*notifyFailed' pkg/processor/processor.go
# Expected: notifyFailed method present

# Run full precommit
make precommit
```
All must pass with no errors.
</verification>
