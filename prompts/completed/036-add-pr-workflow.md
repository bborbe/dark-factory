---
status: completed
container: dark-factory-036-add-pr-workflow
dark-factory-version: v0.8.1
---



# Add PR-based workflow (feature branch + pull request)

## Goal

When `workflow: pr` is configured in `.dark-factory.yaml`, dark-factory should create a feature branch before executing YOLO, then push and create a PR instead of committing directly to master. This enables dark-factory to work on projects that require code review.

Default is `workflow: direct` — this change does NOT affect existing behavior.

## Current Behavior (workflow: direct)

```
YOLO executes → commit completed file → changelog update → tag → push master
```

## Expected Behavior (workflow: pr)

```
create branch → YOLO executes → commit completed file → commit → push branch → create PR → switch back to master
```

No changelog update, no tag, no version bump on the branch. Those happen when the PR is merged to master (handled by human or CI).

## Implementation

### 1. Add `Brancher` interface in `pkg/git/`

```go
// pkg/git/brancher.go

//counterfeiter:generate -o ../../mocks/brancher.go --fake-name Brancher . Brancher
type Brancher interface {
    CreateAndSwitch(ctx context.Context, name string) error
    Push(ctx context.Context, name string) error
    Switch(ctx context.Context, name string) error
    CurrentBranch(ctx context.Context) (string, error)
}
```

Use subprocess `git` (not go-git) — consistent with how `Releaser` does push:
- `git checkout -b <name>` for create
- `git push -u origin <name>` for push
- `git checkout <name>` for switch
- `git rev-parse --abbrev-ref HEAD` for current branch

### 2. Add `PRCreator` interface in `pkg/git/`

```go
// pkg/git/pr_creator.go

//counterfeiter:generate -o ../../mocks/pr-creator.go --fake-name PRCreator . PRCreator
type PRCreator interface {
    Create(ctx context.Context, title string, body string) (string, error)
}
```

Uses `gh pr create --title <title> --body <body>`. Returns the PR URL.

### 3. Inject `Config.Workflow`, `Brancher`, `PRCreator` into processor

Update `processor` struct and `NewProcessor` to accept:
- `workflow config.Workflow`
- `brancher git.Brancher`
- `prCreator git.PRCreator`

### 4. Update `processPrompt()` in processor

```go
func (p *processor) processPrompt(ctx context.Context, pr prompt.Prompt) error {
    // ... existing: content check, metadata setup, title ...

    // PR mode: create feature branch before execution
    originalBranch := ""
    branchName := ""
    if p.workflow == config.WorkflowPR {
        var err error
        originalBranch, err = p.brancher.CurrentBranch(ctx)
        if err != nil {
            return errors.Wrap(ctx, err, "get current branch")
        }
        branchName = "dark-factory/" + baseName
        if err := p.brancher.CreateAndSwitch(ctx, branchName); err != nil {
            return errors.Wrap(ctx, err, "create feature branch")
        }
    }

    // ... existing: execute YOLO, move to completed, commit completed file ...

    gitCtx := context.WithoutCancel(ctx)

    if p.workflow == config.WorkflowPR {
        // PR mode: simple commit + push branch + create PR
        if err := p.releaser.CommitOnly(gitCtx, title); err != nil {
            return errors.Wrap(ctx, err, "commit changes")
        }
        if err := p.brancher.Push(gitCtx, branchName); err != nil {
            return errors.Wrap(ctx, err, "push branch")
        }
        prURL, err := p.prCreator.Create(gitCtx, title, "Automated by dark-factory")
        if err != nil {
            return errors.Wrap(ctx, err, "create pull request")
        }
        log.Printf("dark-factory: created PR: %s", prURL)

        // Switch back to original branch for next prompt
        if err := p.brancher.Switch(gitCtx, originalBranch); err != nil {
            return errors.Wrap(ctx, err, "switch back to %s", originalBranch)
        }
    } else {
        // Direct mode: existing behavior (changelog, tag, push)
        // ... keep existing code unchanged ...
    }

    return nil
}
```

### 5. Update factory

Pass workflow, brancher, and PR creator to processor:

```go
func CreateProcessor(
    queueDir string,
    completedDir string,
    promptManager prompt.Manager,
    releaser git.Releaser,
    versionGetter version.Getter,
    ready <-chan struct{},
    containerImage string,
    workflow config.Workflow,
    brancher git.Brancher,
    prCreator git.PRCreator,
) processor.Processor
```

In `CreateRunner`:
```go
CreateProcessor(
    queueDir, completedDir, promptManager, releaser,
    versionGetter, ready, cfg.ContainerImage,
    cfg.Workflow, git.NewBrancher(), git.NewPRCreator(),
)
```

### 6. Tests

**Brancher:**
- `CreateAndSwitch` runs `git checkout -b <name>`
- `Push` runs `git push -u origin <name>`
- `Switch` runs `git checkout <name>`
- `CurrentBranch` returns current branch name

**PRCreator:**
- `Create` runs `gh pr create` with title and body
- Returns PR URL from stdout

**Processor with WorkflowPR:**
- `CreateAndSwitch` called with `dark-factory/<prompt-name>` before YOLO execution
- `CommitOnly` called (NOT `CommitAndRelease`)
- No changelog update, no tag
- `Push` called with branch name
- `Create` called with prompt title
- `Switch` called with original branch after PR
- On YOLO failure: stays on feature branch, marks failed

**Processor with WorkflowDirect:**
- Existing behavior unchanged
- Brancher and PRCreator NOT called
- CommitAndRelease called with changelog/tag as before

## Branch Naming

Pattern: `dark-factory/<prompt-basename>`

Examples:
- Prompt `017-fix-double-commit.md` → branch `dark-factory/017-fix-double-commit`
- Prompt `add-metrics.md` → branch `dark-factory/add-metrics`

## Error Handling

- **Branch creation fails**: don't execute YOLO, mark failed, stay on original branch
- **YOLO fails**: mark failed, stay on feature branch (user can inspect)
- **Push fails**: log error with branch name, mark failed, stay on feature branch
- **PR creation fails after push**: log error, mark failed, stay on feature branch (branch is pushed, user can create PR manually)
- **Switch back fails**: log error but don't mark failed (PR was created successfully)

## Constraints

- Backward compatible — `workflow: direct` (default) = current behavior unchanged
- `gh` CLI must be available on host (dark-factory runs on host, not in Docker)
- Use subprocess `git` for all branch operations (not go-git)
- Run `make precommit` for validation only — do NOT commit, tag, or push
- Follow `~/.claude-yolo/docs/go-patterns.md` for interface + constructor + counterfeiter
- Follow `~/.claude-yolo/docs/go-composition.md` for dependency injection
- Follow `~/.claude-yolo/docs/go-factory-pattern.md` for zero-logic factory
- Follow `~/.claude-yolo/docs/go-precommit.md` for linter limits
- Coverage ≥80% for changed packages
