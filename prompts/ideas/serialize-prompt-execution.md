# Serialize prompt execution to prevent concurrent git conflicts

## Goal

When multiple prompt files appear in `prompts/` simultaneously, the factory must
process them one at a time. Currently `handleFileEvent` fires independently for
each fsnotify event, spawning concurrent Docker containers that race on git
operations.

## Current Behavior

1. User drops `foo.md` and `bar.md` into `prompts/`
2. fsnotify fires two events
3. `handleFileEvent` runs twice concurrently (via debounce timers)
4. Two Docker containers start simultaneously
5. Both try to `git add / commit / tag / push` — race condition

Note: `processExistingQueued` already processes sequentially in a loop. The
problem is only in the watcher path (`handleFileEvent`).

## Expected Behavior

Only one prompt executes at a time. If a prompt has `status: executing`, no new
prompt should start. The watcher event should be skipped — the prompt will be
picked up after the current one finishes (because `processExistingQueued` loops
back to check for more queued prompts after each completion).

## Implementation

In `handleFileEvent`, before calling `processPrompt`, check if any prompt in the
directory already has `status: executing`:

```go
func (f *factory) handleFileEvent(ctx context.Context, filePath string) {
    // Skip if another prompt is currently executing
    if prompt.HasExecuting(ctx, f.promptsDir) {
        return
    }
    // ... existing logic ...
}
```

Add `HasExecuting` to `pkg/prompt/`:

```go
// HasExecuting returns true if any prompt in dir has status "executing".
func HasExecuting(ctx context.Context, dir string) bool {
    entries, err := os.ReadDir(dir)
    if err != nil {
        return false
    }
    for _, entry := range entries {
        if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
            continue
        }
        fm, err := ReadFrontmatter(ctx, filepath.Join(dir, entry.Name()))
        if err != nil {
            continue
        }
        if fm.Status == "executing" {
            return true
        }
    }
    return false
}
```

After `processPrompt` completes (success or failure), call `processExistingQueued`
to drain any remaining queued prompts that arrived during execution. This is
already the behavior on startup — just reuse it after watcher events too.

## Constraints

- Run `make precommit` for validation only — do NOT commit, tag, or push (dark-factory handles all git operations)
