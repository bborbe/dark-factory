---
status: queued
---
# Add PR-based workflow (feature branch + pull request)

## Goal

When `workflow: pr` is configured in `.dark-factory.yaml`, dark-factory should create a feature branch before executing YOLO, then push and create a PR instead of committing directly to master. This enables dark-factory to work on real projects that require code review.

## Prerequisites

- `add-project-config` prompt must be completed first (provides ConfigLoader + Config)

## Current Behavior (workflow: direct)

```
YOLO executes → git add → changelog update → commit → tag → push master
```

## Expected Behavior (workflow: pr)

```
create branch → YOLO executes → git add → commit → push branch → create PR → switch back to master
```

No changelog update, no tag, no version bump on the branch. Those happen when the PR is merged to master (handled by human or CI).

## Implementation

### 1. Add `Brancher` interface in `pkg/git/`

```go
// pkg/git/brancher.go

// Brancher handles git branch operations.
//
//counterfeiter:generate -o ../../mocks/brancher.go --fake-name Brancher . Brancher
type Brancher interface {
    CreateAndSwitchBranch(ctx context.Context, name string) error
    PushBranch(ctx context.Context, name string) error
    SwitchBranch(ctx context.Context, name string) error
}

type brancher struct{}

func NewBrancher() Brancher {
    return &brancher{}
}

func (b *brancher) CreateAndSwitchBranch(ctx context.Context, name string) error {
    cmd := exec.CommandContext(ctx, "git", "checkout", "-b", name)
    if err := cmd.Run(); err != nil {
        return errors.Wrap(ctx, err, "create branch")
    }
    return nil
}

func (b *brancher) PushBranch(ctx context.Context, name string) error {
    cmd := exec.CommandContext(ctx, "git", "push", "-u", "origin", name)
    if err := cmd.Run(); err != nil {
        return errors.Wrap(ctx, err, "push branch")
    }
    return nil
}

func (b *brancher) SwitchBranch(ctx context.Context, name string) error {
    cmd := exec.CommandContext(ctx, "git", "checkout", name)
    if err := cmd.Run(); err != nil {
        return errors.Wrap(ctx, err, "switch branch")
    }
    return nil
}
```

### 2. Add `PRCreator` interface in `pkg/git/`

```go
// pkg/git/pr_creator.go

// PRCreator creates GitHub pull requests.
//
//counterfeiter:generate -o ../../mocks/pr-creator.go --fake-name PRCreator . PRCreator
type PRCreator interface {
    CreatePR(ctx context.Context, title string, body string) (string, error)
}

type prCreator struct{}

func NewPRCreator() PRCreator {
    return &prCreator{}
}

func (p *prCreator) CreatePR(ctx context.Context, title string, body string) (string, error) {
    cmd := exec.CommandContext(ctx, "gh", "pr", "create",
        "--title", title,
        "--body", body,
    )
    output, err := cmd.Output()
    if err != nil {
        return "", errors.Wrap(ctx, err, "create PR")
    }
    return strings.TrimSpace(string(output)), nil
}
```

### 3. Update runner constructor

```go
func NewRunner(
    promptsDir string,
    exec executor.Executor,
    promptManager prompt.PromptManager,
    releaser git.Releaser,
    configLoader config.ConfigLoader,
    brancher git.Brancher,
    prCreator git.PRCreator,
) Runner
```

### 4. Update `processPrompt()` in runner

```go
func (r *runner) processPrompt(ctx context.Context, p prompt.Prompt) error {
    // ... existing: get content, set container, set status executing ...

    branchName := ""
    if r.config.Workflow == config.WorkflowPR {
        // Create feature branch before execution
        branchName = "dark-factory/" + baseName
        if err := r.brancher.CreateAndSwitchBranch(ctx, branchName); err != nil {
            return errors.Wrap(ctx, err, "create feature branch")
        }
    }

    // ... existing: execute YOLO ...

    // Move to completed
    if err := r.promptManager.MoveToCompleted(ctx, p.Path); err != nil {
        return errors.Wrap(ctx, err, "move to completed")
    }

    gitCtx := context.WithoutCancel(ctx)

    if r.config.Workflow == config.WorkflowPR {
        // PR mode: simple commit + push branch + create PR
        if err := r.releaser.CommitOnly(gitCtx, title); err != nil {
            return errors.Wrap(ctx, err, "commit changes")
        }
        if err := r.brancher.PushBranch(gitCtx, branchName); err != nil {
            return errors.Wrap(ctx, err, "push branch")
        }
        prURL, err := r.prCreator.CreatePR(gitCtx, title, "Automated by dark-factory")
        if err != nil {
            return errors.Wrap(ctx, err, "create pull request")
        }
        log.Printf("dark-factory: created PR: %s", prURL)

        // Switch back to master for next prompt
        if err := r.brancher.SwitchBranch(gitCtx, "master"); err != nil {
            return errors.Wrap(ctx, err, "switch back to master")
        }
    } else {
        // Direct mode: existing behavior
        nextVersion, err := r.releaser.GetNextVersion(gitCtx)
        if err != nil {
            return errors.Wrap(ctx, err, "get next version")
        }
        if err := r.releaser.CommitAndRelease(gitCtx, title); err != nil {
            return errors.Wrap(ctx, err, "commit and release")
        }
        log.Printf("dark-factory: committed and tagged %s", nextVersion)
    }

    return nil
}
```

### 5. Update factory

```go
func CreateRunner(promptsDir string) runner.Runner {
    return runner.NewRunner(
        promptsDir,
        executor.NewDockerExecutor(),
        prompt.NewPromptManager(promptsDir),
        git.NewReleaser(),
        config.NewConfigLoader(),
        git.NewBrancher(),
        git.NewPRCreator(),
    )
}
```

### 6. Tests

- Runner with `WorkflowPR` config:
  - Verify `CreateAndSwitchBranch` called with `dark-factory/<prompt-name>`
  - Verify `CommitOnly` called (not `CommitAndRelease`)
  - Verify `PushBranch` called
  - Verify `CreatePR` called with prompt title
  - Verify `SwitchBranch("master")` called after PR
- Runner with `WorkflowDirect` config:
  - Verify existing behavior unchanged (CommitAndRelease, no brancher/PR calls)
- Brancher unit tests: create branch, push, switch
- PRCreator unit tests: gh pr create

## Branch Naming

Pattern: `dark-factory/<prompt-basename>`

Examples:
- Prompt `017-fix-double-commit.md` → branch `dark-factory/017-fix-double-commit`
- Prompt `add-metrics.md` → branch `dark-factory/add-metrics`

## Error Handling

If PR creation fails after commit+push:
- Log the error with branch name
- Don't switch back to master (leave on feature branch for manual recovery)
- Mark prompt as failed

If branch creation fails:
- Don't execute YOLO
- Mark prompt as failed
- Stay on master

## Constraints

- Backward compatible — `workflow: direct` (default) = current behavior unchanged
- PR mode uses `CommitOnly` (depends on prompt 019-optional-changelog landing first)
- `gh` CLI must be available on host (dark-factory runs on host, not in Docker)
- Run `make precommit` for validation only — do NOT commit, tag, or push
- Follow `~/.claude-yolo/docs/go-composition.md` — small interfaces, all injected
- Follow `~/.claude-yolo/docs/go-patterns.md` for counterfeiter
