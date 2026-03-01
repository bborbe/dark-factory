# Split watcher and processor into independent goroutines

## Goal

Separate prompt normalization (rename, validate, set frontmatter) from prompt execution into two independent processes. Currently both happen in the same loop, so renaming only occurs when the processor looks for work.

## Current Behavior

Runner does everything sequentially:
1. Scan for prompts
2. Normalize filenames (rename to NNN-name.md)
3. Find queued prompt
4. Execute it
5. Back to 1

Problem: If you drop 3 files while a prompt is executing, they sit un-renamed until execution finishes.

## Expected Behavior

Two independent goroutines:

**Watcher/Normalizer** (runs continuously):
- Watches `prompts/` via fsnotify
- On any file event: normalize filenames immediately
- Sets `status: queued` if no frontmatter exists
- Validates frontmatter format
- Renames to `NNN-name.md` (sequential numbering)
- Runs even while a prompt is executing

**Processor** (runs continuously):
- Scans for prompts with `status: queued`
- Picks the lowest-numbered one
- Sets `status: executing`, runs container
- On success: moves to `completed/`
- On failure: sets `status: failed`
- Does NOT rename or validate — assumes watcher already did it

Drop 3 files quickly → watcher renames them all to 028, 029, 030 instantly, even while 027 is still executing.

## Implementation

### 1. Extract `pkg/watcher/` package

```go
//counterfeiter:generate -o ../../mocks/watcher.go --fake-name Watcher . Watcher
type Watcher interface {
    Watch(ctx context.Context) error
}
```

Responsibilities:
- fsnotify watch on `prompts/`
- Debounce (500ms, existing logic)
- Call `promptManager.NormalizeFilenames()` on every event
- Ensure new files get `status: queued` frontmatter
- Runs in its own goroutine

### 2. Extract `pkg/processor/` package

```go
//counterfeiter:generate -o ../../mocks/processor.go --fake-name Processor . Processor
type Processor interface {
    Process(ctx context.Context) error
}
```

Responsibilities:
- `ListQueued()` → find prompts with `status: queued`
- Pick lowest number
- Execute via Docker
- Move to completed, commit, tag, push
- Triggered by watcher (via channel) or periodic scan

### 3. Communication between watcher and processor

Use a channel or simple signal:
```go
// Watcher sends signal when new queued prompt is ready
ready chan struct{}
```

Watcher normalizes files, then signals processor. Processor also does a periodic scan (e.g., on startup) for any queued prompts that were there before the watcher started.

### 4. Simplify runner using `github.com/bborbe/run`

Use the existing `run.CancelOnFirstError` from `github.com/bborbe/run` to coordinate goroutines. If either goroutine fails, context cancels the other automatically.

```go
import "github.com/bborbe/run"

func (r *runner) Run(ctx context.Context) error {
    // Acquire lock
    return run.CancelOnFirstError(ctx,
        r.watcher.Watch,     // goroutine 1: normalize files
        r.processor.Process,  // goroutine 2: execute prompts
    )
}
```

No manual goroutine management, no WaitGroup, no error collection — `run` handles it all.

Add dependency: `go get github.com/bborbe/run`

### 5. Update factory

- `CreateWatcher(promptsDir, promptManager) Watcher`
- `CreateProcessor(promptsDir, executor, promptManager, releaser) Processor`
- `CreateRunner(watcher, processor, locker) Runner`

### 6. Tests

- Watcher renames files immediately on event
- Watcher works while processor is executing
- Processor only picks `status: queued` prompts
- Processor ignores un-normalized files
- Multiple files dropped → all renamed with correct sequential numbers
- Integration: drop 3 files → all get correct NNN prefix before any executes
- Shutdown: both goroutines stop cleanly on context cancel

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Follow `~/.claude-yolo/docs/go-patterns.md` (interface + struct + New*)
- Follow `~/.claude-yolo/docs/go-factory-pattern.md` (Create* in factory, zero logic)
- Follow `~/.claude-yolo/docs/go-composition.md` (inject deps)
- Coverage ≥80% for new packages
