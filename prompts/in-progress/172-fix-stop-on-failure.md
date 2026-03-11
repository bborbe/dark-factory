---
status: executing
container: dark-factory-172-fix-stop-on-failure
dark-factory-version: v0.44.0
created: "2026-03-11T09:50:00Z"
queued: "2026-03-11T15:34:56Z"
started: "2026-03-11T16:45:29Z"
---
<summary>
- One-shot mode stops and exits with an error when a prompt fails, instead of resetting and retrying forever
- Daemon mode leaves a failed prompt in failed state instead of auto-resetting it on startup or retry
- Failed prompts require explicit user action to re-enter the queue — no automatic retry
- Previously failed prompts are preserved across restarts instead of being silently reset
- Existing tests still pass; new tests cover the changed failure behavior in both modes
</summary>

<objective>
Fix the infinite retry cycle: when a prompt fails, dark-factory must stop processing and surface the error rather than silently resetting the prompt and looping. One-shot mode (`dark-factory run`) must exit non-zero. Daemon mode must leave the prompt in `failed` state and wait for manual intervention via `dark-factory prompt retry`.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these files before making any changes:
- `pkg/processor/processor.go` — `Process`, `ProcessQueue`, `processExistingQueued`, `handlePromptFailure` — understand the full call chain
- `pkg/processor/processor_test.go` — existing test patterns (Ginkgo v2 / Gomega)
- `pkg/processor/processor_internal_test.go` — if it exists, check what it covers
- `pkg/prompt/prompt.go` — `Manager` interface, `ResetFailed` method signature
- `mocks/processor.go` — generated mock for `Processor` interface
- `mocks/prompt-manager.go` — generated mock for `Manager` interface
</context>

<requirements>
**Step 1: Remove `ResetFailed` from `ProcessQueue` (one-shot mode)**

In `pkg/processor/processor.go`, inside `ProcessQueue`, remove the `ResetFailed` startup call:

```go
// REMOVE this block from ProcessQueue:
if err := p.promptManager.ResetFailed(ctx); err != nil {
    return errors.Wrap(ctx, err, "reset failed prompts")
}
```

Rationale: in one-shot mode, any `failed` prompt in the queue was failed by a previous run. The user must explicitly run `dark-factory prompt retry` to re-queue it. Auto-resetting on startup causes the infinite retry cycle.

**Step 2: Remove `ResetFailed` from `Process` (daemon mode)**

In `pkg/processor/processor.go`, inside `Process`, remove the `ResetFailed` startup call:

```go
// REMOVE this block from Process:
if err := p.promptManager.ResetFailed(ctx); err != nil {
    return errors.Wrap(ctx, err, "reset failed prompts")
}
```

Rationale: daemon mode restarts should not silently retry previously failed prompts. The failed state is intentional — it signals the user to investigate.

**Step 3: Stop processing on failure in one-shot mode**

The `processExistingQueued` method currently calls `p.handlePromptFailure` and returns `nil` on failure, which lets the loop continue (or the caller returns `nil` to the user):

```go
// CURRENT (wrong for one-shot):
if err := p.processPrompt(ctx, pr); err != nil {
    p.handlePromptFailure(ctx, pr.Path, err)
    return nil // failed — wait for watcher signal or periodic scan
}
```

`processExistingQueued` cannot know whether it's running in one-shot or daemon mode, so the cleanest fix is to return the error from `processExistingQueued` and let the callers decide:

Change the failure block inside `processExistingQueued` to return the error:

```go
// NEW:
if err := p.processPrompt(ctx, pr); err != nil {
    p.handlePromptFailure(ctx, pr.Path, err)
    return errors.Wrap(ctx, err, "prompt failed")
}
```

**Step 4: Daemon mode ignores the returned error from `processExistingQueued`**

Inside `Process` (daemon mode), the three call-sites of `processExistingQueued` must be updated so that a prompt failure does NOT stop the daemon — it just leaves the prompt in `failed` state (which `handlePromptFailure` already does) and waits for the next event:

```go
// In Process, wherever processExistingQueued is called, change:
if err := p.processExistingQueued(ctx); err != nil {
    return errors.Wrap(ctx, err, "process queued prompts")
}

// To:
if err := p.processExistingQueued(ctx); err != nil {
    slog.Warn("prompt failed; queue blocked until manual retry", "error", err)
    // do NOT return — daemon continues running
}
```

Apply this to all three call-sites inside the `Process` for-select loop:
1. The `case <-p.ready:` branch call to `processExistingQueued`
2. The `case <-ticker.C:` branch call to `processExistingQueued`

Also apply to the initial `processExistingQueued` call in `Process` (before the for-select loop):
```go
// Initial scan in Process — also non-fatal:
if err := p.processExistingQueued(ctx); err != nil {
    slog.Warn("prompt failed on startup scan; queue blocked until manual retry", "error", err)
    // do NOT return — daemon continues running
}
```

**Step 5: One-shot mode propagates the error**

Inside `ProcessQueue`, the call to `processExistingQueued` already propagates the error (it uses `return errors.Wrap(...)`). After Step 3, this will now correctly surface prompt failures:

```go
// In ProcessQueue — keep as-is (already returns error):
if err := p.processExistingQueued(ctx); err != nil {
    return errors.Wrap(ctx, err, "process existing queued prompts")
}
```

No change needed here — just verify the existing error propagation is intact after Step 3.

**Step 6: Skip `failed` prompts in `processExistingQueued` for daemon mode**

After Step 3, daemon mode no longer resets failed prompts on startup, but `ListQueued` may or may not include `failed`-status prompts. Check what `ListQueued` returns for failed prompts:

In `pkg/prompt/prompt.go`, find `ListQueued` (or the underlying `listQueued` function). Identify which statuses it returns.

If `ListQueued` does NOT already filter out `failed`-status prompts, update `processExistingQueued` to skip them explicitly:

```go
// After listing queued prompts, skip any that are in failed state:
if pr.Status == prompt.FailedPromptStatus {
    slog.Debug("skipping failed prompt (requires manual retry)", "file", filepath.Base(pr.Path))
    continue
}
```

Add this check after the `autoSetQueuedStatus` call and before the `shouldSkipPrompt` call. This prevents daemon mode from picking up failed prompts during periodic scans.

If `ListQueued` already filters out failed prompts, skip this step (add a comment explaining it's already handled).

**Step 7: Tests**

Add tests to `pkg/processor/processor_test.go` using the existing Ginkgo v2 / Gomega patterns:

1. **One-shot: failed prompt causes non-nil return from `ProcessQueue`**
   - Set up a processor with a prompt that causes `executor.Execute` to return an error
   - Call `ProcessQueue(ctx)`
   - Assert the returned error is non-nil

2. **Daemon: failed prompt does NOT stop `Process`**
   - This is harder to test directly because `Process` blocks. Test the building block instead:
   - Verify that after a prompt failure, the failed prompt's status is `failed` (via `handlePromptFailure`)
   - Optionally: verify `processExistingQueued` returns a non-nil error when `processPrompt` fails

3. **`ProcessQueue` does NOT call `ResetFailed` on startup**
   - Create a mock `Manager` where `ResetFailed` is instrumented
   - Call `ProcessQueue(ctx)`
   - Assert `ResetFailed` was NOT called (call count == 0)

4. **`Process` does NOT call `ResetFailed` on startup**
   - Same as above but cancel ctx immediately to stop the daemon loop after the startup scan
   - Assert `ResetFailed` was NOT called

Use existing mock patterns from `mocks/prompt-manager.go` (counterfeiter-generated). Follow the existing test file style — look at how `processPrompt` failures are tested today, if at all.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- `dark-factory prompt retry` (which calls `ResetFailed` via `CreateRequeueCommand`) must still work — do not remove `ResetFailed` from `prompt.Manager` interface or its implementation, only remove the call-sites in `Process` and `ProcessQueue`
- Daemon mode must NOT exit on prompt failure — it must log a warning and stay alive
- One-shot mode MUST exit non-zero (return error) when any prompt fails
- `failed`-status prompts must not be silently re-queued anywhere in the processing loop
- Do not change `handlePromptFailure` behavior — it already marks the prompt as `failed` correctly
- Follow existing error wrapping: `errors.Wrap(ctx, err, "message")`
- All existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
```bash
# ResetFailed removed from Process and ProcessQueue
grep -n "ResetFailed" /Users/bborbe/Documents/workspaces/dark-factory/pkg/processor/processor.go
# Expected: zero results (the call-sites are gone)

# processExistingQueued returns error on failure
grep -n "return errors.Wrap.*prompt failed" /Users/bborbe/Documents/workspaces/dark-factory/pkg/processor/processor.go

# Daemon call-sites use slog.Warn instead of returning error
grep -n "queue blocked until manual retry" /Users/bborbe/Documents/workspaces/dark-factory/pkg/processor/processor.go

make precommit
```
Must pass with no errors.
</verification>
