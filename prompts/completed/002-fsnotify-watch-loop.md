---
status: failed
---

# Add fsnotify-based directory watching to main loop

## Goal

Replace the current scan-once-and-exit behavior with a persistent daemon that watches `prompts/` using filesystem events. dark-factory should run forever, reacting to file creates/modifications, and only exit on failure or signal.

## Current Behavior (broken)

```
scan prompts/ → no queued → exit 0
```

## Expected Behavior

```
start → scan prompts/ for existing queued → process any found
      → watch prompts/ for new/modified files
      → on event: check if file has status: queued → process it
      → loop forever (until SIGINT/SIGTERM or failure)
```

## Implementation

### Add fsnotify dependency

```bash
go get github.com/fsnotify/fsnotify
```

### Update pkg/factory/factory.go

1. On startup: scan for any existing `status: queued` prompts and process them first
2. Then start watching `prompts/` directory using `fsnotify.NewWatcher()`
3. On `fsnotify.Write` or `fsnotify.Create` events for `.md` files:
   - Read the file, check frontmatter for `status: queued`
   - If queued: process it (same as current loop body)
4. Handle `SIGINT`/`SIGTERM` gracefully — close watcher, exit 0
5. On watcher error: log and exit 1
6. Debounce: editors may fire multiple events for one save. Add a short debounce (500ms) — after receiving an event, wait 500ms for more events on the same file before processing.

### Logging

Add basic logging so the user knows what's happening:

```
dark-factory: watching prompts/ for queued prompts...
dark-factory: found queued prompt: 002-feature.md
dark-factory: executing prompt: Add feature X
dark-factory: docker container exited with code 0
dark-factory: committed and tagged v0.1.1
dark-factory: moved 002-feature.md to completed/
dark-factory: watching prompts/ for queued prompts...
```

Use `log.Printf` with `dark-factory:` prefix. No structured logging library needed for MVP.

### Signal Handling

```go
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()
```

Pass `ctx` through to the factory. When context is cancelled, shut down cleanly.

## Testing

### Integration Test (pkg/factory/)

Write a Ginkgo v2 integration test that validates the full watch-and-process flow:

1. Create a temp directory as fake project root
2. Create a `prompts/` and `prompts/completed/` subdirectory
3. Initialize a git repo in the temp dir (`git init`, `git add`, `git commit`)
4. Create a CHANGELOG.md with `## Unreleased` section
5. Start the factory in a goroutine (with a context you can cancel)
6. **Mock the executor** — don't actually run Docker. Replace the executor with a fake that returns exit code 0.
7. Write a prompt file to `prompts/test-prompt.md` with `status: queued`
8. Wait (with timeout) for the file to appear in `prompts/completed/`
9. Verify:
   - Prompt file moved to `prompts/completed/`
   - Prompt status changed to `completed`
   - Git has a new commit with tag
   - CHANGELOG.md updated with version
10. Cancel context, verify factory exits cleanly

**Important**: The executor must be injectable/mockable for this test. Extract an interface:

```go
type Executor interface {
    Execute(ctx context.Context, projectRoot string, promptContent string) error
}
```

The factory takes an Executor in its constructor. Production code passes the real Docker executor. Tests pass a fake.

### Unit Tests

- Test debounce logic if extracted to a helper
- Test signal handling (cancel context, verify clean shutdown)

## Constraints

- Only add `fsnotify/fsnotify` as new dependency
- Keep the executor interface minimal — just `Execute(ctx, root, content) error`
- No goroutine leaks — all goroutines must respect context cancellation
- Run `make precommit` before finishing to ensure everything passes
