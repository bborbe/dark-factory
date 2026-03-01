---
status: completed
---


# Fix code review findings

## Critical

### 1. `processExistingQueued` infinite loop missing `ctx.Done()` check

`pkg/factory/factory.go:149` — the `for {}` loop has no shutdown guard. If SIGINT arrives between prompts, the loop doesn't exit until the next `processPrompt` returns.

Fix: add `ctx.Done()` check at top of loop:

```go
for {
    select {
    case <-ctx.Done():
        return nil
    default:
    }
    queued, err := prompt.ListQueued(ctx, f.promptsDir)
    ...
```

### 2. `BumpPatchVersion` uses `context.Background()`

`pkg/git/git.go:106` — business logic must never create its own context.

Fix: add `ctx context.Context` as first parameter to `BumpPatchVersion` and pass it through the call chain from `getNextVersion` → `GetNextVersion` → `CommitAndRelease`.

## Important

### 3. Constructors return concrete types instead of interfaces

`NewDockerExecutor()` returns `*DockerExecutor` and `New()` returns `*Factory`. Per project patterns, constructors must return the interface type and structs should be unexported.

Fix:
- `pkg/executor/executor.go`: `DockerExecutor` → `dockerExecutor`, `NewDockerExecutor() Executor`
- `pkg/factory/factory.go`: `Factory` struct → `factory`, `New(...) Factory` (requires defining `Factory` interface with `Run`)

Update counterfeiter annotations and run `make generate` after interface changes.

### 4. Missing `.PHONY` on 14/15 Makefile targets

Only `test` has `.PHONY`. All other targets (`precommit`, `ensure`, `format`, `generate`, `check`, `lint`, `vet`, `errcheck`, `vulncheck`, `osv-scanner`, `gosec`, `trivy`, `addlicense`, `run`, `default`) are missing it.

Fix: add `.PHONY` declaration for every target.

### 5. Counterfeiter `--fake-name` has "Fake" prefix

`pkg/executor/executor.go:19`: `--fake-name FakeExecutor` should be `--fake-name Executor`.

Fix the annotation, run `make generate`, update all test references from `mocks.FakeExecutor` to `mocks.Executor`.

### 6. Mock variable uses "mock" prefix

`pkg/factory/factory_test.go:134,180,229`: `mockExec` → `exec` (or `fakeExec`).

### 7. Missing `//go:generate` in git suite

`pkg/git/git_suite_test.go` is missing:
```go
//go:generate go run -mod=mod github.com/maxbrunsfeld/counterfeiter/v6 -generate
```

### 8. `actions/checkout@v6` doesn't exist (should be v4)

`.github/workflows/ci.yml` and `.github/workflows/claude-code-review.yml` reference `actions/checkout@v6`. Fix to `actions/checkout@v4`.

### 9. `~/.claude-yolo` volume mount should be read-only

`pkg/executor/executor.go:91`: add `:ro` flag:
```go
"-v", home+"/.claude-yolo:/home/node/.claude:ro",
```

### 10. Container name not validated against Docker charset

`pkg/factory/factory.go`: after deriving `containerName`, validate/sanitize `baseName` to `[a-zA-Z0-9_-]` before use with `--name`.

## Constraints

- Run `make generate` after any interface changes
- Run `make precommit` before finishing
- Verify all packages build: `go build ./...`
- Verify tests pass: `make test`
