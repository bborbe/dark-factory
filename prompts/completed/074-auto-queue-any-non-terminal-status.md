---
status: completed
summary: Updated autoSetQueuedStatus to handle any non-terminal status using switch statement
container: dark-factory-074-auto-queue-any-non-terminal-status
dark-factory-version: v0.17.2
created: "2026-03-05T18:43:45Z"
queued: "2026-03-05T18:43:45Z"
started: "2026-03-05T18:43:45Z"
completed: "2026-03-05T18:49:49Z"
---

# Auto-queue any non-terminal status in queue folder

## Problem

`autoSetQueuedStatus` only handles empty and `created` statuses. But specs use `status: approved` which also gets rejected by `ValidateForExecution`. The queue folder should be the source of truth — any file there should be auto-queued regardless of current status.

## Current code (too narrow)

```go
if pr.Status == "" || pr.Status == prompt.Status("created") {
```

## Fix

Change the condition to auto-set queued for any status that isn't already a processing/terminal state:

```go
func (p *processor) autoSetQueuedStatus(ctx context.Context, pr *prompt.Prompt) error {
	switch pr.Status {
	case prompt.StatusQueued, prompt.StatusExecuting, prompt.StatusCompleted, prompt.StatusFailed:
		// Already in a valid processing state — don't override
		return nil
	}
	// Any other status (empty, "created", "approved", "draft", etc.) → auto-set to queued
	// ...
}
```

This way the queue folder is truly the source of truth. Only skip auto-setting if the prompt is already in a valid processing lifecycle state.

## Verification

- `make precommit` must pass
- Test: prompt with `status: approved` in queue/ gets auto-set to `queued`
