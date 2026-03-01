---
status: completed
container: dark-factory-016-extract-service-interfaces
---


# Extract service interfaces from runner god object

## Goal

The runner in `pkg/runner/runner.go` is a god object — it calls `prompt.*` and `git.*` package functions directly. Follow `~/.claude-yolo/docs/go-composition.md`: extract small single-responsibility interfaces, inject them via constructor, wire in factory.

## Current Problem

```go
// runner constructor takes only 2 deps but methods call 10+ package functions
func NewRunner(promptsDir string, exec executor.Executor) Runner

// Inside runner methods:
prompt.ResetExecuting(ctx, r.promptsDir)     // hard dep
prompt.NormalizeFilenames(ctx, r.promptsDir)  // hard dep
prompt.ListQueued(ctx, r.promptsDir)          // hard dep
prompt.ReadFrontmatter(ctx, path)             // hard dep
prompt.SetStatus(ctx, path, status)           // hard dep
prompt.SetContainer(ctx, path, name)          // hard dep
prompt.Content(ctx, path)                     // hard dep
prompt.Title(ctx, path)                       // hard dep
prompt.MoveToCompleted(ctx, path)             // hard dep
prompt.HasExecuting(ctx, r.promptsDir)        // hard dep
git.GetNextVersion(gitCtx)                    // hard dep
git.CommitAndRelease(gitCtx, title)           // hard dep
```

None of these can be mocked in tests. Constructor doesn't show what runner needs.

## Target Architecture

### 1. Extract interfaces in `pkg/prompt/`

```go
// pkg/prompt/prompt.go — add interfaces wrapping existing functions

// PromptManager manages prompt file operations.
//counterfeiter:generate -o ../../mocks/prompt-manager.go --fake-name PromptManager . PromptManager
type PromptManager interface {
    ResetExecuting(ctx context.Context) error
    HasExecuting(ctx context.Context) bool
    ListQueued(ctx context.Context) ([]Prompt, error)
    ReadFrontmatter(ctx context.Context, path string) (*Frontmatter, error)
    SetStatus(ctx context.Context, path string, status string) error
    SetContainer(ctx context.Context, path string, name string) error
    Content(ctx context.Context, path string) (string, error)
    Title(ctx context.Context, path string) (string, error)
    MoveToCompleted(ctx context.Context, path string) error
    NormalizeFilenames(ctx context.Context) ([]Rename, error)
}

type promptManager struct {
    dir string
}

func NewPromptManager(dir string) PromptManager {
    return &promptManager{dir: dir}
}
```

The `promptManager` struct methods delegate to the existing package-level functions — no logic change.

### 2. Extract interface in `pkg/git/`

```go
// pkg/git/git.go — add interface

// Releaser handles git commit, tag, and push.
//counterfeiter:generate -o ../../mocks/releaser.go --fake-name Releaser . Releaser
type Releaser interface {
    GetNextVersion(ctx context.Context) (string, error)
    CommitAndRelease(ctx context.Context, title string) error
}

type releaser struct{}

func NewReleaser() Releaser {
    return &releaser{}
}
```

The `releaser` struct methods delegate to existing private functions.

### 3. Update runner constructor

```go
func NewRunner(
    promptsDir string,
    executor executor.Executor,
    promptManager prompt.PromptManager,
    releaser git.Releaser,
) Runner {
    return &runner{
        promptsDir:    promptsDir,
        executor:      executor,
        promptManager: promptManager,
        releaser:      releaser,
    }
}
```

Replace all `prompt.Function()` calls with `r.promptManager.Method()`.
Replace all `git.Function()` calls with `r.releaser.Method()`.

### 4. Update factory

```go
func CreateRunner(promptsDir string) runner.Runner {
    return runner.NewRunner(
        promptsDir,
        executor.NewDockerExecutor(),
        prompt.NewPromptManager(promptsDir),
        git.NewReleaser(),
    )
}
```

### 5. Update tests

Runner tests should use counterfeiter mocks for `PromptManager` and `Releaser` instead of real files and git commands. This makes tests faster and more reliable.

## Steps

1. Add `PromptManager` interface + `promptManager` struct in `pkg/prompt/`
2. Add `Releaser` interface + `releaser` struct in `pkg/git/`
3. Add counterfeiter annotations on both interfaces
4. Update `runner` struct to hold `promptManager` and `releaser` fields
5. Update `NewRunner()` to accept the new deps
6. Replace all `prompt.*()` calls with `r.promptManager.*()` in runner
7. Replace all `git.*()` calls with `r.releaser.*()` in runner
8. Update `CreateRunner()` in factory to wire new deps
9. Update runner tests to use mocks
10. Run `go generate ./...` to generate mocks
11. Run `make precommit`

## Constraints

- Run `make precommit` for validation only — do NOT commit, tag, or push (dark-factory handles all git operations)
- Keep all existing behavior identical — this is a pure refactoring
- Follow `~/.claude-yolo/docs/go-composition.md` strictly
- Follow `~/.claude-yolo/docs/go-factory-pattern.md` for factory
- Follow `~/.claude-yolo/docs/go-patterns.md` for constructor pattern + counterfeiter
