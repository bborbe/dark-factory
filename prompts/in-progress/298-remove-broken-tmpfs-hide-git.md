---
status: approved
created: "2026-04-16T19:35:00Z"
queued: "2026-04-16T17:39:12Z"
---

<summary>
- The worktree workflow no longer fails with a Docker "not a directory" mount error on container start
- The broken `--tmpfs /workspace/.git` overlay is removed because it cannot mount over a worktree's `.git` pointer file
- Worktree mode works end-to-end — the daemon can add a worktree, run the container, and clean up without runtime errors
- No new options, flags, or config fields are required for projects using worktree mode
- The prompt-executing container still has no functional git access under worktree mode because the in-container `.git` gitdir pointer points to a host path that is not mounted
- Existing clone, branch, and direct workflows are unaffected — none of them ever needed the tmpfs overlay
- Existing tests covering `NewDockerExecutor` and Docker command construction pass after signature changes
</summary>

<objective>
Remove the broken `--tmpfs /workspace/.git` overlay and the associated `worktreeMode`/`hideGitDir` plumbing from the executor, so that `workflow: worktree` no longer fails on container start with a mount error.
</objective>

<context>
Read /workspace/CLAUDE.md for project conventions.

Background: spec 046 added a `worktreeMode bool` parameter to `NewDockerExecutor` that enabled a `--tmpfs /workspace/.git` Docker flag. The intent was to hide `.git` from worktree-mode containers. The implementation is incompatible with git worktrees: a git worktree's `.git` is a **file** (containing a `gitdir:` pointer), not a directory, so Docker rejects the tmpfs overlay with:

```
error mounting "tmpfs" to rootfs at "/workspace/.git": ... not a directory: Are you trying to mount a directory onto a file (or vice-versa)?
```

The feature is safe to remove because:
1. It is only wired from `worktreeMode=true` in production (the `CreateSpecGenerator` caller passes `false`).
2. In worktree mode the worktree's `.git` pointer file points to a host path (`<host>/.git/worktrees/<name>`) that is not mounted into the container — so git commands inside the container already fail naturally without the tmpfs overlay.
3. No completed prompt, spec, or test depends on `--tmpfs /workspace/.git` being present for correctness.

Files to read before making changes:
- pkg/executor/executor.go — has `NewDockerExecutor`, the `dockerExecutor` struct, and `buildDockerCommand`
- pkg/executor/export_test.go — has `BuildDockerCommandWithWorktreeModeForTest` and the legacy `BuildDockerCommandForTest`
- pkg/executor/executor_test.go — contains the two `--tmpfs` test cases (search for `--tmpfs`)
- pkg/factory/factory.go — two call sites of `NewDockerExecutor`: `CreateSpecGenerator` (passes `false`) and `createDockerExecutor` (passes `workflow == config.WorkflowWorktree`)
</context>

<requirements>
1. In `pkg/executor/executor.go`:
   - Remove the `worktreeMode bool` parameter from `NewDockerExecutor` (the last parameter). The remaining parameters and order stay the same.
   - Remove the `hideGitDir bool` field from the `dockerExecutor` struct.
   - Remove the `hideGitDir: worktreeMode,` initializer from the `NewDockerExecutor` body.
   - In `buildDockerCommand`, remove the `if e.hideGitDir { args = append(args, "--tmpfs", "/workspace/.git") }` block entirely.

2. In `pkg/executor/export_test.go`:
   - Delete the `BuildDockerCommandWithWorktreeModeForTest` helper entirely (it only exists to test the removed behavior).
   - `BuildDockerCommandForTest` stays as-is (it already does not use `hideGitDir`).

3. In `pkg/executor/executor_test.go`:
   - Remove the entire `Describe("worktree flag", ...)` block at lines ~976-1022. This block contains three `It` cases and a `buildWorktreeCmd` closure that all depend on `BuildDockerCommandWithWorktreeModeForTest`:
     - `It("does not include --tmpfs when worktreeMode is false", ...)` (~line 997)
     - `It("includes --tmpfs /workspace/.git when worktreeMode is true", ...)` (~line 1004)
     - `It("args differ by exactly two elements between worktreeMode false and true", ...)` (~line 1017)
   - After deletion, confirm no other references remain: `grep -n BuildDockerCommandWithWorktreeModeForTest pkg/executor/*.go` must return zero matches (the helper itself is deleted per requirement 2).
   - Add a single new `It` block in the nearest enclosing `Describe` that already contains `buildDockerCommand` tests — specifically the same `Describe` that the deleted `Describe("worktree flag", ...)` was nested inside. Title it `"never emits --tmpfs /workspace/.git"`. Copy the argument pattern from an existing `executor.BuildDockerCommandForTest(...)` call in the same file (search for `BuildDockerCommandForTest(` and reuse the plausible-arguments pattern). The assertion: iterate over `cmd.Args`, assert no arg equals `"--tmpfs"`. This locks in the removal.

4. In `pkg/factory/factory.go`:
   - In `CreateSpecGenerator` (around line 454): remove the trailing `false, // worktreeMode — spec generator never needs tmpfs` argument from the `executor.NewDockerExecutor(...)` call.
   - In `createDockerExecutor` (around line 526): remove the `worktreeMode bool` parameter from the function signature. Update the body so `executor.NewDockerExecutor(...)` is called without the trailing `worktreeMode` argument.
   - Before editing, run `grep -n 'createDockerExecutor(' pkg/factory/factory.go` to enumerate ALL call sites. Update every call to drop the trailing `workflow == config.WorkflowWorktree` (or equivalent) argument. The known call site is around line 605.

5. Update `docs/workflows.md` to remove references to the deleted `--tmpfs /workspace/.git` behavior:
   - Around line 44, in the `### worktree` section, replace the bullet `"Container mounts the worktree at /workspace with --tmpfs /workspace/.git — .git is hidden inside the container"` with: `"Container mounts the worktree at /workspace — the worktree's .git pointer file is present but its target (the parent repo's .git/worktrees/<name>) is not mounted, so git commands inside the container fail naturally"`.
   - Around line 93, in the "Container semantics" table, replace the `worktree` row's `.git` column from `` `--tmpfs` overmount (empty) `` to: `` `.git` pointer file (target not mounted) ``. Leave the "Container can run git?" column as `NO — prompts must avoid git` (that remains true).
   - The nearby bullet on line 45 (`"Git does not work inside the container. Prompts must not rely on git status..."`) remains accurate and must stay unchanged.

6. Do NOT add any replacement mechanism for hiding `.git` in worktree mode. The natural broken-gitdir-pointer behavior is sufficient for this spec; a proper hide mechanism is a separate future concern.

7. Run `make precommit` in the repo root — must pass cleanly with exit code 0.
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT touch files outside pkg/executor/, pkg/factory/, and docs/workflows.md (except transitively via `make precommit` regeneration of mocks if the build requires it)
- Do NOT add new config fields or env vars
- Existing tests (other than the two deleted `--tmpfs` cases) must still pass
- Follow github.com/bborbe/errors for any new error wrapping (none expected for this change)
- Do NOT introduce `fmt.Errorf`
</constraints>

<verification>
Run `make precommit` in the repo root — must pass with exit code 0.

Additionally confirm the removal with:
- `grep -nw hideGitDir pkg/executor/*.go pkg/factory/*.go` — must return zero matches
- `grep -n -- "--tmpfs" pkg/executor/*.go pkg/factory/*.go` — must return zero matches except inside the new negative-assertion test string literal from requirement 3
- `grep -n BuildDockerCommandWithWorktreeModeForTest pkg/executor/*.go` — must return zero matches
- `grep -n "worktreeMode" pkg/executor/*.go pkg/factory/*.go` — should return zero matches (note: `config.WorkflowWorktree` is a different identifier; `-w` ensures word-boundary match)
</verification>
