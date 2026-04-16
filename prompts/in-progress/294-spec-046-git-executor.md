---
status: approved
spec: [046-workflow-enum-with-worktree-mode]
created: "2026-04-16T12:00:00Z"
queued: "2026-04-16T15:00:47Z"
branch: dark-factory/workflow-enum-with-worktree-mode
---

<summary>
- A new `Worktreer` interface is added to `pkg/git/` with two methods: `Add` (runs `git worktree add`) and `Remove` (runs `git worktree remove --force`)
- The implementation runs the actual `git` binary — no `go-git`, no shell wrapper — matching the pattern used by `Cloner`
- A Counterfeiter fake is generated in `mocks/worktreer.go` via the `//counterfeiter:generate` annotation
- `pkg/executor/executor.go` gains a `hideGitDir bool` field that causes `buildDockerCommand` to append `--tmpfs /workspace/.git` when true
- `NewDockerExecutor` gains a trailing `worktreeMode bool` parameter so callers can opt into `--tmpfs`
- The factory helper `createDockerExecutor` is updated to accept and forward the new flag
- A unit test asserts that the docker args diff between today and after this change is exactly one line (`--tmpfs /workspace/.git`) added only when `worktreeMode == true`, and that the baseline args for `direct`/`branch`/`clone` modes are byte-for-byte identical to today
- `make precommit` passes
</summary>

<objective>
Implement the two git-layer primitives that `workflow: worktree` needs — the `Worktreer` interface for `git worktree add/remove` and the executor `--tmpfs /workspace/.git` flag that hides the `.git` directory from the YOLO container. No processor changes are in scope; this prompt delivers the raw building blocks that prompt 3 will wire up.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions (errors, Ginkgo/Gomega, Counterfeiter).
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `docs/workflows.md` for the behavioral contract of `workflow: worktree`.

Files to read before editing:
- `pkg/git/cloner.go` — model for the new `Worktreer` struct (same package, same pattern: interface + `New*` + private struct)
- `pkg/executor/executor.go` — `NewDockerExecutor`, `dockerExecutor` struct, `buildDockerCommand` (around line 419)
- `pkg/factory/factory.go` — `createDockerExecutor` helper (look for the function that calls `executor.NewDockerExecutor`)
- `pkg/executor/executor_test.go` — understand existing executor test structure
</context>

<requirements>

## 1. Add `pkg/git/worktreer.go`

Create a new file following the same structure as `cloner.go`. Include:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
    "context"
    "log/slog"
    "os/exec"
    "strings"

    "github.com/bborbe/errors"
)

//counterfeiter:generate -o ../../mocks/worktreer.go --fake-name Worktreer . Worktreer

// Worktreer handles git worktree operations.
type Worktreer interface {
    // Add creates a linked worktree at worktreePath on branch.
    // If the branch does not yet exist, it is created from the current HEAD.
    // Returns a wrapped error on failure (e.g. branch already checked out elsewhere).
    Add(ctx context.Context, worktreePath string, branch string) error

    // Remove removes the linked worktree at worktreePath.
    // Uses --force to handle cases where the worktree is in an unclean state.
    // Failure is logged as a warning but does NOT return an error (callers treat
    // cleanup failure as non-fatal, per spec constraint).
    Remove(ctx context.Context, worktreePath string) error
}

// NewWorktreer creates a new Worktreer.
func NewWorktreer() Worktreer {
    return &worktreer{}
}

// worktreer implements Worktreer.
type worktreer struct{}
```

### `Add` implementation

```go
// Add creates a linked worktree at worktreePath on branch.
// If branch already exists locally, checks it out into the new worktree.
// If branch does not exist, creates it from the current HEAD.
func (w *worktreer) Add(ctx context.Context, worktreePath string, branch string) error {
    slog.Info("adding worktree", "path", worktreePath, "branch", branch)

    // Check if branch exists locally — decides whether we pass -b (create) or not (checkout existing).
    check := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
    branchExists := check.Run() == nil

    // #nosec G204 -- worktreePath and branch are derived from config and prompt filename
    var cmd *exec.Cmd
    if branchExists {
        cmd = exec.CommandContext(ctx, "git", "worktree", "add", worktreePath, branch)
    } else {
        cmd = exec.CommandContext(ctx, "git", "worktree", "add", "-b", branch, worktreePath)
    }

    var stderr strings.Builder
    cmd.Stderr = &stderr
    if err := cmd.Run(); err != nil {
        return errors.Wrapf(
            ctx,
            err,
            "git worktree add (path=%s branch=%s exists=%t): %s",
            worktreePath,
            branch,
            branchExists,
            stderr.String(),
        )
    }
    return nil
}
```

**Behavior:** Branch pre-check decides between `git worktree add -b <branch> <path>` (new branch) and `git worktree add <path> <branch>` (existing branch). Either path still fails if the branch is already checked out in another worktree or in the parent repo — the caller (processor) treats that as a hard failure (prompt marked failed, human resolves manually). Do NOT add further recovery logic here.

### `Remove` implementation

```go
// Remove removes the linked worktree at worktreePath.
func (w *worktreer) Remove(ctx context.Context, worktreePath string) error {
    slog.Debug("removing worktree", "path", worktreePath)
    // #nosec G204 -- worktreePath is derived from config and prompt filename
    cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", worktreePath)
    var stderr strings.Builder
    cmd.Stderr = &stderr
    if err := cmd.Run(); err != nil {
        slog.Warn("git worktree remove failed", "path", worktreePath, "error", err, "stderr", stderr.String())
    }
    return nil
}
```

Note: `Remove` always returns `nil`. The spec requires that cleanup failure "log a warning and proceed — never block the queue." The `Worktreer` interface contract matches this: Remove never returns an error.

**Interface asymmetry with `Cloner.Remove` is intentional.** `Cloner.Remove` returns `error` because removing a clone is a plain `os.RemoveAll` that may fail in cases the caller wants to surface (e.g. test cleanup asserting success). `Worktreer.Remove` wraps `git worktree remove --force` whose failure is always non-fatal in production — callers MUST NOT treat it as blocking. Keeping the signature `error`-returning (but always nil) preserves substitutability-by-shape for future consolidation; the concrete nil-return is the contract.

## 2. Generate the Counterfeiter fake

After writing the file, run:
```
cd /workspace && go generate ./pkg/git/...
```

This should produce `mocks/worktreer.go` with `FakeWorktreer` (same pattern as the existing `mocks/cloner.go`).

## 3. Add unit tests for `Worktreer` — `pkg/git/worktreer_test.go`

Create `pkg/git/worktreer_test.go` in the external test package (`package git_test`) following the pattern of `cloner_test.go`. The real `git worktree` commands require an actual git repo, which is complex to set up in unit tests. Write **two** lightweight test cases that verify the interface contract without needing a real git repo:

1. **`Add` with a missing git repo → returns non-nil wrapped error** — call `Add` on a temp directory that is NOT a git repo. Assert the returned error is non-nil and `errors.Is` or string-contains "worktree add" context.
2. **`Remove` on a non-existent path → returns nil** — call `Remove` on a path that does not exist. Assert the returned error is nil (per the contract: Remove never errors).

Use `GinkgoT().TempDir()` for temp paths.

## 4. Update `pkg/executor/executor.go`

### 4a. Add `hideGitDir bool` to `dockerExecutor`

Add the field at the bottom of the `dockerExecutor` struct:
```go
hideGitDir bool
```

### 4b. Update `NewDockerExecutor` signature

Add `worktreeMode bool` as the **last** parameter:

```go
func NewDockerExecutor(
    containerImage string,
    projectName string,
    model string,
    netrcFile string,
    gitconfigFile string,
    env map[string]string,
    extraMounts []config.ExtraMount,
    claudeDir string,
    maxPromptDuration time.Duration,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    worktreeMode bool,
) Executor {
    return &dockerExecutor{
        // ... all existing fields ...
        hideGitDir: worktreeMode,
    }
}
```

**FREEZE all other fields and logic in `NewDockerExecutor`** — only add the parameter and set the field.

### 4c. Add `--tmpfs /workspace/.git` in `buildDockerCommand`

In `buildDockerCommand`, after the existing `-v` mounts block (specifically after the `extraMounts` loop and before `args = append(args, e.containerImage)`), add ONE new conditional:

```go
if e.hideGitDir {
    args = append(args, "--tmpfs", "/workspace/.git")
}
```

This must be exactly before `args = append(args, e.containerImage)` so the flag position is consistent and testable. No other lines in `buildDockerCommand` are changed.

## 5. Update all `NewDockerExecutor` call sites in `pkg/factory/factory.go`

There are **exactly two** call sites (verified by `grep -n "NewDockerExecutor" pkg/factory/factory.go`):
- Line ~454 inside `CreateSpecGenerator` — direct call
- Line ~537 inside `createDockerExecutor` helper

### 5a. Update `createDockerExecutor` helper

Add `worktreeMode bool` as a parameter and pass it through to `NewDockerExecutor`:

```go
func createDockerExecutor(
    containerImage, projectName, model, netrcFile, gitconfigFile string,
    env map[string]string,
    extraMounts []config.ExtraMount,
    claudeDir string,
    maxPromptDuration time.Duration,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    worktreeMode bool,
) executor.Executor {
    return executor.NewDockerExecutor(
        containerImage, projectName, model, netrcFile,
        gitconfigFile, env, extraMounts, claudeDir, maxPromptDuration,
        currentDateTimeGetter,
        worktreeMode,
    )
}
```

Update every call site of `createDockerExecutor` in `factory.go` to pass `false` for `worktreeMode` for now (prompt 3 will change the relevant call site to pass `cfg.Workflow == config.WorkflowWorktree`).

### 5b. Update the direct call in `CreateSpecGenerator` (line ~454)

This function generates prompts FROM specs; it never needs `worktree` mode. Pass `false` as the new trailing `worktreeMode` argument so compilation succeeds:

```go
func CreateSpecGenerator(
    cfg config.Config,
    containerImage string,
    currentDateTimeGetter libtime.CurrentDateTimeGetter,
    slugMigrator slugmigrator.Migrator,
) generator.SpecGenerator {
    return generator.NewSpecGenerator(
        executor.NewDockerExecutor(
            containerImage,
            project.Name(cfg.ProjectName),
            cfg.Model,
            cfg.NetrcFile,
            cfg.GitconfigFile,
            cfg.Env,
            cfg.ExtraMounts,
            cfg.ResolvedClaudeDir(),
            cfg.ParsedMaxPromptDuration(),
            currentDateTimeGetter,
            false, // worktreeMode — spec generator never needs tmpfs
        ),
        // ... rest unchanged ...
    )
}
```

**Do not miss this call site** — skipping it will break compilation.

## 6. Unit tests for executor `buildDockerCommand` — `pkg/executor/executor_test.go`

Add a new `Describe("buildDockerCommand worktree flag", ...)` block that:

1. Builds a baseline `dockerExecutor` with `worktreeMode = false` and calls `buildDockerCommand` (or asserts via the full `Execute` path — use whichever approach the existing executor tests use to inspect args). Assert the output args do NOT contain `"--tmpfs"` anywhere.

2. Builds a `dockerExecutor` with `worktreeMode = true`. Assert the output args contain the string `"--tmpfs"` followed immediately by `"/workspace/.git"`.

3. Assert that the args list for `worktreeMode = false` and `worktreeMode = true` differ by exactly two elements (`"--tmpfs"` and `"/workspace/.git"`). This is the "exactly ONE new conditional" regression test from the spec acceptance criteria.

**Test package:** the existing `pkg/executor/executor_test.go` uses `package executor_test` and accesses internals via `pkg/executor/export_test.go`. Add a new exported helper to `export_test.go` — e.g., `BuildDockerCommandWithWorktreeModeForTest(...)` mirroring the existing `BuildDockerCommandForTest` but with the new `worktreeMode bool` parameter wired into the constructed `dockerExecutor` — and use it from the external test. Do NOT introduce a new `executor_internal_test.go`. Do NOT change the package of existing tests.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- **FREEZE `pkg/git/cloner.go`** — no changes permitted.
- **FREEZE all docker args in `buildDockerCommand` except the ONE new conditional** — every other mount and flag line stays byte-for-byte identical.
- The `Remove` method on `Worktreer` must return `nil` always (per spec: cleanup failure is non-fatal; caller logs and proceeds).
- Wrap all non-nil errors with `errors.Wrapf` / `errors.Errorf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never bare `return err`.
- Error messages: lowercase, no file paths in the message string.
- Do not touch `go.mod` / `go.sum` / `vendor/`.
- Existing tests must still pass.
- The `Worktreer` interface must have a `//counterfeiter:generate` annotation so the fake can be regenerated.
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional checks:
1. `grep -n "tmpfs" pkg/executor/executor.go` — exactly ONE occurrence, inside the `if e.hideGitDir` block.
2. `ls mocks/worktreer.go` — Counterfeiter fake exists.
3. The new executor test (requirement 6, case 3) asserts the args difference is exactly 2 elements.
4. `grep -cn "NewDockerExecutor" pkg/factory/factory.go` — returns `2` and both call sites updated to pass `worktreeMode` (both `false` after this prompt; prompt 3 changes the `createDockerExecutor` invocation).
</verification>
