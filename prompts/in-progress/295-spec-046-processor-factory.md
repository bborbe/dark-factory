---
status: approved
spec: [046-workflow-enum-with-worktree-mode]
created: "2026-04-16T12:00:00Z"
queued: "2026-04-16T15:00:47Z"
branch: dark-factory/workflow-enum-with-worktree-mode
---

<summary>
- `NewProcessor` replaces the `worktree bool` parameter with `workflow config.Workflow` and gains a new `worktreer git.Worktreer` parameter
- The processor's internal routing switches on `p.workflow` (direct/branch/worktree/clone) instead of `p.worktree bool`
- `setupWorkflow` creates the right isolation for each workflow: branch-in-place for `branch`, worktree for `worktree`, clone for `clone`, nothing for `direct`
- A new `handleWorktreeWorkflow` implements the worktree isolation path (commit in worktree → chdir back → remove worktree → push → optional PR)
- A new `handleAfterIsolatedCommit` helper is extracted and shared by `handleCloneWorkflow` and `handleWorktreeWorkflow`; it handles push + optional PR + autoMerge/autoReview/complete
- `handleCloneWorkflow` is modified to call `handleAfterIsolatedCommit` and no longer unconditionally creates a PR
- The `workflow: branch, pr: true` path is fully wired: after in-place commit and branch restore, pushes and creates PR via `handleAfterIsolatedCommit`
- The `workflow: branch, pr: false` path continues via `handleBranchCompletion` (existing)
- `workflow: clone, pr: false` pushes the branch but does NOT open a PR (new behavior)
- `workflow: worktree, pr: false` pushes the branch but does NOT open a PR
- `reconstructWorkflowState` and `cleanupIsolationOnError` switch on `p.workflow` instead of `p.worktree`
- `CreateProcessor` in `pkg/factory/factory.go` replaces the `pr bool, worktree bool` parameters with `workflow config.Workflow` and passes `git.NewWorktreer()`
- `createDockerExecutor` call in factory passes `cfg.Workflow == config.WorkflowWorktree` for the `worktreeMode` flag added in prompt 2
- Unit tests verify all four workflow modes with mocked dependencies
- `make precommit` passes
</summary>

<objective>
Wire the four `workflow` enum values into the processor and factory, replacing the legacy `worktree bool` / `pr bool` dispatch. This prompt depends on prompt 1 (the config enum) and prompt 2 (the `Worktreer` interface and executor `--tmpfs` flag) being already merged into the branch. By the end of this prompt, all four workflow modes work end-to-end, PR creation is orthogonal to workflow, and the processor no longer contains `worktree bool`.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `docs/workflows.md` for the behavioral contract of each mode.

Files to read in full before editing:
- `pkg/processor/processor.go` — the full file; focus on:
  - `NewProcessor` signature (lines ~49–85)
  - `processor` struct (lines ~127–165)
  - `processPrompt` around line 920–955 (setupWorkflow + cleanup defer + handlePostExecution)
  - `handlePostExecution` lines ~1099–1158
  - `setupWorkflow` lines ~1189–1207
  - `setupCloneWorkflowState` lines ~1271–1295
  - `handleCloneWorkflow` lines ~1472–1539
  - `handleDirectWorkflow` lines ~1565–1615
  - `handleBranchCompletion` lines ~1616–1655
  - `handleAutoMergeForClone` lines ~1439–1470
  - `reconstructWorkflowState` lines ~396–426
  - `cleanupCloneOnError` lines ~1083–1097
  - `workflowState` struct lines ~1073–1081
- `pkg/factory/factory.go` — `createDockerExecutor` (line ~524), `CreateProcessor` (line ~559), and both call sites (lines ~302–320 and ~395–435)
- `pkg/git/worktreer.go` — the `Worktreer` interface added in prompt 2
- `mocks/worktreer.go` — the Counterfeiter fake added in prompt 2
- `mocks/cloner.go` — reference for mock usage in tests
</context>

<requirements>

## 1. Update `processor` struct and `NewProcessor`

### Remove `worktree bool`, add `workflow config.Workflow` and `worktreer git.Worktreer`

The current `NewProcessor` parameter order (verified from `pkg/processor/processor.go:49–85`) is:
```
queueDir, completedDir, logDir, projectName, exec, promptManager, releaser, versionGetter,
ready, pr bool, worktree bool, brancher, prCreator, cloner, prMerger, autoMerge, ...
```

The NEW parameter order after this change is:
```
queueDir, completedDir, logDir, projectName, exec, promptManager, releaser, versionGetter,
ready, pr bool, workflow config.Workflow, brancher, prCreator, cloner, worktreer git.Worktreer,
prMerger, autoMerge, ...
```

Specifically:
- DELETE the `worktree bool` parameter (was immediately after `pr bool`).
- INSERT `workflow config.Workflow` in the same slot (immediately after `pr bool`, immediately before `brancher`).
- INSERT `worktreer git.Worktreer` BETWEEN `cloner git.Cloner` and `prMerger git.PRMerger`.

Apply the same deletion/insertion to the `processor` struct fields (around line 127–165): remove `worktree bool`, add `workflow config.Workflow` and `worktreer git.Worktreer`.

Wire the new fields in the `return &processor{...}` block:
```go
workflow:  workflow,
worktreer: worktreer,
```

Remove the line `worktree: worktree` from the struct initializer.

## 2. Update `setupWorkflow`

Replace the body of `setupWorkflow` entirely. The current code checks `p.worktree` then `p.pr`. The new code switches on `p.workflow`:

```go
func (p *processor) setupWorkflow(
    ctx context.Context,
    baseName string,
    pf *prompt.PromptFile,
) (*workflowState, error) {
    state := &workflowState{}
    switch p.workflow {
    case config.WorkflowClone:
        return p.setupCloneWorkflowState(ctx, baseName, pf, state)
    case config.WorkflowWorktree:
        return p.setupWorktreeWorkflowState(ctx, baseName, pf, state)
    case config.WorkflowBranch:
        branch := pf.Branch()
        if branch == "" {
            branch = "dark-factory/" + baseName
        }
        return p.setupInPlaceBranchState(ctx, branch, state)
    default: // WorkflowDirect
        return state, nil
    }
}
```

### Add `setupWorktreeWorkflowState`

Add a new method adjacent to `setupCloneWorkflowState`. It is identical in structure but calls `p.worktreer.Add` instead of `p.cloner.Clone`:

```go
// setupWorktreeWorkflowState configures state for the worktree workflow.
func (p *processor) setupWorktreeWorkflowState(
    ctx context.Context,
    baseName string,
    pf *prompt.PromptFile,
    state *workflowState,
) (*workflowState, error) {
    branch := pf.Branch()
    if branch == "" {
        branch = "dark-factory/" + baseName
    }
    state.branchName = branch
    state.worktreePath = filepath.Join(os.TempDir(), "dark-factory", p.projectName+"-"+baseName)
    
    originalDir, err := os.Getwd()
    if err != nil {
        return nil, errors.Wrap(ctx, err, "get current directory")
    }
    state.originalDir = originalDir
    
    if err := p.worktreer.Add(ctx, state.worktreePath, branch); err != nil {
        return nil, errors.Wrap(ctx, err, "add worktree")
    }
    
    if err := os.Chdir(state.worktreePath); err != nil {
        // Remove worktree since we couldn't chdir into it
        _ = p.worktreer.Remove(ctx, state.worktreePath)
        return nil, errors.Wrap(ctx, err, "chdir to worktree")
    }
    
    return state, nil
}
```

### Update `workflowState` struct

Add `worktreePath string` field to `workflowState`. The `clonePath` field remains (used by clone mode); `worktreePath` is used by worktree mode. Both live in the same struct since only one is active at a time:

```go
type workflowState struct {
    branchName           string
    clonePath            string     // clone workflow only
    worktreePath         string     // worktree workflow only
    originalDir          string
    cleanedUp            bool
    inPlaceBranch        string
    inPlaceDefaultBranch string
}
```

## 3. Update `processPrompt` cleanup defer

At the point where the current code has:
```go
if p.worktree && workflowState.clonePath != "" {
    defer p.cleanupCloneOnError(ctx, workflowState)
}
```

Replace with:
```go
if (p.workflow == config.WorkflowClone || p.workflow == config.WorkflowWorktree) &&
    (workflowState.clonePath != "" || workflowState.worktreePath != "") {
    defer p.cleanupIsolationOnError(ctx, workflowState)
}
```

### Rename `cleanupCloneOnError` → `cleanupIsolationOnError`

Rename the function and update its body to handle both clone and worktree paths:

```go
// cleanupIsolationOnError restores the original directory and removes the clone or worktree
// when processPrompt exits with an error (success path cleanup is handled by each workflow handler).
func (p *processor) cleanupIsolationOnError(ctx context.Context, state *workflowState) {
    if state.cleanedUp {
        return
    }
    if state.originalDir != "" {
        if err := os.Chdir(state.originalDir); err != nil {
            slog.Warn("failed to chdir back to original directory on error", "error", err)
        }
    }
    if state.clonePath != "" {
        if err := p.cloner.Remove(ctx, state.clonePath); err != nil {
            slog.Warn("failed to remove clone on error", "path", state.clonePath, "error", err)
        }
    }
    if state.worktreePath != "" {
        if err := p.worktreer.Remove(ctx, state.worktreePath); err != nil {
            slog.Warn("failed to remove worktree on error", "path", state.worktreePath, "error", err)
        }
    }
}
```

Note: `worktreer.Remove` never returns an error (per the Worktreer contract), so the `if err` check here is nominal but harmless.

## 4. Update `reconstructWorkflowState`

Replace the `if !p.worktree` check with a workflow switch:

```go
func (p *processor) reconstructWorkflowState(
    ctx context.Context,
    baseName string,
    pf *prompt.PromptFile,
) (*workflowState, bool, error) {
    switch p.workflow {
    case config.WorkflowClone:
        // Clone workflow: check clone directory exists
        clonePath := filepath.Join(os.TempDir(), "dark-factory", p.projectName+"-"+baseName)
        if _, err := os.Stat(clonePath); err != nil {
            return nil, false, nil // Clone missing — signal reset-to-approved
        }
        branchName := pf.Branch()
        if branchName == "" {
            branchName = "dark-factory/" + baseName
        }
        originalDir, err := os.Getwd()
        if err != nil {
            return nil, false, errors.Wrap(ctx, err, "get working directory for resume")
        }
        return &workflowState{clonePath: clonePath, branchName: branchName, originalDir: originalDir}, true, nil

    case config.WorkflowWorktree:
        // Worktree workflow: check worktree directory exists
        worktreePath := filepath.Join(os.TempDir(), "dark-factory", p.projectName+"-"+baseName)
        if _, err := os.Stat(worktreePath); err != nil {
            return nil, false, nil // Worktree missing — signal reset-to-approved
        }
        branchName := pf.Branch()
        if branchName == "" {
            branchName = "dark-factory/" + baseName
        }
        originalDir, err := os.Getwd()
        if err != nil {
            return nil, false, errors.Wrap(ctx, err, "get working directory for resume")
        }
        return &workflowState{worktreePath: worktreePath, branchName: branchName, originalDir: originalDir}, true, nil

    default:
        // Direct or branch workflow: no isolated directory needed
        return &workflowState{}, true, nil
    }
}
```

## 5. Update `handlePostExecution` routing

Replace the current `if p.worktree { ... } else { ... }` dispatch with a workflow switch:

```go
func (p *processor) handlePostExecution(
    ctx context.Context,
    pf *prompt.PromptFile,
    promptPath string,
    title string,
    logFile string,
    state *workflowState,
) error {
    // ... (existing validation report + summary save code — UNCHANGED) ...

    completedPath := filepath.Join(p.completedDir, filepath.Base(promptPath))

    switch p.workflow {
    case config.WorkflowClone:
        return p.handleCloneWorkflow(gitCtx, ctx, pf, title, promptPath, completedPath, state)
    case config.WorkflowWorktree:
        return p.handleWorktreeWorkflow(gitCtx, ctx, pf, title, promptPath, completedPath, state)
    }

    // WorkflowBranch or WorkflowDirect — in-place path
    featureBranch := state.inPlaceBranch
    if err := p.moveToCompletedAndCommit(ctx, gitCtx, pf, promptPath, completedPath); err != nil {
        p.restoreDefaultBranch(ctx, state)
        return errors.Wrap(ctx, err, "move to completed and commit")
    }
    if err := p.handleDirectWorkflow(gitCtx, ctx, title, featureBranch); err != nil {
        p.restoreDefaultBranch(ctx, state)
        return errors.Wrap(ctx, err, "handle direct workflow")
    }
    p.restoreDefaultBranch(ctx, state)

    if featureBranch != "" {
        if p.pr {
            // workflow: branch, pr: true — push the feature branch and open a PR
            return p.handleBranchPRCompletion(gitCtx, ctx, pf, featureBranch, title, completedPath)
        }
        // workflow: branch, pr: false — check if last prompt on branch, merge+release
        return p.handleBranchCompletion(gitCtx, ctx, promptPath, title, featureBranch)
    }
    return nil
}
```

**Important:** The `gitCtx` and `ctx` variables come from `buildContexts` (or however the existing code obtains them at the top of `handlePostExecution`). Check how `gitCtx` is currently defined in `handlePostExecution` and replicate the pattern — do NOT change the context-building logic.

## 6. Add `handleAfterIsolatedCommit` shared helper

This helper is invoked from `handleCloneWorkflow` and `handleWorktreeWorkflow` after the code has been committed in the isolated directory and we have returned to the original repo. It handles push + optional PR:

```go
// handleAfterIsolatedCommit handles push + optional PR creation + prompt lifecycle completion
// for clone and worktree workflows. Called after: (1) code committed in isolated dir,
// (2) chdir back to original repo, (3) cleanup of isolated dir (clone removed / worktree removed).
// p.pr controls whether a PR is created. For pr: false, the branch is still pushed.
func (p *processor) handleAfterIsolatedCommit(
    gitCtx context.Context,
    ctx context.Context,
    pf *prompt.PromptFile,
    branchName string,
    title string,
    promptPath string,
    completedPath string,
) error {
    // Always push the feature branch
    if err := p.brancher.Push(gitCtx, branchName); err != nil {
        return errors.Wrap(ctx, err, "push branch")
    }

    if !p.pr {
        // No PR — just move prompt to completed
        return p.moveToCompletedAndCommit(ctx, gitCtx, pf, promptPath, completedPath)
    }

    // Find or create PR (idempotent)
    prURL, err := p.findOrCreatePR(gitCtx, ctx, branchName, title, pf.Issue())
    if err != nil {
        return errors.Wrap(ctx, err, "find or create PR")
    }

    if p.autoMerge {
        return p.handleAutoMergeForClone(gitCtx, ctx, pf, branchName, promptPath, completedPath, prURL, title)
    }

    if p.autoReview {
        p.savePRURLToFrontmatter(gitCtx, promptPath, prURL)
        if err := p.promptManager.SetStatus(ctx, promptPath, string(prompt.InReviewPromptStatus)); err != nil {
            return errors.Wrap(ctx, err, "set in_review status")
        }
        slog.Info("PR created, waiting for review", "url", prURL)
        return nil
    }

    // Default: move to completed and save PR URL
    if err := p.moveToCompletedAndCommit(ctx, gitCtx, pf, promptPath, completedPath); err != nil {
        return errors.Wrap(ctx, err, "move to completed and commit")
    }
    p.savePRURLToFrontmatter(gitCtx, completedPath, prURL)
    return nil
}
```

## 7. Refactor `handleCloneWorkflow` to use `handleAfterIsolatedCommit`

Replace the body of `handleCloneWorkflow` with:

```go
func (p *processor) handleCloneWorkflow(
    gitCtx context.Context,
    ctx context.Context,
    pf *prompt.PromptFile,
    title string,
    promptPath string,
    completedPath string,
    state *workflowState,
) error {
    branchName := state.branchName
    clonePath := state.clonePath
    originalDir := state.originalDir

    // Commit only code changes in the clone (no prompt files)
    if err := p.releaser.CommitOnly(gitCtx, title); err != nil {
        return errors.Wrap(ctx, err, "commit changes")
    }

    // Switch back to original directory before managing prompt
    if err := os.Chdir(originalDir); err != nil {
        return errors.Wrap(ctx, err, "chdir back to original directory")
    }

    // Remove clone (best-effort cleanup)
    if err := p.cloner.Remove(gitCtx, clonePath); err != nil {
        slog.Warn("failed to remove clone", "path", clonePath, "error", err)
    }
    state.cleanedUp = true

    // --- From here, we're back in the original repo ---
    return p.handleAfterIsolatedCommit(gitCtx, ctx, pf, branchName, title, promptPath, completedPath)
}
```

## 8. Add `handleWorktreeWorkflow`

Add a new method adjacent to `handleCloneWorkflow`. It mirrors the clone workflow but uses `p.worktreer` instead of `p.cloner`:

```go
// handleWorktreeWorkflow handles the worktree-based workflow: commit code in the worktree,
// remove the worktree, then manage the prompt lifecycle in the original repo.
func (p *processor) handleWorktreeWorkflow(
    gitCtx context.Context,
    ctx context.Context,
    pf *prompt.PromptFile,
    title string,
    promptPath string,
    completedPath string,
    state *workflowState,
) error {
    branchName := state.branchName
    worktreePath := state.worktreePath
    originalDir := state.originalDir

    // Commit only code changes in the worktree (no prompt files)
    if err := p.releaser.CommitOnly(gitCtx, title); err != nil {
        return errors.Wrap(ctx, err, "commit changes")
    }

    // Switch back to original directory before managing prompt
    if err := os.Chdir(originalDir); err != nil {
        return errors.Wrap(ctx, err, "chdir back to original directory")
    }

    // Remove worktree (best-effort cleanup — Remove never returns an error per interface contract)
    if err := p.worktreer.Remove(gitCtx, worktreePath); err != nil {
        slog.Warn("failed to remove worktree", "path", worktreePath, "error", err)
    }
    state.cleanedUp = true

    // --- From here, we're back in the original repo ---
    return p.handleAfterIsolatedCommit(gitCtx, ctx, pf, branchName, title, promptPath, completedPath)
}
```

## 9. Add `handleBranchPRCompletion`

Add a new method adjacent to `handleBranchCompletion`. Called when `workflow: branch, pr: true` and the prompt is already at `completedPath`:

```go
// handleBranchPRCompletion pushes the feature branch and creates a PR after an in-place commit.
// Called after: (1) prompt moved to completedPath + committed, (2) code committed on feature branch,
// (3) default branch restored. The prompt is already in completedPath when this runs.
func (p *processor) handleBranchPRCompletion(
    gitCtx context.Context,
    ctx context.Context,
    pf *prompt.PromptFile,
    featureBranch string,
    title string,
    completedPath string,
) error {
    // Push the feature branch (brancher.Push already uses `git push -u origin <name>` — verified
    // in pkg/git/brancher.go:85 — so upstream tracking is set on first push).
    if err := p.brancher.Push(gitCtx, featureBranch); err != nil {
        return errors.Wrap(ctx, err, "push feature branch")
    }

    // Find or create PR (idempotent)
    prURL, err := p.findOrCreatePR(gitCtx, ctx, featureBranch, title, pf.Issue())
    if err != nil {
        return errors.Wrap(ctx, err, "find or create PR")
    }

    if p.autoMerge {
        // Prompt already moved — check remaining and merge if last
        hasMore, err := p.promptManager.HasQueuedPromptsOnBranch(ctx, featureBranch, completedPath)
        if err != nil {
            slog.Warn("failed to check remaining prompts on branch", "branch", featureBranch, "error", err)
        }
        if hasMore {
            slog.Info("more prompts queued on branch — deferring auto-merge", "branch", featureBranch)
            p.savePRURLToFrontmatter(gitCtx, completedPath, prURL)
            return nil
        }
        if err := p.prMerger.WaitAndMerge(gitCtx, prURL); err != nil {
            return errors.Wrap(ctx, err, "wait and merge PR")
        }
        return p.postMergeActions(gitCtx, ctx, title)
    }

    // Default: save PR URL to the already-completed prompt
    p.savePRURLToFrontmatter(gitCtx, completedPath, prURL)
    return nil
}
```

## 10. Update `pkg/factory/factory.go`

### 10a. Update `CreateProcessor` signature

Replace the `pr bool, worktree bool` parameters with `workflow config.Workflow`:

```go
func CreateProcessor(
    inProgressDir string,
    completedDir string,
    logDir string,
    projectName string,
    promptManager prompt.Manager,
    releaser git.Releaser,
    versionGetter version.Getter,
    ready <-chan struct{},
    containerImage string,
    model string,
    netrcFile string,
    gitconfigFile string,
    workflow config.Workflow,   // replaces: pr bool, worktree bool
    pr bool,
    brancher git.Brancher,
    prCreator git.PRCreator,
    prMerger git.PRMerger,
    // ... rest unchanged ...
) processor.Processor {
    return processor.NewProcessor(
        inProgressDir,
        completedDir,
        logDir,
        projectName,
        createDockerExecutor(
            containerImage, projectName, model, netrcFile,
            gitconfigFile, env, extraMounts, claudeDir, maxPromptDuration,
            currentDateTimeGetter,
            workflow == config.WorkflowWorktree, // worktreeMode for --tmpfs flag
        ),
        promptManager,
        releaser,
        versionGetter,
        ready,
        pr,
        workflow,    // new parameter
        brancher,
        prCreator,
        git.NewCloner(),
        git.NewWorktreer(),  // new parameter
        prMerger,
        // ... rest unchanged ...
    )
}
```

### 10b. Update the two call sites of `CreateProcessor`

**Call site 1** (around line 302, `CreateRunner`):
```go
proc := CreateProcessor(
    inProgressDir, completedDir, cfg.Prompts.LogDir, projectName,
    promptManager, releaser, versionGetter, ready,
    cfg.ContainerImage, cfg.Model, cfg.NetrcFile, cfg.GitconfigFile,
    cfg.Workflow, cfg.PR,    // was: cfg.PR, cfg.Worktree
    deps.brancher, deps.prCreator, deps.prMerger,
    // ... rest unchanged ...
)
```

**Call site 2** (around line 395, `CreateOneShotRunner`):
```go
CreateProcessor(
    inProgressDir,
    completedDir,
    cfg.Prompts.LogDir,
    projectName,
    promptManager,
    releaser,
    versionGetter,
    make(chan struct{}, 10),
    cfg.ContainerImage,
    cfg.Model,
    cfg.NetrcFile,
    cfg.GitconfigFile,
    cfg.Workflow, cfg.PR,    // was: cfg.PR, cfg.Worktree
    deps.brancher,
    deps.prCreator,
    deps.prMerger,
    // ... rest unchanged ...
),
```

### 10c. Update `LogEffectiveConfig`

In `LogEffectiveConfig` (around line 80 — verified in the current file), update the logged fields: replace `"worktree", cfg.Worktree` with `"workflow", cfg.Workflow`. The `pr` field is already logged separately:

```go
// Replace:
"worktree", cfg.Worktree,
// With:
"workflow", cfg.Workflow,
```

## 11. Unit tests

Add a `Describe("processor workflow routing", ...)` block to `pkg/processor/processor_internal_test.go` (or wherever processor internal tests live). Use the existing `mocks.FakeCloner`, `mocks.FakeBrancher`, `mocks.FakePRCreator`, and `mocks.FakeWorktreer` from the `mocks` package.

### Required test cases

**11a. `workflow: worktree, pr: true`** — `setupWorktreeWorkflowState` calls `worktreer.Add` with the expected path and branch. After `handleWorktreeWorkflow`, `worktreer.Remove` is called once, `brancher.Push` is called once, `prCreator.Create` (or however `findOrCreatePR` works) is called once. Use `GinkgoT().TempDir()` for the project root; create a minimal git repo (`git init`) so `os.Getwd()` and `os.Chdir` work.

**11b. `workflow: worktree, pr: false`** — `worktreer.Add` called, `worktreer.Remove` called, `brancher.Push` called, PR creator NOT called. Prompt moved to completed.

**11c. `workflow: branch, pr: true`** — `brancher.CreateAndSwitch` or `brancher.FetchAndVerifyBranch` called (in-place branch setup). After execution, `brancher.Push` called, PR created. Prompt moved to completed.

**11d. `workflow: branch, pr: false`** — branch created in-place, committed, restored to default. `brancher.Push` NOT called. `handleBranchCompletion` logic runs (prompt queued check).

**11e. `workflow: clone, pr: false`** — `cloner.Clone` called, `cloner.Remove` called, `brancher.Push` called, PR creator NOT called. Prompt moved to completed.

**11f. `workflow: clone, pr: true`** — `cloner.Clone` called, `cloner.Remove` called, `brancher.Push` called, PR created. Regression test (behavior unchanged).

**11g. `handleAfterIsolatedCommit` shared helper test (unit)** — directly invoke `handleAfterIsolatedCommit` with mocked dependencies and assert:
- When `p.pr = false`: `prCreator.CreateCallCount() == 0` (PR helper not invoked)
- When `p.pr = true`: `prCreator.CreateCallCount() == 1` (PR helper invoked exactly once)
- In both cases: `brancher.PushCallCount() == 1` (branch always pushed after isolated commit)

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- **FREEZE `pkg/git/cloner.go`** — no changes permitted.
- **FREEZE `pkg/executor/executor.go`** — the `--tmpfs` flag was added in prompt 2; no further changes here.
- Wrap all non-nil errors with `errors.Wrap` / `errors.Wrapf` / `errors.Errorf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`.
- Error messages: lowercase, no file paths.
- No `fmt.Errorf`, no `errors.New`, no bare `return err` in production code.
- `worktreer.Remove` returns `nil` per interface contract — the `if err != nil` check in `cleanupIsolationOnError` is nominal but harmless.
- The `gitCtx` / `ctx` dual-context pattern in `handlePostExecution` must be preserved exactly as in the existing code. Do not change context construction.
- All `mocks.FakeWorktreer` usage in tests must use the Counterfeiter fake generated in prompt 2 (`mocks/worktreer.go`).
- Existing tests must still pass.
- Do not touch `go.mod` / `go.sum` / `vendor/`.
- The `pr bool` parameter in `NewProcessor` stays — it is NOT removed. It remains the delivery flag, orthogonal to `workflow`.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional checks:
1. `grep -n "p\.worktree\b" pkg/processor/processor.go` — must return zero matches (all `p.worktree` references removed).
2. `grep -n "worktree bool" pkg/processor/processor.go` — must return zero matches.
3. `grep -n "worktree bool" pkg/factory/factory.go` — must return zero matches.
4. `grep -rn "WorkflowWorktree\|WorkflowBranch\|WorkflowClone\|WorkflowDirect" pkg/processor/processor.go` — must return at least 4 matches (the switch cases).
5. `grep -n "NewWorktreer\(\)" pkg/factory/factory.go` — must return one match (injected by `CreateProcessor`).
</verification>
