---
status: approved
spec: [049-split-processor-workflow]
created: "2026-04-16T19:34:55Z"
queued: "2026-04-16T21:01:39Z"
---

<summary>
- Four concrete `WorkflowExecutor` implementations are added to `pkg/processor/`: `directWorkflowExecutor`, `branchWorkflowExecutor`, `cloneWorkflowExecutor`, and `worktreeWorkflowExecutor`, each in its own file
- All workflow-specific methods extracted from `processor.go` into the concrete executors: `setupWorkflow`, `setupCloneWorkflowState`, `setupWorktreeWorkflowState`, `setupInPlaceBranchState`, `setupCloneForExecution`, `cleanupIsolationOnError`, `handlePostExecution` (the dispatch), `handleCloneWorkflow`, `handleWorktreeWorkflow`, `handleDirectWorkflow`, `handleBranchCompletion`, `handleBranchPRCompletion`, `handleAfterIsolatedCommit`, `handleAutoMergeForClone`, `findOrCreatePR`, `savePRURLToFrontmatter`, `postMergeActions`, `restoreDefaultBranch`, `reconstructWorkflowState`
- The `processor` struct loses five git-dependency fields (`brancher`, `prCreator`, `cloner`, `worktreer`, `prMerger`) and the `workflow`, `pr`, `autoMerge`, `autoReview`, `autoRelease` fields; gains a single `workflowExecutor WorkflowExecutor` field
- `processPrompt` delegates to `executor.Setup`, `defer executor.CleanupOnError`, `executor.Complete` — no `switch p.workflow` or `if p.pr` branches remain
- `resumePrompt` delegates to `executor.ReconstructState` instead of calling `reconstructWorkflowState` directly
- `NewProcessor` loses brancher/prCreator/cloner/worktreer/prMerger/workflow/pr/autoMerge/autoReview/autoRelease parameters; gains a single `workflowExecutor WorkflowExecutor` parameter
- `pkg/processor/processor.go` no longer references the git workflow types `git.Brancher`, `git.Cloner`, `git.PRCreator`, `git.PRMerger`, `git.Worktreer` — only `git.Releaser` remains (used by `enrichPromptContent`). Note: all these types live in the flat `pkg/git/` package (no subpackages).
- The factory creates the correct executor based on `cfg.Workflow` and passes it to `NewProcessor`; `CreateProcessor` signature is updated accordingly
- Each concrete executor has its own `*_test.go` file with ≥80% statement coverage; processor tests are updated for the simplified constructor; factory tests assert correct executor type selection
- `make precommit` passes
</summary>

<objective>
Implement the three-step extraction: (1) create four concrete `WorkflowExecutor` types containing all workflow-specific git logic currently embedded in `processor.go`, (2) refactor the `processor` struct to delegate to the executor interface rather than holding five git dependencies directly, (3) update the factory to construct and inject the right executor based on config. This prompt depends on prompt 1 having added `WorkflowExecutor` interface and `WorkflowDeps` struct to `pkg/processor/workflow_executor.go`.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `docs/workflows.md` for the behavioral contract of each workflow mode.

Files to read in full before editing:
- `pkg/processor/workflow_executor.go` — the interface and WorkflowDeps from prompt 1
- `mocks/workflow_executor.go` — the FakeWorkflowExecutor generated in prompt 1
- `pkg/processor/processor.go` — the FULL file; key sections:
  - `NewProcessor` (line ~49)
  - `processor` struct (line ~131)
  - `resumePrompt` (~270) and `reconstructWorkflowState` (~402)
  - `processPrompt` (~913)
  - `workflowState` struct (~1100) and `cleanupIsolationOnError` (~1113)
  - `handlePostExecution` (~1141)
  - `moveToCompletedAndCommit` (~1206)
  - `setupWorkflow` (~1235) and all setup*WorkflowState helpers (~1259–1372)
  - `postMergeActions` (~1448)
  - `handleAfterIsolatedCommit` (~1558)
  - `handleCloneWorkflow` (~1615)
  - `handleWorktreeWorkflow` (~1658)
  - `setupCloneForExecution` (~1701)
  - `handleDirectWorkflow` (~1725)
  - `handleBranchCompletion` (~1776)
  - `handleBranchPRCompletion` (~1817)
  - `findOrCreatePR` (~1489)
  - `savePRURLToFrontmatter` (~1957)
  - `restoreDefaultBranch` (~1336)
- `pkg/processor/processor_test.go` — understand current test setup (constructor args, BeforeEach)
- `pkg/factory/factory.go` — `CreateProcessor` function (~566) and both call sites in `CreateRunner` (~303) and `CreateOneShotRunner` (~397)
- `pkg/factory/factory_test.go` — existing `CreateProcessor` test (~57)
- `pkg/config/workflow.go` — the four `WorkflowDirect/Branch/Clone/Worktree` constants
</context>

<requirements>

## 1. Create four concrete WorkflowExecutor implementations

Each file lives in `pkg/processor/` (same package as `processor.go`). Each file starts with the copyright header. Each type implements the `WorkflowExecutor` interface (four methods: `Setup`, `CleanupOnError`, `Complete`, `ReconstructState`).

All four implementations use `WorkflowDeps` (defined in `workflow_executor.go`) to hold their dependencies. Each has an unexported struct that embeds or contains `WorkflowDeps` plus any internal state fields (paths, branch names) accumulated by `Setup` and used by `CleanupOnError`/`Complete`/`ReconstructState`.

**Shared helpers**: `moveToCompletedAndCommit`, `findOrCreatePR`, `savePRURLToFrontmatter`, `postMergeActions`, `handleAutoMergeForClone`, and `handleAfterIsolatedCommit` are currently methods on `processor`. These are used by multiple concrete executors. Extract them into **package-level functions** in a new file `pkg/processor/workflow_helpers.go` so all four executors can call them without duplication. Their signatures become:

```go
// moveToCompletedAndCommit moves the prompt to completed/, triggers spec auto-complete, and commits.
func moveToCompletedAndCommit(
    ctx context.Context,
    gitCtx context.Context,
    deps WorkflowDeps,
    pf *prompt.PromptFile,
    promptPath string,
    completedPath string,
) error

// findOrCreatePR checks for an existing open PR on the branch, creates one if absent.
func findOrCreatePR(
    gitCtx context.Context,
    ctx context.Context,
    deps WorkflowDeps,
    branchName string,
    title string,
    issue string,
) (string, error)

// savePRURLToFrontmatter saves the PR URL to the prompt frontmatter.
func savePRURLToFrontmatter(gitCtx context.Context, promptPath string, prURL string)

// postMergeActions switches to default branch, pulls, and optionally releases.
func postMergeActions(gitCtx context.Context, ctx context.Context, deps WorkflowDeps, title string) error

// handleAutoMergeForClone defers or immediately merges the PR based on remaining queued prompts.
func handleAutoMergeForClone(
    gitCtx context.Context,
    ctx context.Context,
    deps WorkflowDeps,
    pf *prompt.PromptFile,
    branchName string,
    promptPath string,
    completedPath string,
    prURL string,
    title string,
) error

// handleAfterIsolatedCommit handles push + optional PR + prompt lifecycle for clone/worktree.
func handleAfterIsolatedCommit(
    gitCtx context.Context,
    ctx context.Context,
    deps WorkflowDeps,
    pf *prompt.PromptFile,
    branchName string,
    title string,
    promptPath string,
    completedPath string,
) error
```

Also keep `buildPRBody` as a package-level function in `workflow_helpers.go` (it was already package-level).

### 1a. `pkg/processor/workflow_executor_direct.go`

The direct executor handles `WorkflowDirect`: no isolation, commit in-place, push with optional release.

```go
type directWorkflowExecutor struct {
    deps WorkflowDeps
}

// NewDirectWorkflowExecutor creates a WorkflowExecutor for the direct workflow.
func NewDirectWorkflowExecutor(deps WorkflowDeps) WorkflowExecutor {
    return &directWorkflowExecutor{deps: deps}
}
```

- `Setup`: no-op (returns nil immediately — direct workflow needs no pre-execution setup)
- `CleanupOnError`: no-op
- `Complete`: extracts logic from `handlePostExecution` for the `WorkflowDirect` path (the case where `featureBranch == ""`). Calls `moveToCompletedAndCommit`, then calls `handleDirectWorkflow` logic inline (or extract `handleDirectWorkflow` into the executor — see current `processor.handleDirectWorkflow` at line ~1725).
- `ReconstructState`: returns `(true, nil)` — direct workflow has no isolated state to reconstruct; always resumable.

### 1b. `pkg/processor/workflow_executor_branch.go`

The branch executor handles `WorkflowBranch`: create/switch branch in-place before execution, restore after.

```go
type branchWorkflowExecutor struct {
    deps                 WorkflowDeps
    // Internal state set by Setup, used by CleanupOnError/Complete
    inPlaceBranch        string // feature branch name (empty = run directly on current branch)
    inPlaceDefaultBranch string // branch to restore after execution
}

// NewBranchWorkflowExecutor creates a WorkflowExecutor for the branch workflow.
func NewBranchWorkflowExecutor(deps WorkflowDeps) WorkflowExecutor {
    return &branchWorkflowExecutor{deps: deps}
}
```

- `Setup`: if `pf.Branch()` is empty, no-op (no branch to switch to). Otherwise, calls `setupInPlaceBranchState` logic (extracted from `processor.setupInPlaceBranchState` at line ~1292). Sets `inPlaceBranch` and `inPlaceDefaultBranch` on the executor struct.
- `CleanupOnError`: if `inPlaceDefaultBranch != ""`, calls `brancher.Switch(ctx, inPlaceDefaultBranch)` to restore the branch (log warning on failure, do not return error — same as `processor.restoreDefaultBranch`).
- `Complete`: extracts the `WorkflowBranch` path from `handlePostExecution` (~1181–1201). Calls `moveToCompletedAndCommit`, calls `handleDirectWorkflow` logic, restores default branch, then (if `inPlaceBranch != ""`): for `pr=true` calls `handleBranchPRCompletion` logic; for `pr=false` calls `handleBranchCompletion` logic.
- `ReconstructState`: returns `(&workflowState{}, true, nil)` — branch workflow has no isolated directory. Always resumable. (Since `workflowState` is no longer needed by the processor after the refactor, `ReconstructState` just returns `(true, nil)` for branch.)

### 1c. `pkg/processor/workflow_executor_clone.go`

The clone executor handles `WorkflowClone`: clone repo into a temp dir, run YOLO there, commit and push from clone, then clean up.

```go
type cloneWorkflowExecutor struct {
    deps         WorkflowDeps
    // Internal state set by Setup
    branchName   string
    clonePath    string
    originalDir  string
    cleanedUp    bool
}

// NewCloneWorkflowExecutor creates a WorkflowExecutor for the clone workflow.
func NewCloneWorkflowExecutor(deps WorkflowDeps) WorkflowExecutor {
    return &cloneWorkflowExecutor{deps: deps}
}
```

- `Setup`: moves logic from `setupCloneWorkflowState` + `setupCloneForExecution` (lines ~1354–1370, ~1701–1724). Sets `branchName`, `clonePath`, `originalDir` on the executor. On error, calls `deps.Cloner.Remove` best-effort before returning.
- `CleanupOnError`: if `!cleanedUp && clonePath != ""`, calls `os.Chdir(originalDir)` (warn on failure), then `deps.Cloner.Remove(ctx, clonePath)` (warn on failure). Sets `cleanedUp = true`.
- `Complete`: moves logic from `handleCloneWorkflow` (~1615). Commits in clone via `deps.Releaser.CommitOnly`, chdirs back, removes clone, sets `cleanedUp = true`, then calls `handleAfterIsolatedCommit`.
- `ReconstructState`: checks `os.Stat(clonePath)` — if missing, returns `(false, nil)`. If present, sets `branchName`, `clonePath`, `originalDir` and returns `(true, nil)`. Mirrors the `config.WorkflowClone` case in `processor.reconstructWorkflowState` (~408–427).

### 1d. `pkg/processor/workflow_executor_worktree.go`

The worktree executor handles `WorkflowWorktree`: add a git worktree, run YOLO there, commit and push from worktree, then remove it.

```go
type worktreeWorkflowExecutor struct {
    deps          WorkflowDeps
    // Internal state set by Setup
    branchName    string
    worktreePath  string
    originalDir   string
    cleanedUp     bool
}

// NewWorktreeWorkflowExecutor creates a WorkflowExecutor for the worktree workflow.
func NewWorktreeWorkflowExecutor(deps WorkflowDeps) WorkflowExecutor {
    return &worktreeWorkflowExecutor{deps: deps}
}
```

- `Setup`: moves logic from `setupWorktreeWorkflowState` (~1259). Sets `branchName`, `worktreePath`, `originalDir` on executor. On error calling `worktreer.Add`, returns error. On error calling `os.Chdir`, calls `worktreer.Remove` best-effort and returns error.
- `CleanupOnError`: if `!cleanedUp && worktreePath != ""`, calls `os.Chdir(originalDir)` (warn on failure), then `deps.Worktreer.Remove(ctx, worktreePath)` (Remove never errors per contract). Sets `cleanedUp = true`.
- `Complete`: moves logic from `handleWorktreeWorkflow` (~1658). Commits in worktree via `deps.Releaser.CommitOnly`, chdirs back, removes worktree, sets `cleanedUp = true`, then calls `handleAfterIsolatedCommit`.
- `ReconstructState`: checks `os.Stat(worktreePath)` — if missing, returns `(false, nil)`. If present, sets `branchName`, `worktreePath`, `originalDir` and returns `(true, nil)`. Mirrors the `config.WorkflowWorktree` case in `processor.reconstructWorkflowState` (~428–451).

**Note on `workflowState`:** After this prompt, `workflowState` is no longer needed by the processor. Remove it from `processor.go` unless it is still referenced by any test. If tests reference it directly, refactor those tests to use `FakeWorkflowExecutor` instead.

## 2. Refactor `pkg/processor/processor.go`

### 2a. Remove from `processor` struct

Delete these fields:
```
workflow      config.Workflow
pr            bool
brancher      git.Brancher
prCreator     git.PRCreator
cloner        git.Cloner
worktreer     git.Worktreer
prMerger      git.PRMerger
autoMerge     bool
autoReview    bool
autoRelease   bool
```

### 2b. Add to `processor` struct

```go
workflowExecutor WorkflowExecutor
```

Place it after `ready <-chan struct{}` in the field list.

### 2c. Update `NewProcessor` signature

Remove parameters: `pr bool`, `workflow config.Workflow`, `brancher git.Brancher`, `prCreator git.PRCreator`, `cloner git.Cloner`, `worktreer git.Worktreer`, `prMerger git.PRMerger`, `autoMerge bool`, `autoReview bool`, `autoRelease bool`.

Add parameter: `workflowExecutor WorkflowExecutor` (after `ready <-chan struct{}`).

Wire it: `workflowExecutor: workflowExecutor` in the return struct.

**Do NOT remove** `releaser git.Releaser` — it is still used by `enrichPromptContent` (the `HasChangelog` call). Do NOT remove `versionGetter version.Getter` — still used by `preparePromptForExecution`. Do NOT remove `validationCommand`, `validationPrompt`, `testCommand`, `verificationGate`, `autoCompleter`, `specLister`, etc. — those are NOT workflow-specific.

### 2d. Update `processPrompt`

Replace the current workflow setup/cleanup/dispatch block:

**Before** (around line 953–982):
```go
workflowState, err := p.setupWorkflow(ctx, baseName, pf)
if err != nil {
    return errors.Wrap(ctx, err, "setup workflow")
}
if (p.workflow == config.WorkflowClone || p.workflow == config.WorkflowWorktree) &&
    (workflowState.clonePath != "" || workflowState.worktreePath != "") {
    defer p.cleanupIsolationOnError(ctx, workflowState)
}
// ... container slot prep ...
return p.handlePostExecution(ctx, pf, pr.Path, title, logFile, workflowState)
```

**After:**
```go
if err := p.workflowExecutor.Setup(ctx, baseName, pf); err != nil {
    return errors.Wrap(ctx, err, "setup workflow")
}
defer p.workflowExecutor.CleanupOnError(ctx)

// ... container slot prep (UNCHANGED) ...

cancelled, execErr := p.runContainer(ctx, content, logFile, containerName, pr.Path)
if cancelled {
    return nil
}
if execErr != nil {
    return execErr
}

gitCtx := context.WithoutCancel(ctx)
completedPath := filepath.Join(p.completedDir, filepath.Base(pr.Path))

// Verification gate: pause before git operations if enabled
if p.verificationGate {
    return p.enterPendingVerification(ctx, pf, pr.Path)
}

// Validate completion report from log
completionReport, err := validateCompletionReport(ctx, logFile)
if err != nil {
    p.notifyFromReport(ctx, logFile, pr.Path)
    return errors.Wrap(ctx, err, "validate completion report")
}
// Store summary in frontmatter before moving to completed
if completionReport != nil && completionReport.Summary != "" {
    pf.SetSummary(completionReport.Summary)
    if err := pf.Save(ctx); err != nil {
        return errors.Wrap(ctx, err, "save summary")
    }
}

return p.workflowExecutor.Complete(gitCtx, ctx, pf, title, pr.Path, completedPath)
```

**Important:** `handlePostExecution` contained both (a) the validation report check / summary save, and (b) the workflow dispatch. After refactoring:
- Part (a) moves into `processPrompt` directly (shown above).
- Part (b) is replaced by `executor.Complete(...)`.
- `handlePostExecution` is deleted entirely.

### 2e. Update `resumePrompt`

Replace `reconstructWorkflowState` call with executor delegation.

**Before** (around line 304–345):
```go
ws, ok, err := p.reconstructWorkflowState(ctx, baseName, pf)
if err != nil {
    return errors.Wrap(ctx, err, "reconstruct workflow state")
}
if !ok {
    // ... reset to approved ...
}
// ... chdir into ws.clonePath ...
return p.handlePostExecution(ctx, pf, promptPath, title, logFile, ws)
```

**After:**
```go
canResume, err := p.workflowExecutor.ReconstructState(ctx, baseName, pf)
if err != nil {
    return errors.Wrap(ctx, err, "reconstruct workflow state for resume")
}
if !canResume {
    slog.Warn("cannot resume prompt: isolation directory missing; resetting to approved",
        "file", filepath.Base(promptPath))
    pf.MarkApproved()
    if err := pf.Save(ctx); err != nil {
        return errors.Wrap(ctx, err, "save prompt after failed resume")
    }
    return nil
}

gitCtx := context.WithoutCancel(ctx)
completedPath := filepath.Join(p.completedDir, filepath.Base(promptPath))
return p.workflowExecutor.Complete(gitCtx, ctx, pf, title, promptPath, completedPath)
```

Read the current `resumePrompt` carefully (line ~270–346) to ensure all the container-reattach logic (reattach timeout, log file, `runContainer` call) is preserved. Only the `reconstructWorkflowState` + `handlePostExecution` calls at the end change.

### 2f. Delete methods no longer needed on processor

Delete ALL of these methods from `processor.go`:
- `reconstructWorkflowState`
- `setupWorkflow`
- `setupCloneWorkflowState`
- `setupWorktreeWorkflowState`
- `setupInPlaceBranchState`
- `setupCloneForExecution`
- `restoreDefaultBranch`
- `cleanupIsolationOnError`
- `handlePostExecution`
- `handleCloneWorkflow`
- `handleWorktreeWorkflow`
- `handleDirectWorkflow`
- `handleBranchCompletion`
- `handleBranchPRCompletion`
- `handleAfterIsolatedCommit`
- `handleAutoMergeForClone`
- `findOrCreatePR`
- `buildPRBody`
- `savePRURLToFrontmatter`
- `postMergeActions`
- `workflowState` struct

Also remove the `config` package import if `config.Workflow*` constants are no longer referenced in `processor.go`. The git workflow types (`Brancher`, `Cloner`, `PRCreator`, `PRMerger`, `Worktreer`) all live in the flat `pkg/git/` package — they share the single `"github.com/bborbe/dark-factory/pkg/git"` import with `git.Releaser`, so do NOT remove that import. Only remove struct fields and constructor params that reference those types.

**Keep:**
- `git.Releaser` import and field (used by `enrichPromptContent`, `syncWithRemote`)
- `version.Getter` import and field (used by `preparePromptForExecution`)
- All non-workflow methods (`processExistingQueued`, `Process`, `ProcessQueue`, `enrichPromptContent`, `checkPreflightConditions`, `syncWithRemote`, `prepareContainerSlot`, `startContainerLockRelease`, `runContainer`, `watchForCancellation`, `notifyFailed`, `notifyFromReport`, `checkPromptedSpecs`, `waitForContainerSlot`, `hasFreeSlot`, `checkDirtyFileThreshold`, `checkGitIndexLock`, `logBlockedOnce`, `shouldSkipPrompt`, `autoSetQueuedStatus`, `handleProcessError`, `checkPostExecutionFailure`, `handlePromptFailure`, `enterPendingVerification`, `hasPendingVerification`, `handleEmptyPrompt`, `killTimedOutContainer`, `computeReattachDuration`)

## 3. Update `pkg/factory/factory.go`

### 3a. Update `CreateProcessor` signature

Remove parameters that are now bundled into the executor:
- `workflow config.Workflow`
- `pr bool`
- `brancher git.Brancher`
- `prCreator git.PRCreator`
- `prMerger git.PRMerger`
- `autoMerge bool`
- `autoReview bool`
- `autoRelease bool`
- `hideGit bool` (was used to construct executor; now derived from workflow type)

Add parameter:
```go
workflowExecutor processor.WorkflowExecutor
```
(after `ready <-chan struct{}`).

Pass `workflowExecutor` to `processor.NewProcessor`.

### 3b. Create `CreateWorkflowExecutor` factory function

Add a new factory function in `pkg/factory/factory.go` that selects and creates the correct executor:

```go
// CreateWorkflowExecutor creates the WorkflowExecutor for the given workflow configuration.
func CreateWorkflowExecutor(
    cfg config.Config,
    projectName string,
    promptManager prompt.Manager,
    autoCompleter spec.AutoCompleter,
    releaser git.Releaser,
    versionGetter version.Getter,
    brancher git.Brancher,
    prCreator git.PRCreator,
    prMerger git.PRMerger,
) processor.WorkflowExecutor {
    deps := processor.WorkflowDeps{
        ProjectName:   projectName,
        PromptManager: promptManager,
        AutoCompleter: autoCompleter,
        Releaser:      releaser,
        VersionGetter: versionGetter,
        Brancher:      brancher,
        PRCreator:     prCreator,
        Cloner:        git.NewCloner(),
        Worktreer:     git.NewWorktreer(),
        PRMerger:      prMerger,
        PR:            cfg.PR,
        AutoMerge:     cfg.AutoMerge,
        AutoReview:    cfg.AutoReview,
        AutoRelease:   cfg.AutoRelease,
    }
    switch cfg.Workflow {
    case config.WorkflowBranch:
        return processor.NewBranchWorkflowExecutor(deps)
    case config.WorkflowClone:
        return processor.NewCloneWorkflowExecutor(deps)
    case config.WorkflowWorktree:
        return processor.NewWorktreeWorkflowExecutor(deps)
    default: // WorkflowDirect
        return processor.NewDirectWorkflowExecutor(deps)
    }
}
```

### 3c. Update `createDockerExecutor` call sites

The `hideGit` parameter was passed to `createDockerExecutor` as `workflow == config.WorkflowWorktree || hideGit`. After this prompt, the factory call sites must compute this without the old `hideGit` parameter:
```go
createDockerExecutor(
    containerImage, projectName, model, netrcFile,
    gitconfigFile, env, extraMounts, claudeDir, maxPromptDuration,
    currentDateTimeGetter,
    cfg.Workflow == config.WorkflowWorktree, // worktreeMode
)
```

### 3d. Update both `CreateProcessor` call sites in `CreateRunner` and `CreateOneShotRunner`

Both call sites (~line 303 and ~397) currently pass `cfg.Workflow, cfg.PR, deps.brancher, deps.prCreator, deps.prMerger, cfg.AutoMerge, cfg.AutoRelease, cfg.AutoReview, hideGit`. Replace with a single call to `CreateWorkflowExecutor`:

```go
workflowExecutor := CreateWorkflowExecutor(
    cfg,
    projectName,
    promptManager,
    createAutoCompleter(
        inProgressDir, completedDir,
        specsInboxDir, specsInProgressDir, specsCompletedDir,
        currentDateTimeGetter, projectName, n,
    ),
    releaser,
    versionGetter,
    deps.brancher,
    deps.prCreator,
    deps.prMerger,
)
proc := CreateProcessor(
    inProgressDir, completedDir, cfg.Prompts.LogDir, projectName,
    promptManager, releaser, versionGetter, ready,
    cfg.ContainerImage, cfg.Model, cfg.NetrcFile, cfg.GitconfigFile,
    workflowExecutor,
    // ... remaining params unchanged ...
)
```

**Important:** The `autoCompleter` that goes into `WorkflowDeps` is the same one passed to the processor. If it was previously constructed inside `CreateProcessor`, move its construction to the call site so it can be shared. Check `createAutoCompleter` call in the current `CreateProcessor` body (~line 631) and factor it out.

## 4. Tests

### 4a. Concrete executor tests

Create `pkg/processor/workflow_executor_direct_test.go`, `workflow_executor_branch_test.go`, `workflow_executor_clone_test.go`, `workflow_executor_worktree_test.go`. Use `package processor_test`. Follow the Ginkgo/Gomega pattern from `processor_test.go`.

For each executor, test:
- `Setup` happy path: correct internal state set, dependencies called correctly (use `mocks.FakeBrancher`, `mocks.FakeCloner`, `mocks.FakeWorktreer` as appropriate)
- `CleanupOnError`: if setup created state, cleanup is called; if setup was never called, no panic
- `Complete` happy path: dependencies called in correct order, prompt moved to completed
- `ReconstructState`: returns correct `canResume` for both the "state found" and "state missing" cases

For clone and worktree executors: use `GinkgoT().TempDir()` for the temp paths; create a minimal git repo (`git init && git commit --allow-empty -m init`) so `os.Getwd()`/`os.Chdir()` calls work.

For each test, use `mocks.FakeReleaser`, `mocks.FakeManager` (promptManager), and the git-specific mocks already in `mocks/`.

Minimum coverage target: ≥80% statement coverage per executor file. Check with:
```
cd /workspace && go test -coverprofile=/tmp/cover.out ./pkg/processor/... && go tool cover -func=/tmp/cover.out | grep workflow_executor
```

### 4b. Update `pkg/processor/processor_test.go`

The existing tests construct `processor.NewProcessor(...)` with ~35 args. After the refactor, `NewProcessor` takes `workflowExecutor WorkflowExecutor` instead of the five git deps + workflow/pr/autoMerge/autoReview/autoRelease.

Update every `processor.NewProcessor(...)` call in `processor_test.go`:
- Replace `pr bool, workflow config.Workflow, brancher, prCreator, cloner, worktreer, prMerger, autoMerge, autoReview, autoRelease` with `&mocks.FakeWorkflowExecutor{}`
- Remove the `brancher`, `prCreator`, `cloner`, `worktreer`, `prMerger`, `workflow`, `pr`, `autoMerge`, `autoReview`, `autoRelease` variables from `BeforeEach` (they are no longer needed for the processor; they are only needed for executor tests)
- Add `workflowExecutor *mocks.FakeWorkflowExecutor` to the test's variable block
- Initialize it in `BeforeEach`: `workflowExecutor = &mocks.FakeWorkflowExecutor{}`
- Tests that previously asserted behavior about specific git operations (e.g., "brancher.Push was called") must be updated to assert on `workflowExecutor.SetupCallCount()`, `workflowExecutor.CompleteCallCount()`, `workflowExecutor.CleanupOnErrorCallCount()` instead.

### 4c. Update `pkg/factory/factory_test.go`

Update the `CreateProcessor` test to:
1. Pass a `FakeWorkflowExecutor` (or the result of `CreateWorkflowExecutor`) instead of the old set of args.
2. Add a new `Describe("CreateWorkflowExecutor", ...)` block with table-driven tests for all four workflow values:
   - `WorkflowDirect` → returns `*directWorkflowExecutor` type (verify via type assertion or a dedicated interface check)
   - `WorkflowBranch` → returns `*branchWorkflowExecutor` type
   - `WorkflowClone` → returns `*cloneWorkflowExecutor` type
   - `WorkflowWorktree` → returns `*worktreeWorkflowExecutor` type

Since the concrete types are unexported, use a helper: add `export_test.go` to `pkg/processor/` with:
```go
package processor

// Exported type aliases for testing concrete WorkflowExecutor types.
type DirectWorkflowExecutorForTest = directWorkflowExecutor
type BranchWorkflowExecutorForTest = branchWorkflowExecutor
type CloneWorkflowExecutorForTest = cloneWorkflowExecutor
type WorktreeWorkflowExecutorForTest = worktreeWorkflowExecutor
```

Then in `factory_test.go`:
```go
It("direct workflow creates directWorkflowExecutor", func() {
    cfg.Workflow = config.WorkflowDirect
    ex := factory.CreateWorkflowExecutor(cfg, ...)
    _, ok := ex.(*processor.DirectWorkflowExecutorForTest)
    Expect(ok).To(BeTrue())
})
```

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- This prompt depends on prompt 1 already adding `WorkflowExecutor` interface and `WorkflowDeps` to `pkg/processor/workflow_executor.go`. If that file does not exist, STOP and report failure.
- **No behavioral changes.** Every existing `make test` case passes after this refactor. The only test changes are constructor-arg updates and switching from git-mock assertions to executor-mock assertions.
- **One cut-over only.** The processor must not carry both old and new code paths. All old workflow methods must be deleted in the same commit.
- `pkg/processor/processor.go` must NOT reference the types `git.Brancher`, `git.Cloner`, `git.PRCreator`, `git.PRMerger`, `git.Worktreer` after this change. (They live in the flat `pkg/git/` package; the package import stays because `git.Releaser` is still used.)
- Dual-context pattern preserved: `gitCtx := context.WithoutCancel(ctx)` is computed in `processPrompt` (not inside the executor), then passed to `executor.Complete(gitCtx, ctx, ...)`. The executor receives BOTH contexts.
- All errors wrapped with `errors.Wrap` / `errors.Wrapf` / `errors.Errorf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`.
- Error messages: lowercase, no file paths in the message string.
- Counterfeiter fakes: use `mocks.FakeWorkflowExecutor` for processor tests, NOT the concrete executor types directly.
- Concrete executor types are unexported structs. Constructors (`NewDirectWorkflowExecutor`, etc.) are exported functions returning `WorkflowExecutor`.
- Do not touch `go.mod` / `go.sum` / `vendor/`.
- The `verificationGate` check (`enterPendingVerification`) happens in `processPrompt` BEFORE calling `executor.Complete` — do not move it inside the executor.
- The `validateCompletionReport` + summary-save logic also stays in `processPrompt` BEFORE calling `executor.Complete`.
- `autoCompleter` stays on the processor struct AND in `WorkflowDeps` (both need it: processor for `checkPromptedSpecs`, executor for `moveToCompletedAndCommit`). Do NOT remove `autoCompleter` from `NewProcessor`.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional checks:
1. `grep -rn "p\.workflow\b\|p\.pr\b\|p\.brancher\b\|p\.cloner\b\|p\.worktreer\b\|p\.prCreator\b\|p\.prMerger\b\|p\.autoMerge\b\|p\.autoReview\b\|p\.autoRelease\b" pkg/processor/processor.go` — must return zero matches.
2. `grep -n "git\.Brancher\|git\.Cloner\|git\.PRCreator\|git\.PRMerger\|git\.Worktreer" pkg/processor/processor.go` — must return zero matches (the workflow git types live in the flat `pkg/git/` package; scoping them out by type name rather than subpackage path).
3. `grep -n "WorkflowExecutor" pkg/processor/processor.go` — must return at least one match (the field declaration).
4. `grep -rn "switch p\.workflow\|if p\.pr\b\|if p\.worktree\b" pkg/processor/processor.go` — must return zero matches.
5. `go test -coverprofile=/tmp/cover.out ./pkg/processor/... && go tool cover -func=/tmp/cover.out | grep workflow_executor` — each executor file ≥80% coverage.
6. `grep -n "CreateWorkflowExecutor" pkg/factory/factory.go` — must return at least one match.
</verification>
