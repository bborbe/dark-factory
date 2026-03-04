# Add auto-merge and auto-release to PR workflow

## Goal

After creating a PR (`workflow: pr` or `workflow: worktree`), optionally watch it until mergeable, auto-merge to the default branch, then optionally do a full changelog + tag + push release — all driven by two new config flags. Also removes all hardcoded `master` references in favor of detecting the repo's default branch via `gh repo view`.

## New Config Fields

Add to `pkg/config/config.go` `Config` struct (both default `false`):

```go
AutoMerge   bool `yaml:"autoMerge"`   // watch PR until mergeable, then merge to default branch
AutoRelease bool `yaml:"autoRelease"` // after merge: update changelog, tag, push
```

Update `Defaults()` — both `false`.

Update `Validate()`:
- If `autoMerge` is `true` and `workflow` is `direct` → return validation error: `"autoMerge requires workflow 'pr' or 'worktree'"`
- If `autoRelease` is `true` and `autoMerge` is `false` → return validation error: `"autoRelease requires autoMerge"`

Update tests in `pkg/config/config_test.go`.

## New Interface: PRMerger

Add `pkg/git/pr_merger.go`:

```go
// PRMerger watches a PR until mergeable and merges it.
//
//counterfeiter:generate -o ../../mocks/pr_merger.go --fake-name PRMerger . PRMerger
type PRMerger interface {
    // WaitAndMerge polls the PR until mergeable (or timeout), then merges.
    // prURL is the URL returned by PRCreator.Create.
    // Only handles the GitHub PR lifecycle — does NOT touch local git state.
    WaitAndMerge(ctx context.Context, prURL string) error
}
```

Implement using `gh` CLI:
- Poll `gh pr view <prURL> --json mergeStateStatus` every 30 seconds
- `mergeStateStatus == "MERGEABLE"` → proceed
- `mergeStateStatus == "CONFLICTING"` → return error immediately
- Timeout after 30 minutes → return error
- Merge with `gh pr merge <prURL> --merge --delete-branch`
- Does NOT touch local git state (no checkout, no pull) — that's the processor's job

## Processor Changes

### `pkg/processor/processor.go`

Add fields to `processor` struct:
```go
autoMerge   bool
autoRelease bool
prMerger    git.PRMerger
```

Add parameters to `NewProcessor` (after `prCreator`):
```go
prMerger    git.PRMerger,
autoMerge   bool,
autoRelease bool,
```

### `handlePRWorkflow` changes

Replace the current tail of the method. After `slog.Info("created PR", "url", prURL)`:

```go
if p.autoMerge {
    slog.Info("waiting for PR to become mergeable", "url", prURL)
    if err := p.prMerger.WaitAndMerge(gitCtx, prURL); err != nil {
        // Switch back before returning error so next prompt starts clean
        _ = p.brancher.Switch(gitCtx, originalBranch)
        return errors.Wrap(ctx, err, "auto-merge PR")
    }
    slog.Info("PR merged", "url", prURL)

    // Switch to default branch and pull merged changes
    defaultBranch, err := p.brancher.DefaultBranch(gitCtx)
    if err != nil {
        return errors.Wrap(ctx, err, "resolve default branch")
    }
    if err := p.brancher.Switch(gitCtx, defaultBranch); err != nil {
        return errors.Wrap(ctx, err, "switch to "+defaultBranch+" after merge")
    }
    if err := p.brancher.Pull(gitCtx); err != nil {
        return errors.Wrap(ctx, err, "pull after merge")
    }

    if p.autoRelease && p.releaser.HasChangelog(gitCtx) {
        slog.Info("running auto-release on "+defaultBranch)
        // handleDirectWorkflow reads CHANGELOG.md ## Unreleased section
        // (which was written by YOLO on the feature branch and merged in)
        if err := p.handleDirectWorkflow(gitCtx, ctx, title); err != nil {
            return errors.Wrap(ctx, err, "auto-release after merge")
        }
    }
    // Already on default branch — done, no switch-back needed
    return nil
}

// autoMerge disabled — switch back to original branch for next prompt
if err := p.brancher.Switch(gitCtx, originalBranch); err != nil {
    return errors.Wrap(ctx, err, "switch back to "+originalBranch)
}

return nil
```

### `handleWorktreeWorkflow` changes

Same pattern: after creating the PR (before worktree cleanup), add the auto-merge + auto-release block. After merge, chdir back to `originalDir` and remove worktree, then switch to default branch + pull + optionally release.

## New Brancher Methods

Add to `Brancher` interface in `pkg/git/brancher.go`:

### `DefaultBranch(ctx context.Context) (string, error)`

Detects the repo's default branch name (e.g. `main`, `master`) via `gh`:

```go
// DefaultBranch returns the repo's default branch name from GitHub.
func (b *brancher) DefaultBranch(ctx context.Context) (string, error) {
    cmd := exec.CommandContext(ctx, "gh", "repo", "view", "--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name")
    output, err := cmd.Output()
    if err != nil {
        return "", errors.Wrap(ctx, err, "get default branch")
    }
    branch := strings.TrimSpace(string(output))
    if branch == "" {
        return "", errors.Errorf(ctx, "empty default branch from gh")
    }
    slog.Debug("default branch", "branch", branch)
    return branch, nil
}
```

### `Pull(ctx context.Context) error`

```go
// Pull pulls the current branch from remote.
func (b *brancher) Pull(ctx context.Context) error {
    slog.Debug("pulling from remote")
    cmd := exec.CommandContext(ctx, "git", "pull")
    if err := cmd.Run(); err != nil {
        return errors.Wrap(ctx, err, "git pull")
    }
    return nil
}
```

### Refactor `MergeOriginMaster`

Replace the hardcoded `origin/master` in the existing `MergeOriginMaster` method. Rename to `MergeOriginDefault` and use `DefaultBranch` to resolve the branch name:

```go
// MergeOriginDefault merges the remote default branch into the current branch.
func (b *brancher) MergeOriginDefault(ctx context.Context) error {
    defaultBranch, err := b.DefaultBranch(ctx)
    if err != nil {
        return errors.Wrap(ctx, err, "resolve default branch")
    }
    slog.Debug("merging remote default branch", "branch", defaultBranch)
    cmd := exec.CommandContext(ctx, "git", "merge", "origin/"+defaultBranch)
    if err := cmd.Run(); err != nil {
        return errors.Wrap(ctx, err, "merge origin/"+defaultBranch)
    }
    return nil
}
```

Update all callers of `MergeOriginMaster` → `MergeOriginDefault` (currently `processPrompt` in `processor.go`).

## Factory Changes

### `pkg/factory/factory.go`

Pass `git.NewPRMerger()`, `cfg.AutoMerge`, and `cfg.AutoRelease` to `CreateProcessor` and through to `processor.NewProcessor`.

## Docs

Update `README.md` config table and `example/.dark-factory.yaml`:

```yaml
autoMerge: false    # auto-merge PR when mergeable (requires workflow: pr or worktree)
autoRelease: false  # after merge: update changelog, tag, push (requires autoMerge)
```

## Tests

### `pkg/git/pr_merger_test.go`
- `WaitAndMerge` returns error when context is cancelled
- `WaitAndMerge` returns error on `CONFLICTING` state
- `WaitAndMerge` does NOT call any git checkout/pull commands (verify via mock or by checking no brancher calls)

### `pkg/processor/processor_test.go`
- When `autoMerge: false` → `prMerger.WaitAndMerge` never called, `brancher.Switch` called with original branch
- When `autoMerge: true` and PR merge succeeds → `prMerger.WaitAndMerge` called once, `brancher.DefaultBranch` called, `brancher.Switch(defaultBranch)` and `brancher.Pull` called
- When `autoMerge: true` and PR merge fails → `brancher.Switch(originalBranch)` called (cleanup), error returned
- When `autoMerge: true, autoRelease: true` and changelog exists → `releaser.CommitAndRelease` called after merge
- When `autoMerge: true, autoRelease: true` and no changelog → `releaser.CommitAndRelease` NOT called
- When `autoMerge: true, autoRelease: false` → `releaser.CommitAndRelease` never called

### `pkg/config/config_test.go`
- `Defaults()` has `AutoMerge: false, AutoRelease: false`
- YAML with `autoMerge: true` parses correctly
- YAML with `autoRelease: true` parses correctly
- Validation: `autoMerge: true` + `workflow: direct` → error
- Validation: `autoRelease: true` + `autoMerge: false` → error
- Validation: `autoMerge: true` + `workflow: pr` → no error

## Constraints

- Run `make precommit` for validation only
- Do NOT commit, tag, or push (dark-factory handles git)
- Coverage ≥80% for changed packages
- Regenerate mocks with `go generate ./...` after adding `PRMerger` interface and `Brancher.Pull` method
- Follow existing patterns in `pkg/git/` exactly (subprocess via `exec.CommandContext`, `// #nosec G204` comments for validated inputs)
