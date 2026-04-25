---
status: committing
summary: Extracted watchForCancellation from processor into pkg/cancellationwatcher with a Watcher interface — replaces *bool out-parameter with a closed-on-cancel channel; counterfeiter mock generated; all NewProcessor call sites updated; make precommit exited 0
container: dark-factory-341-processor-A2-cancellation-watcher
dark-factory-version: v0.135.3-1-gf3b7a3f
created: "2026-04-25T14:31:00Z"
queued: "2026-04-25T15:41:25Z"
started: "2026-04-25T16:07:27Z"
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
    //
    // containerName is passed as a string to avoid an import cycle with pkg/processor.
    // PromptLoader is a minimal local interface for the same reason.
    Watch(ctx context.Context, promptPath string, containerName string) <-chan struct{}
}

// PromptLoader is the minimal subset of processor.PromptManager that this package needs.
// Defined locally to avoid an import cycle (pkg/processor imports cancellationwatcher).
type PromptLoader interface {
    Load(ctx context.Context, path string) (*prompt.PromptFile, error)
}

func NewWatcher(executor executor.Executor, promptLoader PromptLoader) Watcher { ... }
```

**Avoiding the import cycle is critical.** `pkg/processor` will import `cancellationwatcher`. Therefore `cancellationwatcher` MUST NOT import `pkg/processor`. The two ways to handle dependencies on processor types:

1. Use primitive types (`string` instead of `processor.ContainerName`) in this package's API
2. Define a minimal local interface (like `PromptLoader` above) — Go interface satisfaction is structural, so `processor.PromptManager` satisfies `PromptLoader` automatically

Both are applied in the signature above.

Move the body of `watchForCancellation` into `(*watcher).Watch`. Replace the `*bool` out-param + external `execCancel` with: caller selects on the returned channel and cancels its own exec context when the channel closes.

## 2. Update `runContainer` in processor

```go
execCtx, execCancel := context.WithCancel(ctx)
defer execCancel()

cancelled := p.cancellationWatcher.Watch(execCtx, promptPath, string(containerName))
// IMPORTANT: track whether cancellation closed BEFORE Execute returned.
// Otherwise a watcher that closes its channel during/after Execute's natural
// return (e.g., from ctx cleanup) would be misclassified as a user-cancel.
cancelledByUser := false
go func() {
    select {
    case <-execCtx.Done():
        return
    case <-cancelled:
        cancelledByUser = true
        execCancel()
    }
}()

execErr := p.executor.Execute(execCtx, content, logFile, containerName)

if cancelledByUser {
    return true, nil
}
// fall through to existing error handling for execErr
```

Adjust to fit the existing return signature `(cancelled bool, err error)`. The `cancelledByUser` flag is set under the goroutine and read after Execute returns — they don't overlap because Execute blocks until either it finishes naturally OR the watcher's `execCancel()` fires. The `select default` pattern in the original sketch races; this version doesn't.

## 3. Wire into processor

- Add `cancellationWatcher cancellationwatcher.Watcher` to `processor` struct
- Add as constructor parameter (services group)
- Delete `watchForCancellation` method from `processor.go`

## 4. Wire from factory and update ALL `NewProcessor` call sites

```bash
grep -rn "processor\.NewProcessor(" --include="*.go"
```

Update every call site (factory + all test files — recurring lesson from 337/338/339: a `newTestProcessor` helper does NOT cover all direct constructor calls in tests).

`pkg/factory/factory.go`: construct `cancellationwatcher.NewWatcher(executor, promptManager)` and pass into `NewProcessor`.

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

# No reverse import — cancellationwatcher MUST NOT import processor
! grep -n "github.com/bborbe/dark-factory/pkg/processor" pkg/cancellationwatcher/

# All NewProcessor call sites updated
grep -rn "processor\.NewProcessor(" --include='*.go'

make precommit
```
</verification>
