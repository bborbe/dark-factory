---
status: queued
---

# Reset failed prompts to queued on startup

On startup, the processor should scan the queue directory and reset any prompts with `status: failed` back to `status: queued`. This allows the factory to retry failed prompts after a restart without manual intervention.

## Implementation

In `pkg/processor/processor.go`, add a method `resetFailedPrompts(ctx)` that:
1. Lists all `.md` files in `queueDir`
2. For each file, read frontmatter
3. If `status: failed`, call `prompt.SetStatus(ctx, path, prompt.StatusQueued)`
4. Log: `"reset failed prompt %s to queued"`

Call this method once at the start of `Process()`, before entering the main loop.

## Verification

Run `make precommit` — must pass.
