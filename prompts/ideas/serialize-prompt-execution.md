# Serialize prompt execution to prevent concurrent git conflicts

## Goal

When multiple prompt files appear in `prompts/` simultaneously, the factory must
process them one at a time. Currently `handleFileEvent` fires independently for
each fsnotify event, spawning concurrent Docker containers that race on git
operations.

## Current Behavior

1. User drops `foo.md` and `bar.md` into `prompts/`
2. fsnotify fires two events
3. `handleFileEvent` runs twice concurrently
4. Two Docker containers start simultaneously
5. Both try to `git add / commit / tag / push` — race condition

## Expected Behavior

Only one prompt executes at a time. Additional prompts wait in a queue and are
processed after the current one finishes.

## Implementation

### Option A: Processing mutex (simplest)

Add a `sync.Mutex` to `factory` and lock it in `handleFileEvent` before calling
`processPrompt`:

```go
type factory struct {
    promptsDir string
    executor   executor.Executor
    processMu  sync.Mutex // serialize prompt execution
}

func (f *factory) handleFileEvent(ctx context.Context, filePath string) {
    f.processMu.Lock()
    defer f.processMu.Unlock()
    // ... existing logic ...
}
```

`processExistingQueued` already runs sequentially in a loop, so it only needs the
mutex if it could overlap with watcher events. Since `processExistingQueued` runs
before the watcher starts, this is safe. But for extra safety, lock there too.

### Option B: Channel-based queue

Send file paths to a buffered channel, drain with a single goroutine:

```go
type factory struct {
    promptsDir string
    executor   executor.Executor
    queue      chan string
}
```

More complex but allows ordered processing.

## Recommended: Option A

Mutex is simplest, no ordering guarantees needed, and prevents the race.

## Constraints

- Run `make precommit` for validation only — do NOT commit, tag, or push (dark-factory handles all git operations)
