---
status: completed
tags:
    - dark-factory
    - spec
approved: "2026-04-16T18:59:28Z"
generating: "2026-04-16T18:59:28Z"
prompted: "2026-04-16T19:05:26Z"
verifying: "2026-04-16T19:40:56Z"
completed: "2026-04-17T12:34:33Z"
branch: dark-factory/hide-git
---

## Summary

- Mask the host's `.git` inside the container so the container never sees host-side git state
- Always on for `workflow: worktree` (fixes a real crash: worktree `.git` is a pointer file whose target isn't mounted, so git-in-container exits 128)
- Opt-in via `hideGit: true` for other workflows (`direct`, `branch`, `clone`), default off
- Mount shape picked by `os.Stat` on the project root: directory → anonymous volume; file → `/dev/null` bind; missing → no-op

## Problem

In `workflow: worktree` mode, the working tree's `.git` entry is a file pointing at `<parentRepo>/.git/worktrees/<name>`. The container mounts only the worktree, not the parent repo, so the pointer dangles. Any git invocation inside the container — including claude-yolo's own startup probes — fails with `fatal: not a git repository` and the container exits 128, aborting the prompt. A previous attempt in v0.116.1 used `--tmpfs /workspace/.git`, but tmpfs is always a directory mount and crashes when `.git` is a regular file; that mount was removed, leaving `worktree` unusable. Users running `worktree` today cannot complete prompts at all.

## Goal

After this work, git inside the container is cleanly masked for any workflow that cannot support it, and containers no longer crash on git probes. `worktree` runs prompts to completion. For other workflows, users can opt in to the same masking when they want the container isolated from host git state.

## Non-goals

- Making git actually work inside the container for worktree (a follow-up feature may reconstruct a usable `.git` via additional mounts)
- Introducing new workflow modes or changing the four existing ones
- Modifying claude-yolo or any container image
- Distinguishing read-only vs read-write masking (anonymous volume and `/dev/null` already give safe defaults)
- Masking any path other than `/workspace/.git`

## Assumptions

- Existing `direct`, `branch`, `clone` workflows have a real `.git` directory inside the container by design — masking must be opt-in only
- `worktree` cannot support in-container git today; the documented constraint "prompts must not rely on git inside the container" (see `docs/workflows.md`) continues to hold
- `os.Stat` on the project root's `.git` is cheap and safe at container-launch time
- Anonymous Docker volumes and `-v /dev/null:<path>` binds are stable, portable primitives on Linux and macOS Docker Desktop
- Naming the executor field `hideGit` (matching the config key) lets factory wiring and executor tests share one vocabulary, reducing translation churn across prompts

## Desired Behavior

1. Worktree auto-hides: when the resolved workflow is `worktree`, the container's `/workspace/.git` is always masked from the host. No configuration is required or honored to disable this — it is a correctness requirement.

2. Other workflows opt in: `direct`, `branch`, and `clone` honor a new top-level config flag `hideGit: bool` (default `false`). When `true`, the same masking is applied. When `false` or absent, the container sees the real `.git` as it does today.

3. Mount shape follows path shape: before launching the container, the system inspects the project root's `.git` entry:
   - Directory (normal repo, clone): mask it with an anonymous volume at `/workspace/.git`, hiding the host directory contents
   - File (worktree pointer, submodule pointer): mask it with a bind of `/dev/null` to `/workspace/.git`, hiding the pointer contents
   - Missing: skip masking entirely — there is nothing to hide

4. No impact when disabled: for `direct`/`branch`/`clone` with `hideGit: false` (the default), the docker command is byte-for-byte identical to today's command. No new mounts, no new flags.

5. Documented in workflow docs: `docs/workflows.md` explains that `worktree` auto-masks `.git`, and that `hideGit: true` is available as an opt-in for the other workflows. The existing "container can run git?" table remains accurate.

6. Config surface is minimal: one new YAML field (`hideGit`), one new boolean in the executor constructor. No new validation rules beyond the boolean itself.

## Constraints

- `docs/workflows.md` container-semantics table must still describe reality after this change
- The four workflow values (`direct`, `branch`, `worktree`, `clone`) remain unchanged — this spec does not rename or split them
- `workflow: direct` + `hideGit: true` is allowed and stays valid (it is the spiritual successor of the removed tmpfs behavior)
- Existing configs without `hideGit` must continue to parse and behave identically to today for `direct`/`branch`/`clone`
- Containers must not gain new capabilities, network access, or mounted paths beyond the single `/workspace/.git` mask
- `make precommit` passes; existing executor tests continue to pass unchanged
- The executor exposes a single `hideGit bool` constructor parameter; factory and tests must use that exact field name

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `.git` is a directory (direct/branch/clone with `hideGit: true`) | Anonymous volume masks directory; container sees empty `.git` | N/A |
| `.git` is a file (worktree) | `/dev/null` bind masks the pointer file; git-in-container fails cleanly as "not a repository" rather than "gitdir not found" | N/A |
| `.git` missing entirely (not a git repo at all) | No mount added; container sees no `.git` | N/A |
| `os.Stat` returns a permission error | Treat as missing (skip mount) and log a debug message; do not abort the container | User fixes permissions if they want masking |
| User sets `hideGit: true` with `workflow: worktree` | Same behavior as `hideGit: false` — worktree always hides regardless | N/A |
| User sets `hideGit: false` with `workflow: worktree` | Worktree still hides (auto-hide cannot be disabled); no warning needed | N/A |
| `.git` changes from directory to file between stat and container start (race) | Worst case: container sees a stale mount shape. Not a correctness issue — it is still masked | N/A |

## Security / Abuse Cases

- `hideGit: true` is strictly additive isolation — it reduces what the container can see, never expands it
- `/dev/null` bind target is a standard device node, not user-controllable
- Anonymous volumes are scoped to the container lifetime and cleaned up on `--rm` exit
- No new user-provided paths are interpolated into docker args, so no path-injection surface is added
- The mount mask does not leak credentials: the host's `.git/config` (which may contain tokens) becomes invisible to the container, which is a security improvement for the opt-in case

## Acceptance Criteria

- [ ] `HideGit` field parses from `.dark-factory.yaml` under key `hideGit`
- [ ] Missing `hideGit` defaults to `false`
- [ ] `workflow: worktree` produces a container with `/workspace/.git` masked regardless of `hideGit` value
- [ ] `workflow: direct|branch|clone` with `hideGit: false` produces the same docker command as before this change
- [ ] `workflow: direct|branch|clone` with `hideGit: true` produces a container with `/workspace/.git` masked
- [ ] Directory `.git` is masked with an anonymous volume (`-v /workspace/.git`)
- [ ] File `.git` is masked with `-v /dev/null:/workspace/.git`
- [ ] Missing `.git` skips the mount (no error, no warning above debug)
- [ ] `docs/workflows.md` documents the worktree auto-hide behavior and the `hideGit` opt-in
- [ ] Executor tests cover: worktree-dir, worktree-file, opt-in-enabled, opt-in-disabled-default, missing-dotgit
- [ ] `make precommit` passes

## Verification

```bash
make precommit
```

Manual verification:

1. In a `worktree` project, approve a prompt — container runs to completion, no `exit 128`
2. In a `direct` project with default config, run a prompt — `/workspace/.git` still visible inside the container (unchanged behavior)
3. In a `direct` project with `hideGit: true`, run a prompt — `/workspace/.git` is empty/masked
4. In a directory with no `.git` at all, run a prompt with `hideGit: true` — no mount added, container launches normally

## Do-Nothing Option

`workflow: worktree` remains unusable end-to-end: every prompt crashes at startup on the git probe. Users who want worktree's fast-setup benefit for huge repos cannot adopt it, and must pay the slow `clone` cost instead. For other workflows, the current "container sees host `.git`" behavior is acceptable — the do-nothing cost is concentrated entirely on the worktree workflow. Not acceptable.
