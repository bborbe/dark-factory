---
status: completed
summary: Wired worktree workflow into processor and factory with comprehensive tests and refactoring
container: dark-factory-059-wire-worktree-workflow
dark-factory-version: v0.13.2
created: "2026-03-03T16:52:52Z"
queued: "2026-03-03T16:52:52Z"
started: "2026-03-03T16:52:52Z"
completed: "2026-03-03T17:05:07Z"
---
# Wire worktree workflow into processor and factory

## Goal

Add `handleWorktreeWorkflow` to the processor and wire the `Worktree` interface into the factory. When `workflow: worktree` is configured, dark-factory creates a git worktree for each prompt, executes inside it, commits, pushes, creates a PR, then cleans up the worktree.

## Current Behavior

- `workflow: pr` creates a branch in-place, executes, pushes, creates PR, switches back
- The main checkout is dirty during execution

## Expected Behavior

- `workflow: worktree` creates a git worktree at `../<projectName>-<promptBaseName>` with branch `dark-factory/<promptBaseName>`
- Execution happens inside the worktree (isolated)
- After execution: commit, push, create PR from worktree
- Clean up: remove worktree, switch back to original directory
- Main checkout stays clean throughout

## Implementation

### 1. Add `Worktree` field to processor

In `pkg/processor/processor.go`, add `worktree git.Worktree` to the struct and `NewProcessor` constructor parameters.

### 2. Add `handleWorktreeWorkflow` method

In `pkg/processor/processor.go`, add a new method following the pattern of `handlePRWorkflow`:

```go
func (p *processor) handleWorktreeWorkflow(
    gitCtx context.Context,
    ctx context.Context,
    title string,
    branchName string,
    worktreePath string,
) error {
    // Commit changes (we're already in the worktree directory)
    if err := p.releaser.CommitOnly(gitCtx, title); err != nil {
        return errors.Wrap(ctx, err, "commit changes")
    }

    // Push branch
    if err := p.brancher.Push(gitCtx, branchName); err != nil {
        return errors.Wrap(ctx, err, "push branch")
    }

    // Create PR
    prURL, err := p.prCreator.Create(gitCtx, title, "Automated by dark-factory")
    if err != nil {
        return errors.Wrap(ctx, err, "create pull request")
    }
    slog.Info("created PR", "url", prURL)

    // Switch back to original directory
    originalDir, _ := filepath.Abs(".")
    // ... we need to cd back to the project root before removing worktree

    // Remove worktree
    if err := p.worktree.Remove(gitCtx, worktreePath); err != nil {
        slog.Warn("failed to remove worktree", "path", worktreePath, "error", err)
        // Non-fatal — worktree cleanup is best-effort
    }

    return nil
}
```

### 3. Update `processPrompt` to handle worktree workflow

In the `processPrompt` method, add a `WorkflowWorktree` case alongside the existing `WorkflowPR` case. Key difference from PR mode:

- Instead of `brancher.CreateAndSwitch`, use `worktree.Add("../<projectName>-<baseName>", "dark-factory/<baseName>")`
- `os.Chdir` into the worktree directory before execution
- After execution, `os.Chdir` back to original directory
- Call `handleWorktreeWorkflow` instead of `handlePRWorkflow`
- Clean up worktree with `worktree.Remove`

**Important**: The worktree path should be a sibling directory: `filepath.Join("..", p.projectName+"-"+baseName)`.

**Important**: Use `defer` to ensure we always chdir back and attempt worktree cleanup, even on failure.

### 4. Wire into factory

In `pkg/factory/factory.go`:

- Add `git.NewWorktree()` to `CreateProcessor` call
- Pass it through to `processor.NewProcessor`

### 5. Tests

In `pkg/processor/processor_test.go`, add test cases for the worktree workflow:

- When workflow is `worktree`, `Worktree.Add` is called with correct path and branch name
- After successful execution, commit, push, PR create, and `Worktree.Remove` are called
- On execution failure, worktree is still cleaned up (defer)
- `Worktree.Remove` failure is logged but doesn't fail the prompt

Use the counterfeiter mock `mocks/worktree.go` generated in the previous prompt.

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Coverage ≥80% for changed packages
- Follow existing patterns in `handlePRWorkflow` closely
- The executor must run from within the worktree directory (os.Chdir)
- Always restore original directory and attempt cleanup, even on error
