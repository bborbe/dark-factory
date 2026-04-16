---
status: created
spec: [049-split-processor-workflow]
created: "2026-04-16T19:34:55Z"
branch: dark-factory/split-processor-workflow
---

<summary>
- A new `WorkflowExecutor` interface is added to `pkg/processor/` with four methods covering the full git-lifecycle contract: `Setup`, `CleanupOnError`, `Complete`, and `ReconstructState`
- A Counterfeiter fake (`FakeWorkflowExecutor`) is generated in `mocks/workflow_executor.go` via the `//counterfeiter:generate` annotation
- A `WorkflowDeps` struct is added to `pkg/processor/` holding all dependencies shared across concrete executor implementations: promptManager, releaser, versionGetter, autoCompleter, brancher, prCreator, cloner, worktreer, prMerger, and the behavior booleans (pr, autoMerge, autoReview, autoRelease, projectName)
- No existing code is changed — this prompt is purely additive: two new files, one generated file
- All existing tests continue to pass
- The interface design is verified to be sufficient for all four workflow variants (direct, branch, clone, worktree) by reading the existing processor methods it will eventually replace
</summary>

<objective>
Define the `WorkflowExecutor` interface and `WorkflowDeps` struct that will be used in the next prompt to extract all git-lifecycle logic from the processor. This prompt is purely additive — no existing code changes. Its output is the stable contract that the concrete implementations and the refactored processor will depend on.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `docs/workflows.md` for the behavioral contract of each workflow mode.

Files to read before writing:
- `pkg/processor/processor.go` — read ALL of: `processPrompt` (line ~913), `setupWorkflow` (~1235), `cleanupIsolationOnError` (~1113), `handlePostExecution` (~1141), `workflowState` struct (~1100), `reconstructWorkflowState` (~402), `handleCloneWorkflow` (~1615), `handleWorktreeWorkflow` (~1658), `handleDirectWorkflow` (~1725), `handleBranchCompletion` (~1776), `handleBranchPRCompletion` (~1817), `handleAfterIsolatedCommit` (~1558), `moveToCompletedAndCommit` (~1206), `postMergeActions` (~1448), `findOrCreatePR` (~1489), `savePRURLToFrontmatter` (~1957). These are the methods whose logic will move into concrete executors in the next prompt.
- `pkg/git/brancher.go` — Brancher interface
- `pkg/git/cloner.go` — Cloner interface
- `pkg/git/worktreer.go` — Worktreer interface
- `mocks/worktreer.go` — reference for Counterfeiter annotation format
- `mocks/cloner.go` — reference for Counterfeiter annotation format
</context>

<requirements>

## 1. Create `pkg/processor/workflow_executor.go`

Create a new file `pkg/processor/workflow_executor.go` with the following exact content (copyright header required):

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
    "context"

    "github.com/bborbe/dark-factory/pkg/git"
    "github.com/bborbe/dark-factory/pkg/prompt"
    "github.com/bborbe/dark-factory/pkg/spec"
    "github.com/bborbe/dark-factory/pkg/version"
)

//counterfeiter:generate -o ../../mocks/workflow_executor.go --fake-name WorkflowExecutor . WorkflowExecutor

// WorkflowExecutor handles the git lifecycle for a single prompt execution.
// It encapsulates the pre-execution environment setup and post-execution git
// operations for one workflow variant (direct, branch, clone, or worktree).
//
// Implementations are stateful: Setup stores paths and branches internally so
// that CleanupOnError and Complete can use them without the caller tracking
// workflowState.
type WorkflowExecutor interface {
    // Setup prepares the execution environment before the YOLO container runs.
    // For clone/worktree: creates the isolated directory and chdirs into it.
    // For branch: creates or switches to the feature branch in-place.
    // For direct: no-op.
    // Returns a wrapped error on failure; partial setup is cleaned up before returning.
    Setup(ctx context.Context, baseName string, pf *prompt.PromptFile) error

    // CleanupOnError undoes any environment setup performed by Setup when
    // execution or post-execution fails. Idempotent — safe to call if Setup was
    // not called or has already been cleaned up. No-op for direct and branch
    // executors where no isolated directory exists.
    CleanupOnError(ctx context.Context)

    // Complete performs all post-execution git operations after the YOLO container
    // exits successfully: commit, chdir back, cleanup isolation, push, optional PR
    // creation/merge, and moving the prompt file to completedPath.
    //
    // gitCtx is a non-cancellable context (context.WithoutCancel) for git ops.
    // ctx is the normal request context (used for prompt-manager calls and error wrapping).
    // completedPath is the destination path — the prompt has NOT been moved yet when
    // Complete is called; each implementation calls moveToCompleted internally.
    Complete(gitCtx context.Context, ctx context.Context, pf *prompt.PromptFile, title, promptPath, completedPath string) error

    // ReconstructState restores internal state for a prompt being resumed after a
    // daemon restart. Returns canResume=false if the isolated directory no longer
    // exists (caller resets the prompt to approved). Returns an error only for
    // unexpected filesystem failures.
    ReconstructState(ctx context.Context, baseName string, pf *prompt.PromptFile) (canResume bool, err error)
}

// WorkflowDeps holds all dependencies that WorkflowExecutor implementations may need.
// Factory functions populate only the fields required by the selected implementation;
// unused fields are nil and must not be dereferenced by implementations that do not
// need them.
type WorkflowDeps struct {
    ProjectName   string
    PromptManager prompt.Manager
    AutoCompleter spec.AutoCompleter
    Releaser      git.Releaser
    VersionGetter version.Getter
    Brancher      git.Brancher
    PRCreator     git.PRCreator
    Cloner        git.Cloner
    Worktreer     git.Worktreer
    PRMerger      git.PRMerger
    PR            bool
    AutoMerge     bool
    AutoReview    bool
    AutoRelease   bool
}
```

**Important notes:**
- The `//counterfeiter:generate` annotation path `../../mocks/workflow_executor.go` is correct relative to `pkg/processor/`. Verify by checking that `mocks/worktreer.go` uses the same relative path from `pkg/git/`.
- Do NOT add any concrete types or logic in this file — only the interface, deps struct, and the counterfeiter annotation.

## 2. Generate the Counterfeiter fake

After writing the file, run:
```
cd /workspace && go generate ./pkg/processor/...
```

This should produce `mocks/workflow_executor.go` with a `FakeWorkflowExecutor` type following the same pattern as `mocks/cloner.go` and `mocks/worktreer.go`.

If `go generate` does not pick up the annotation automatically (because there is no existing `generate.go` or `doc.go` in `pkg/processor/`), run the counterfeiter command directly:
```
cd /workspace && counterfeiter -o mocks/workflow_executor.go --fake-name WorkflowExecutor ./pkg/processor/. WorkflowExecutor
```

Verify the generated file exists:
```
ls -la /workspace/mocks/workflow_executor.go
```

## 3. Verify no existing files were changed

After writing the new file and generating the fake, confirm that only new files were created and no existing files were modified:
```
git -C /workspace diff --name-only
```
The output must be empty (no modified tracked files). Only untracked files should appear in `git status`.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- This prompt is PURELY ADDITIVE — do not modify any existing file.
- The `WorkflowExecutor` interface must have exactly the four methods specified: `Setup`, `CleanupOnError`, `Complete`, `ReconstructState`.
- `WorkflowDeps` must be exported (capital W) — it is referenced from `pkg/factory/factory.go` in the next prompt.
- Do NOT add concrete implementations in this prompt — only the interface definition and deps struct.
- The counterfeiter annotation must use the exact format: `//counterfeiter:generate -o ../../mocks/workflow_executor.go --fake-name WorkflowExecutor . WorkflowExecutor`
- Do not touch `go.mod` / `go.sum` / `vendor/`.
- Error messages in GoDoc comments: lowercase, no file paths in the message body.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional checks:
1. `ls mocks/workflow_executor.go` — fake exists.
2. `grep -n "FakeWorkflowExecutor" mocks/workflow_executor.go` — fake type is present.
3. `git -C /workspace diff --name-only` — output is empty (no existing files modified).
4. `grep -n "WorkflowDeps" pkg/processor/workflow_executor.go` — struct is present.
</verification>
