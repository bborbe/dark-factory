---
status: executing
spec: [084-fail-fast-on-worktree-without-hidegit]
container: dark-factory-exec-400-fail-fast-worktree-detection
dark-factory-version: v0.164.0
created: "2026-05-21T21:45:00Z"
queued: "2026-05-21T21:33:36Z"
started: "2026-05-21T21:40:11Z"
branch: dark-factory/fail-fast-on-worktree-without-hidegit
---

<summary>
- New `detectWorktreeOrSubmodule()` helper in `pkg/runner/` classifies CWD as worktree, submodule, or neither using only `os.Stat` (no git subprocess)
- `Runner.Run()` calls the helper immediately after lock acquisition; when `.git` is a file (worktree or submodule) and `hideGit=false`, exits non-zero before any container spawns
- Error message names the detected condition, names `hideGit=true` as remediation, and references `docs/troubleshooting.md` and the pre-created worktree runbook
- Unit tests cover six CWD shapes: worktree, submodule, regular repo, non-git dir, stat-error (unreachable `.git`), and symlink-to-directory
- Integration tests in `pkg/runner/runner_test.go` verify the gating behavior for worktree+hideGit=false (blocks), worktree+hideGit=true (passes), submodule+hideGit=false (blocks), and regular repo (passes)
</summary>

<objective>
Implement the fail-fast worktree/submodule detection gate in `pkg/runner/`. When dark-factory is started from a git worktree or submodule CWD (`.git` is a regular file, not a directory) without `hideGit=true`, it must refuse to start before any container is launched, with an actionable error message.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/runner/runner.go` — existing `.git/index.lock` precondition check at line 164 as the canonical pattern to follow.
Read `pkg/runner/runner_test.go` — existing `.git/index.lock` integration test at line 1125 as the canonical pattern for the startup-gating tests.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — error wrapping with `github.com/bborbe/errors`.
Read `go-validation-framework-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — precondition/startup check patterns.
Read `test-pyramid-triggers.md` in `~/.claude/plugins/marketplaces/coding/docs/` — which test types to write.
</context>

<requirements>
1. Create `pkg/runner/worktree.go` with an exported `ErrWorktreeOrSubmodule` sentinel error and a `detectWorktreeOrSubmodule(ctx context.Context) error` helper:
   - Use `os.Lstat(".git")` to get the FileInfo without following symlinks
   - If the error is `os.ErrNotExist` → return `nil` (not a worktree, not a submodule)
   - If the error is anything else → return `errors.Wrapf(ctx, err, "stat .git failed")`
   - If `Lstat` succeeds and the file mode `IsRegular()` returns `false` → return `nil` (`.git` is a directory, or a symlink whose target's type is evaluated by the OS at mount time — regular repo)
   - If `Lstat` succeeds and the file mode `IsRegular()` returns `true` → return the sentinel `ErrWorktreeOrSubmodule`
   - The sentinel error message must contain the words `worktree`, `hideGit`, `docs/troubleshooting.md`, and `PR via Pre-Created Worktree` runbook reference

2. In `pkg/runner/runner.go`, in the `Run()` method, add the worktree gate immediately AFTER the `slog.Info("acquired lock", ...)` call and BEFORE the existing `.git/index.lock` precondition block:
   ```go
   // Refuse to start from a worktree or submodule CWD unless hideGit is enabled.
   // The .git pointer in the CWD points to the parent repo's worktrees/ directory,
   // which is not mounted into the container.
   if !r.hideGit {
       if err := detectWorktreeOrSubmodule(ctx); err != nil {
           return errors.Wrap(ctx, err, "worktree/submodule CWD detected")
       }
   }
   ```
   Anchor by symbol (`slog.Info("acquired lock"...)` and the `.git/index.lock` check), not line numbers — line numbers drift.

3. Add the sentinel error to `pkg/runner/worktree.go`. Project convention for sentinel errors is the stdlib `errors` package aliased as `stderrors` (see `pkg/preflightconditions/conditions.go:31` for the canonical pattern: `var ErrPreflightFailed = stderrors.New(...)`):
   ```go
   import stderrors "errors"

   var ErrWorktreeOrSubmodule = stderrors.New(
       "worktree CWD detected: .git is a file (worktree or submodule); " +
           "dark-factory cannot run from a worktree unless hideGit=true. " +
           "To proceed: --set hideGit=true or add 'hideGit: true' to .dark-factory.yaml. " +
           "See docs/troubleshooting.md and the 'PR via Pre-Created Worktree' runbook for details.",
   )
   ```
   Naming: exported `ErrX` matches the project's `ErrPreflightFailed` convention. Do NOT use `github.com/bborbe/errors.New` for the sentinel — that takes a `context.Context` and is intended for wrapped errors at call sites, not package-level sentinels.

4. Add unit tests in `pkg/runner/runner_test.go` — table-driven tests for `detectWorktreeOrSubmodule`:
   - Case 1: "worktree" — create temp dir, write `"\n"` to `.git` (regular file), call helper, expect sentinel error
   - Case 2: "submodule" — create temp dir, write `"gitdir: ../.git/modules/foo"` to `.git`, call helper, expect sentinel error
   - Case 3: "regular repo" — create temp dir, `os.MkdirAll(".git", 0755)`, call helper, expect `nil`
   - Case 4: "non-git dir" — create temp dir with no `.git`, call helper, expect `nil`
   - Case 5: "stat-error" — make `.git` unreachable via `os.Stat`/`os.Lstat` (mode-0 on a file is NOT sufficient because `Lstat` reads the directory entry, not the file body — it still succeeds). Use ONE of: (a) create temp dir, `os.MkdirAll("parent", 0755)`, write `parent/.git` then `os.Chdir("parent")` then `os.Chmod("..", 0000)` (parent-dir EACCES makes `Lstat(".git")` fail) and restore mode 0755 in cleanup; OR (b) skip this case as architecturally unreachable on macOS/Linux when running as the file owner — instead add a unit test for the helper that injects a stub stat function returning an error. Pick (a) if the test can reliably restore permissions; pick (b) otherwise.
   - Case 6: "symlink to directory" — create temp dir, `os.MkdirAll(".real-git", 0755)`, `os.Symlink(".real-git", ".git")`, call helper, expect `nil` (symlink-to-directory must NOT trigger the gate; `Lstat` sees mode-symlink which is `IsRegular()==false`)
   - Use `os.Chdir` to switch CWD for each case; restore original CWD after each test via deferred `os.Chdir`. Do NOT mark the `Describe` block with `t.Parallel()` or run the suite with `-procs > 1` — `os.Chdir` is process-global and parallel execution races on the working directory
   - Run each case in its own `It` block within a `Describe("detectWorktreeOrSubmodule", ...)`

5. Add integration tests in `pkg/runner/runner_test.go` — startup gating tests:
   - Test: "refuses to start from worktree CWD when hideGit=false" — create a worktree-shaped temp dir (`.git` as regular file), create a Runner with `hideGit=false`, call `Run()` with a short timeout, expect non-nil error containing "worktree" and "hideGit"
   - Test: "starts successfully from worktree CWD when hideGit=true" — same setup but Runner with `hideGit=true`, expect `nil` error (or context cancel as the runner loops)
   - Test: "refuses to start from submodule CWD when hideGit=false" — submodule-shaped temp dir, same gating expectation
   - Test: "starts successfully from regular repo CWD regardless of hideGit" — `.git` as directory, both `hideGit` values pass
   - These tests go in a new `Describe("worktree gating", ...)` block

6. For the integration tests, the runner needs all its dependencies mocked. Use the existing `newTestRunner` helper from line ~58, which creates a runner with mocked locker, watcher, processor, etc. For the worktree CWD tests, call `os.Chdir(worktreeDir)` BEFORE creating the runner so the runner inherits the worktree CWD. Defer restore to original directory.
</requirements>

<constraints>
- Do NOT shell out to `git` in the detection helper — use only `os.Lstat`
- The check must run after lock acquisition but before any container is spawned (before watcher/processor/server start)
- The check applies to BOTH `daemon` and `run` commands (both call `Runner.Run()`)
- The error message must be actionable: names the condition, names the remediation, references docs
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrapf(ctx, err, "message")` for wrapping — never `fmt.Errorf`
- `os.Chdir` in tests must be matched with deferred restore to original directory
</constraints>

<verification>
```bash
make precommit
```
</verification>
