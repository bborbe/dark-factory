---
status: completed
summary: Fixed runner_test.go by adding missing hideGit bool parameter to all 10 runner.NewRunner calls, and added //nolint:funlen to CreateOneShotRunner which grew past the 80-line limit after the hideGit guard was added.
container: dark-factory-312-fix-hidegit-direct-workflow
dark-factory-version: v0.121.1-dirty
created: "2026-04-17T10:10:38Z"
queued: "2026-04-17T10:10:38Z"
started: "2026-04-17T10:30:52Z"
completed: "2026-04-17T10:49:44Z"
---

<summary>
- Fix hideGit so it actually hides .git when the workspace is bind-mounted (workflow: direct)
- Anonymous Docker volumes cannot mask subdirectories of bind mounts — switch to tmpfs overlay
- Add hideGit to the YAML config parser so it is actually read from .dark-factory.yaml
- Skip host-side git checks (dirty files, index.lock) when hideGit is enabled
- Pass hideGit to the daemon runner so it skips the startup index.lock check
- Log hideGit in the effective config output for debugging
- Update tests for the new tmpfs mount strategy
</summary>

<objective>
Make `hideGit: true` work correctly with `workflow: direct`. Currently it is broken because:
(a) the anonymous volume approach cannot mask a subdirectory of a bind mount,
(b) `HideGit` was never added to `partialConfig` so it is never parsed from YAML,
(c) the daemon's startup and preflight checks still run git operations that block or timeout on large repos.
After this change, containers started with `hideGit: true` must have an empty `/workspace/.git`.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/executor/executor.go` — find `buildHideGitArgs` method.
Read `pkg/config/loader.go` — find `partialConfig` struct and `mergePartialContainer` function.
Read `pkg/factory/factory.go` — find `CreateRunner` and `CreateOneShotRunner` functions, look at where `DirtyFileChecker` and `GitLockChecker` are created.
Read `pkg/runner/runner.go` — find the `.git/index.lock` check in the `Run` method.
Read `pkg/processor/processor.go` — find `checkDirtyFileThreshold` method.
Read `pkg/executor/executor_test.go` — find the `buildDockerCommand hideGit` test suite.

Note: some of these changes may already be partially implemented. Check the current state of each file before making changes. Only implement what is missing.
</context>

<requirements>
1. **executor.go — buildHideGitArgs**: When `.git` is a directory, return `--tmpfs /workspace/.git:rw,size=1k` instead of the anonymous volume `-v /workspace/.git`. The anonymous volume approach does not work because Docker cannot mask a subdirectory of a bind mount with an anonymous volume. Keep the `/dev/null` bind for the file (worktree pointer) case.

2. **config/loader.go — partialConfig**: Add `HideGit *bool \`yaml:"hideGit"\`` to the `partialConfig` struct.

3. **config/loader.go — mergePartialContainer**: Add merge logic: `if partial.HideGit != nil { cfg.HideGit = *partial.HideGit }`.

4. **factory.go — CreateRunner**: When `cfg.HideGit` is true, set `dirtyFileChecker` and `gitLockChecker` to nil instead of creating real checkers. Both `NewDirtyFileChecker` and `NewGitLockChecker` run git commands that timeout on large repos.

5. **factory.go — CreateOneShotRunner**: Same as above — conditionally create `DirtyFileChecker` and `GitLockChecker` based on `cfg.HideGit`.

6. **factory.go — LogEffectiveConfig**: Add `"hideGit", cfg.HideGit` to the `slog.Info("effective config", ...)` call so it appears in daemon logs.

7. **runner.go — Run method**: Add `hideGit bool` parameter to `NewRunner`, store it in the `runner` struct, and guard the `.git/index.lock` startup check with `if !r.hideGit`.

8. **processor.go — checkDirtyFileThreshold**: Add nil guard for `dirtyFileChecker`: return early when `p.dirtyFileChecker == nil`.

9. **executor_test.go**: Update the test `"adds anonymous volume when hideGit is true and .git is a directory"` to expect `--tmpfs` and `/workspace/.git:rw,size=1k` instead of the anonymous volume. Update the `"never emits --tmpfs"` test to only assert no tmpfs when `hideGit=false` (rename to `"does not emit --tmpfs when hideGit is false"`).

10. **factory.go — CreateRunner call to runner.NewRunner**: Pass `cfg.HideGit` as the new last argument.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Do NOT add GOFLAGS or ROOTDIR env vars — hideGit only hides .git, nothing more
- Do NOT modify any files outside the dark-factory repo
- Keep the /dev/null bind approach for the file case (worktree pointer)
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
