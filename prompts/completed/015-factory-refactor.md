---
status: completed
container: dark-factory-015-factory-refactor
---


# Refactor factory to follow Go factory pattern

## Goal

Refactor `pkg/factory/factory.go` to follow the Go factory pattern from `~/.claude-yolo/docs/go-factory-pattern.md`. Currently the "Factory" is actually a runner — it contains ~300 lines of business logic (file watching, prompt processing, signal handling, debouncing). The factory pattern requires zero business logic.

## Current Violations

### Factory violations
1. **Named `New` instead of `Create*`** — `func New(exec executor.Executor) Factory`
2. **Massive business logic in factory** — `Run()`, `watchLoop()`, `handleWatchEvent()`, `handleFileEvent()`, `processExistingQueued()`, `processPrompt()`, `sanitizeContainerName()`
3. **Factory IS a runner** — `Factory` interface has `Run(ctx context.Context) error`
4. **Not in factory file** — all code is in one file mixing factory + implementation

### Constructor pattern violations
5. **Multi-method interface** — `Factory` has 3 methods (`Run`, `SetPromptsDir`, `GetPromptsDir`) instead of single-method SRP
6. **Setters/getters instead of DI** — `SetPromptsDir()`, `GetPromptsDir()` instead of constructor parameter
7. **Interface exposes state** — getters/setters expose internal state, interface methods should do real work

## Target Architecture

### 1. `pkg/runner/runner.go` — Constructor pattern: public interface + private struct + `New*`

Follow `~/.claude-yolo/docs/go-factory-pattern.md` "Constructor Pattern" section:

```go
package runner

// Runner orchestrates the main processing loop.
// Single-method interface (SRP).
type Runner interface {
    Run(ctx context.Context) error
}

// Private struct — lowercase, unexported.
type runner struct {
    promptsDir string
    executor   executor.Executor
    processMu  sync.Mutex
}

// NewRunner creates a new Runner.
// All dependencies via constructor params — no setters.
func NewRunner(promptsDir string, exec executor.Executor) Runner {
    return &runner{
        promptsDir: promptsDir,
        executor:   exec,
    }
}
```

Move these methods to the runner struct:
- `Run()` (including signal handling)
- `watchLoop()`
- `handleWatchEvent()`
- `handleFileEvent()`
- `processExistingQueued()`
- `processPrompt()`
- `sanitizeContainerName()`

No `SetPromptsDir` / `GetPromptsDir` — `promptsDir` is a constructor parameter, not mutable state.

### 2. `pkg/factory/factory.go` — Pure factory (zero logic)

```go
package factory

// CreateRunner creates a Runner that watches prompts/ and executes via Docker.
func CreateRunner(promptsDir string) runner.Runner {
    return runner.NewRunner(
        promptsDir,
        executor.NewDockerExecutor(),
    )
}
```

### 3. `cmd/dark-factory/main.go` — Update to use factory

```go
r := factory.CreateRunner(promptsDir)
if err := r.Run(ctx); err != nil {
    log.Fatal(err)
}
```

## Steps

1. Create `pkg/runner/` package with `Runner` interface
2. Move all business logic from `pkg/factory/factory.go` to `pkg/runner/runner.go`
3. Remove `SetPromptsDir`/`GetPromptsDir` — accept `promptsDir` in constructor
4. Reduce `pkg/factory/factory.go` to a single `CreateRunner()` function
5. Update `cmd/dark-factory/main.go` to use `factory.CreateRunner()`
6. Move factory tests to `pkg/runner/runner_test.go` (they test business logic)
7. Add simple factory test in `pkg/factory/factory_test.go` (just verifies CreateRunner returns non-nil)
8. Update mocks if needed (counterfeiter generate directives)

## Constraints

- Run `make precommit` for validation only — do NOT commit, tag, or push (dark-factory handles all git operations)
- Keep all existing behavior identical — this is a pure refactoring
- Follow `~/.claude-yolo/docs/go-factory-pattern.md` strictly
