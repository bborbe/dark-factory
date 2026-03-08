---
status: executing
container: dark-factory-141-replace-worktree-with-clone
dark-factory-version: v0.30.3
created: "2026-03-08T22:59:02Z"
queued: "2026-03-08T22:59:02Z"
started: "2026-03-08T23:14:06Z"
---

<summary>
- PR workflow now uses a local git clone instead of git worktree for isolation
- Docker container gets a self-contained repo (real `.git` dir, no dangling references)
- Clone path moved from sibling directory to `/tmp/dark-factory/projectName-baseName`
- The isolation mechanism switches from git worktree to a full local clone
- No functional change to the PR workflow — still creates branch, commits, pushes, opens PR
</summary>

<objective>
Replace git worktree with local git clone in the PR workflow. Worktrees create a `.git` file that references the parent repo's `.git/worktrees/` path, which breaks when mounted inside Docker (the reference points outside the container). A local clone has a self-contained `.git` directory that works everywhere.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read these files before making changes:
- `pkg/git/worktree.go` — current Worktree interface (Add/Remove)
- `pkg/processor/processor.go` — `setupWorktreeForExecution` (~line 706), `setupWorktreeWorkflowState` (~line 517), `handleWorktreeWorkflow` (~line 643), `cleanupWorktreeOnError` (~line 408)
- `pkg/processor/processor_test.go` — tests using `mockWorktree`
</context>

<requirements>
1. Rename `pkg/git/worktree.go` to `pkg/git/cloner.go`. Replace the interface:

   ```go
   // OLD:
   type Worktree interface {
       Add(ctx context.Context, path string, branch string) error
       Remove(ctx context.Context, path string) error
   }

   // NEW:
   type Cloner interface {
       Clone(ctx context.Context, srcDir string, destDir string, branch string) error
       Remove(ctx context.Context, path string) error
   }
   ```

2. Implement `Clone` using local git clone with a new branch:
   ```go
   func (c *cloner) Clone(ctx context.Context, srcDir string, destDir string, branch string) error {
       cmd := exec.CommandContext(ctx, "git", "clone", "--branch", defaultBranch(ctx), srcDir, destDir)
       if err := cmd.Run(); err != nil {
           return errors.Wrap(ctx, err, "clone repo")
       }
       // Create and switch to feature branch
       checkoutCmd := exec.CommandContext(ctx, "git", "-C", destDir, "checkout", "-b", branch)
       if err := checkoutCmd.Run(); err != nil {
           return errors.Wrap(ctx, err, "create branch")
       }
       // Set push remote to origin (clone sets origin to local path, we need the real remote)
       // Get the original remote URL
       remoteCmd := exec.CommandContext(ctx, "git", "-C", srcDir, "remote", "get-url", "origin")
       remoteOutput, err := remoteCmd.Output()
       if err != nil {
           return errors.Wrap(ctx, err, "get remote url")
       }
       setRemoteCmd := exec.CommandContext(ctx, "git", "-C", destDir, "remote", "set-url", "origin", strings.TrimSpace(string(remoteOutput)))
       if err := setRemoteCmd.Run(); err != nil {
           return errors.Wrap(ctx, err, "set remote url")
       }
       return nil
   }
   ```

   Note: clone without `--branch` flag (clones default branch), then create the feature branch. Remove the `--branch defaultBranch(ctx)` from the clone command above — just use `git clone srcDir destDir`.

3. Implement `Remove` using `os.RemoveAll`:
   ```go
   func (c *cloner) Remove(ctx context.Context, path string) error {
       return os.RemoveAll(path)
   }
   ```

4. Update `pkg/processor/processor.go`:

   a. Change the `worktree` field type from `Worktree` to `Cloner` (and rename the field to `cloner`)

   b. In `setupWorktreeWorkflowState` (~line 528), change the clone path:
   ```go
   // OLD:
   state.worktreePath = filepath.Join("..", p.projectName+"-"+baseName)
   // NEW:
   state.worktreePath = filepath.Join(os.TempDir(), "dark-factory", p.projectName+"-"+baseName)
   ```

   c. In `setupWorktreeForExecution` (~line 706), replace `p.worktree.Add` with `p.cloner.Clone`:
   ```go
   // OLD:
   if err := p.worktree.Add(ctx, worktreePath, branchName); err != nil {
   // NEW:
   if err := p.cloner.Clone(ctx, originalDir, worktreePath, branchName); err != nil {
   ```

   d. In `handleWorktreeWorkflow` (~line 679) and `cleanupWorktreeOnError` (~line 416), replace `p.worktree.Remove` with `p.cloner.Remove`.

5. Update `pkg/factory/factory.go` — replace `git.NewWorktree()` with `git.NewCloner()` and update the parameter name.

6. Update `NewProcessor` signature — rename the `worktree` parameter to `cloner` with type `git.Cloner`.

7. Regenerate mocks: `make generate` (generates `mocks/cloner.go`, removes `mocks/worktree.go`)

8. Update `pkg/processor/processor_test.go`:
   - Replace all `mockWorktree` with `mockCloner`
   - Replace `mockWorktree.AddCallCount()` / `mockWorktree.AddArgsForCall(i)` with `mockCloner.CloneCallCount()` / `mockCloner.CloneArgsForCall(i)` (note: Clone takes `srcDir, destDir, branch` instead of `path, branch`)
   - Replace `mockWorktree.RemoveCallCount()` / `mockWorktree.RemoveArgsForCall(i)` with `mockCloner.RemoveCallCount()` / `mockCloner.RemoveArgsForCall(i)`

9. Delete old `mocks/worktree.go` if `make generate` doesn't clean it up automatically.

10. Delete `pkg/git/worktree_test.go` (tests for the old Worktree Add/Remove methods).
</requirements>

<constraints>
- `workflow: direct` must not change
- Branch naming convention `dark-factory/{baseName}` stays
- Clone path must be under `os.TempDir()/dark-factory/` (e.g. `/tmp/dark-factory/projectName-baseName`)
- The clone's `origin` remote must point to the real GitHub remote (not the local source dir)
- `autoMerge`, `autoRelease`, `autoReview` behavior unchanged
- Do NOT commit — dark-factory handles git
- Existing passing tests must still pass (updated for new interface)
</constraints>

<verification>
Run `make precommit` — must pass.

Verify no remaining worktree references:
```bash
grep -rn "Worktree\|worktree" pkg/ --include="*.go" | grep -v "_test.go" | grep -v "// "
# Should return zero results (all worktree references removed)
```
</verification>
