# Fetch and merge origin/master before each prompt execution

## Goal

Before executing each prompt, dark-factory should run `git fetch origin` and `git merge origin/master` to ensure it starts from the latest code. This prevents working on stale local state.

## Current Behavior

dark-factory executes prompts against whatever local state exists. If someone pushed to origin/master since the last prompt, the next prompt works on stale code.

## Expected Behavior

At the start of `processPrompt`, before any workflow-specific logic:
1. `git fetch origin`
2. `git merge origin/master`

If the merge fails (conflicts), mark the prompt as failed with a clear error message.

## Implementation

### 1. Add `Fetch` and `MergeOriginMaster` to `Brancher` interface in `pkg/git/brancher.go`

```go
type Brancher interface {
    CreateAndSwitch(ctx context.Context, name string) error
    Push(ctx context.Context, name string) error
    Switch(ctx context.Context, name string) error
    CurrentBranch(ctx context.Context) (string, error)
    Fetch(ctx context.Context) error
    MergeOriginMaster(ctx context.Context) error
}
```

Implementation uses `exec.CommandContext`:
- `Fetch`: `git fetch origin`
- `MergeOriginMaster`: `git merge origin/master`

### 2. Update counterfeiter mock

Run `go generate ./pkg/git/...` to regenerate `mocks/brancher.go`.

### 3. Call in `processPrompt` in `pkg/processor/processor.go`

Add at the very start of `processPrompt`, before loading the prompt file:

```go
// Sync with remote before execution
slog.Info("syncing with origin/master")
if err := p.brancher.Fetch(ctx); err != nil {
    return errors.Wrap(ctx, err, "git fetch origin")
}
if err := p.brancher.MergeOriginMaster(ctx); err != nil {
    return errors.Wrap(ctx, err, "git merge origin/master")
}
```

### 4. Tests

In `pkg/git/brancher_test.go`:
- `Fetch` succeeds in a repo with a remote
- `MergeOriginMaster` succeeds when no conflicts
- `MergeOriginMaster` returns error on conflict

In `pkg/processor/processor_test.go`:
- Verify `Fetch` and `MergeOriginMaster` are called before execution
- Verify prompt fails if `Fetch` fails
- Verify prompt fails if `MergeOriginMaster` fails

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Coverage ≥80% for changed packages
- Use `exec.CommandContext` for git commands (consistent with existing brancher methods)
