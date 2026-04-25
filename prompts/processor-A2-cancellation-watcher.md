---
status: idea
created: "2026-04-25T14:31:00Z"
---

<summary>
- Extract `watchForCancellation` (pkg/processor/processor.go line ~1146) into a `pkg/cancellationwatcher/` package behind a `Watcher` interface
- Single method: `Watch(ctx, promptPath, containerName) <-chan struct{}` — closes the returned channel when the prompt's status flips to `cancelled` (caller cancels its execution context)
- Removes ~50 lines from processor.go and removes the `*bool` out-parameter pattern
- Counterfeiter mock isolates fsnotify from processor tests
</summary>

<objective>
Pull the cancellation-watching goroutine out of the processor god-object so it can be tested without spinning up a real prompt file watcher.
</objective>

<context>
**Prerequisites:** `extract-validationprompt-package.md`, `processor-typed-primitives.md`, `processor-rename-and-reorder.md`, `processor-A1-leaf-extractions.md` must have landed first.

Read `CLAUDE.md` for project conventions.
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/`.

Current code (`pkg/processor/processor.go`):
- `watchForCancellation(ctx, execCancel, promptPath, containerName, cancelled *bool)` at line ~1146 — runs fsnotify on the prompt file, calls `executor.StopAndRemoveContainer` + `execCancel()` on a `cancelled` status flip, sets `*cancelled = true`
- Called as a goroutine from `runContainer` at line ~1092
- Uses: `fsnotify.NewWatcher`, `p.promptManager.Load`, `p.executor.StopAndRemoveContainer`

The existing `*bool` flag pattern is awkward (caller checks the bool after ctx returns). Replace with a returned read-only channel.
</context>

<requirements>

## 1. New package `pkg/cancellationwatcher/`

`pkg/cancellationwatcher/watcher.go`:

```go
package cancellationwatcher

//counterfeiter:generate -o ../../mocks/cancellation-watcher.go --fake-name Watcher . Watcher

// Watcher monitors a prompt file for cancellation and stops its container when triggered.
type Watcher interface {
    // Watch starts a goroutine that watches promptPath for status==cancelled.
    // When detected, it stops/removes the container and closes the returned channel.
    // The goroutine exits when ctx is cancelled or the cancellation channel is closed.
    Watch(ctx context.Context, promptPath string, containerName processor.ContainerName) <-chan struct{}
}

func NewWatcher(executor executor.Executor, promptManager processor.PromptManager) Watcher { ... }
```

Move the body of `watchForCancellation` into `(*watcher).Watch`. Replace the `*bool` out-param + external `execCancel` with: caller selects on the returned channel and cancels its own exec context when the channel closes.

## 2. Update `runContainer` in processor

```go
execCtx, execCancel := context.WithCancel(ctx)
defer execCancel()

cancelled := p.cancellationWatcher.Watch(execCtx, promptPath, containerName)
go func() {
    select {
    case <-execCtx.Done():
    case <-cancelled:
        execCancel()
    }
}()

execErr := p.executor.Execute(execCtx, content, logFile, containerName)
// existing post-exec branches...

select {
case <-cancelled:
    return true, nil  // cancelled by user
default:
    // fall through to existing error handling
}
```

Adjust to fit the existing return signature `(cancelled bool, err error)`.

## 3. Wire into processor

- Add `cancellationWatcher cancellationwatcher.Watcher` to `processor` struct
- Add as constructor parameter (services group)
- Delete `watchForCancellation` method from `processor.go`

## 4. Wire from factory

`pkg/factory/factory.go`: construct `cancellationwatcher.NewWatcher(executor, promptManager)` and pass.

## 5. Tests

- `pkg/cancellationwatcher/watcher_test.go` (external test pkg, uses real fsnotify on a `t.TempDir()`): cover — no status change (channel never closes), status flips to cancelled (channel closes, `StopAndRemoveContainer` invoked), ctx cancelled mid-watch (goroutine exits cleanly), watcher cannot be created (logs and returns)
- Update processor tests that exercised cancellation to use the counterfeiter mock

## 6. CHANGELOG

```
- refactor: extracted CancellationWatcher from processor — replaces *bool out-parameter with a closed-on-cancel channel; pure refactor
```

## 7. Verify

```bash
cd /workspace
make generate
make precommit
```

Both must exit 0.

</requirements>

<constraints>
- Pure refactor — no behaviour change visible to callers other than internals of `runContainer`
- The new channel-based signal must be drained / handled — no goroutine leaks
- External test packages
- Coverage ≥80% on new package
- `errors.Wrap` from `github.com/bborbe/errors`
- Do not commit
</constraints>

<verification>
```bash
cd /workspace

! grep -n "func (p \*processor) watchForCancellation" pkg/processor/processor.go
ls pkg/cancellationwatcher/watcher.go mocks/cancellation-watcher.go
grep -n "cancellationwatcher.Watcher" pkg/processor/processor.go

make precommit
```
</verification>
