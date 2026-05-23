# Workflows

Dark-factory's workflow has two dimensions: **separation** (how much the prompt's work is isolated from the host's current state) and **delivery** (whether changes go straight to the base branch or through a PR, and whether they get tagged/released).

The dimensions are orthogonal — pick one separation mode and any combination of delivery flags.

## Separation: `workflow`

Progressive isolation levels.

| Value | Separation | Mental model |
|-------|------------|--------------|
| `direct` (default) | none | use dark-factory as part of your current git workflow |
| `branch` | branch only | work with branch separation |
| `worktree` | branch + worktree | work with branch + worktree separation |
| `clone` | branch + clone | work with branch + clone separation |

| Value | Branch created from | Working tree | Use when |
|-------|---------------------|--------------|----------|
| `direct` | no branch creation | current repo, current branch | solo maintainer, trusted prompts, no isolation needed |
| `branch` | `origin/<defaultBranch>` | current repo, new branch checked out in-place | isolation without a second checkout |
| `worktree` | `origin/<defaultBranch>` | `git worktree add /tmp/dark-factory/<name>` | huge repos — fast setup, shares `.git/objects` with parent; container does not need git inside |
| `clone` | `origin/<defaultBranch>` | `git clone srcDir /tmp/dark-factory/<name>` | full container isolation, or when the container must run git against a standalone repo |

### `direct`

- No branch, no clone, no worktree
- Container mounts the parent repo at `/workspace`
- Changes commit onto the current branch
- Serial execution (one container per project at a time)

### `branch`

- dark-factory runs `git fetch origin`, then `git checkout -b <promptBranch> origin/<defaultBranch>` in the parent repo
- Container mounts the parent repo at `/workspace`, now on the new branch
- After the prompt: commit, optionally push / open PR
- Serial execution (parent repo is shared)
- **Working-tree cleanliness check:** before switching branches, dark-factory verifies the tree is clean — but ignores uncommitted changes inside the four prompt directories (`inboxDir`, `inProgressDir`, `completedDir`, `logDir`) because those are dark-factory's own bookkeeping writes, not user work. Any uncommitted change outside those directories still aborts Setup with an error naming the specific dirty file. After the check passes, dark-factory discards the bookkeeping dirt (via `git checkout HEAD -- <prefix>` for each configured prefix) so that `git checkout <featureBranch>` does not refuse when the feature branch has divergent content for those same files.

### `worktree`

- dark-factory runs `git worktree add /tmp/dark-factory/<project>-<prompt> <promptBranch>` in the parent repo
- No object-store copy — worktree shares `.git/objects` with the parent (fast even for huge repos)
- The worktree's `.git` is a FILE pointing at `<parentRepo>/.git/worktrees/<name>`, which is NOT accessible inside the container
- Container mounts the worktree at `/workspace` — the worktree's `.git` pointer file is present but its target (`<parentRepo>/.git/worktrees/<name>`) is not mounted, so git commands inside the container fail naturally
- **`.git` is always masked** inside the container: the worktree's `.git` pointer file is covered by a Docker volume overlay, preventing the dangling-pointer error (`fatal: not a git repository`) that previously crashed container startup
- **Git does not work inside the container.** Prompts must not rely on `git status`, `git diff`, or `git commit` from inside
- Parallel execution: each prompt gets its own worktree
- Host-side commits: dark-factory runs `git add` / `commit` / `push` against the worktree path on the host, where the `.git` pointer resolves correctly
- Cleanup: `git worktree remove /tmp/dark-factory/<project>-<prompt>` (prunes the parent's `.git/worktrees/<name>` entry)

### `clone`

- dark-factory runs `git clone <parentRepo> /tmp/dark-factory/<project>-<prompt>`
- Sets the clone's `origin` to the parent's `origin` URL (not the local path)
- Creates `<promptBranch>` from `origin/<defaultBranch>`
- Container mounts the clone at `/workspace`
- Parallel execution: each prompt gets its own clone
- Clone contains a full `.git` directory — git works inside the container
- Cleanup: `rm -rf` the clone after commit/push
- **Push-before-remove:** the clone executor pushes the feature branch to origin from inside the clone (before `os.Chdir` back to the original repo and before `Cloner.Remove`). This ensures the branch is reachable on origin when `handleAfterIsolatedCommit` runs in the parent repo after the clone is gone.

## Delivery: orthogonal flags

These flags stack on top of any isolation mode.

| Flag | Default | Purpose | Requires |
|------|---------|---------|----------|
| `pr` | `false` | Push the branch and open a pull request at the end | `workflow: branch \| clone \| worktree` (direct has no feature branch) |
| `autoMerge` | `false` | Merge the PR automatically once checks pass | `pr: true` |
| `autoRelease` | `false` | Push commits to remote after each completion. Additionally bump `CHANGELOG.md` `## Unreleased` → `## vX.Y.Z` and create+push a tag, but **only when `CHANGELOG.md` exists**. Without `CHANGELOG.md`, only push happens (no version, no tag). | any |

## Combinations

| workflow | pr | autoMerge | autoRelease | Effect |
|----------|----|-----------|-------------|--------|
| direct | false | - | false | commit to current branch, stay local |
| direct | false | - | true | commit to current branch, push; tag if CHANGELOG.md present |
| branch | false | - | false | create branch, commit, stay local |
| branch | true | false | false | create branch, commit, open PR, await manual merge |
| branch | true | true | true | create branch, commit, open PR, merge, push; tag if CHANGELOG.md present |
| worktree | false | - | false | worktree, commit, stay local — fast setup for huge repos, no push/PR |
| worktree | true | true | true | worktree, commit, push, PR, merge; tag if CHANGELOG.md present — fast setup for huge repos |
| clone | false | - | false | clone, commit, push branch — no PR |
| clone | true | true | true | clone, commit, push, PR, merge; tag if CHANGELOG.md present |

**Tagging is gated on `CHANGELOG.md` presence**, independent of `autoRelease`. With `autoRelease: true` and no `CHANGELOG.md`, commits are still pushed but no version bump or tag is produced.

**For protected-master projects, do NOT set `autoRelease: true`.** Branch protection will reject the tag push (and any direct push to master) immediately after the PR is merged. Instead, ship code changes via a PR workflow (`workflow: branch | clone | worktree` + `pr: true`, optionally with `autoMerge: true`) and delegate release/tagging to a separate pipeline — e.g. the **GitHub Release Agent**, which watches merged commits on master and creates releases asynchronously with its own elevated permissions. The split also matches branch-protection reality at most orgs: PR-merge permissions are usually less restrictive than tag-push permissions, so they naturally belong in different pipelines.

**Protected master is not 100% supported today** — the PR produced by each prompt run contains the prompt's work commit only (code change + that single prompt's `in-progress → completed` rename). Other spec/prompt admin work (new prompt files written by spec generation, status flips like `idea → approved`, spec lifecycle transitions like `prompted → verifying → completed`) accumulates as uncommitted changes in the original repo's working tree. On unprotected master those changes get committed and pushed directly; on protected master they need their own PR. There is no built-in mechanism today to bundle spec/prompt admin into the prompt's PR. Tracked in `specs/ideas/protected-master-admin-bundle.md` — describes the problem only, no chosen solution.

Invalid:
- `workflow: direct` with `pr: true` — no feature branch exists to open a PR from; validation rejects.
- `pr: true` + `autoMerge: false` + `autoRelease: true` for any non-direct workflow — `autoRelease` requires tagging the merged commit on master, but `autoMerge: false` means the branch is never merged automatically; validation rejects with three actionable resolutions: set `autoMerge: true`, or set `autoRelease: false`, or set `pr: false`.

## Container semantics

| workflow | `/workspace` contents | `.git` inside container | Container can run git? |
|----------|-----------------------|--------------------------|------------------------|
| direct | parent repo | real `.git/` directory | yes |
| branch | parent repo (new branch checked out) | real `.git/` directory | yes |
| clone | fresh clone | real `.git/` directory (cloned) | yes |
| worktree | worktree files | `.git` masked (anonymous volume or `/dev/null` bind — see hideGit) | NO — prompts must avoid git |

## Choosing a mode

- **Small/medium repo, solo work** → `direct` + `autoRelease`
- **Small/medium repo, team with PR review** → `branch` + `pr` + `autoMerge`
- **Parallel prompts on the same repo** → `clone` or `worktree`
- **Huge repo** (sm-octopus/billomat scale, > 1 GB) → `worktree` — avoids the slow clone cost. Accept the no-git-in-container constraint.
- **Huge repo where container MUST run git** → `clone`. Slower setup, but the container sees a working repo.

## Migration from legacy config

Only `worktree: bool` is legacy — `pr: bool` stays as an orthogonal delivery flag. Mapping of the legacy `worktree: bool` (combined with `pr: bool`) to the new `workflow`:

| `worktree` | `pr` | New workflow | Resolved `pr` | Notes |
|------------|------|--------------|---------------|-------|
| false | false | `direct` | `false` | unchanged |
| false | true  | `branch` | `true` | behavior improves: PR is now actually created (was silent no-PR bug) |
| true  | false | `clone`  | `true` | compatibility override: today's clone mode hard-codes PR creation; mapping preserves that. `slog.Warn` naming both legacy fields (`worktree`, `pr`) tells the user to set `pr: true` explicitly to silence the warning |
| true  | true  | `clone`  | `true` | unchanged |

The previous 2-value `workflow` enum also maps forward:

| Legacy enum | New |
|-------------|-----|
| `workflow: direct` | `workflow: direct` (unchanged) |
| `workflow: pr` | `workflow: clone`, `pr: true` (matches the legacy `pr: true, worktree: true` boolean pair) |

When both `workflow` and the legacy `worktree: bool` are set, `workflow` wins for isolation and dark-factory logs a deprecation warning naming `worktree`. `pr: bool` alongside `workflow` is NOT a conflict — it coexists with any workflow value. To fully migrate, set `workflow:` and remove `worktree:`; keep `pr:` if you want a PR.

## Example configs

### Huge monorepo, PR workflow

```yaml
projectName: billomat
workflow: worktree
pr: true
autoMerge: true
autoRelease: false
defaultBranch: master
```

### Small library, direct release

```yaml
projectName: my-lib
workflow: direct
autoRelease: true
defaultBranch: master
```

### Team with review, medium repo

```yaml
projectName: api-service
workflow: branch
pr: true
autoMerge: false  # manual merge
defaultBranch: main
```

## Changing workflow flags mid-execution

Workflow is bound to a prompt when its `Setup` first runs — persisted via `branch:` frontmatter (or absence of it) plus clone/worktree directory presence. Restarting the daemon with different `--set workflow=...` flags does **not** retroactively re-route in-flight prompts; the daemon re-attaches to the running container and lets the prompt finish under its original workflow.

This can silently produce direct-to-master commits when the operator expected a PR. To change workflow flags safely:

1. Drain in-flight prompts (wait for completion), **or**
2. Cancel + requeue affected prompts under the new workflow.

## Lifecycle

All workflow modes use the lifecycle **move → stage → commit → push**. The prompt file is renamed from `prompts/in-progress/<id>.md` to `prompts/completed/<id>.md` before the work commit is staged, so a single commit (and a single push) carries both the code change and the rename. If the work commit fails, the rename is rolled back; the on-disk state always matches what HEAD reflects.

### Post-push mirror for clone and worktree

After a **clone** or **worktree** workflow completes, the prompt file rename (`in-progress/` → `completed/`) exists only inside the isolated clone/worktree and was committed and pushed from there. The original repo on the host still shows the prompt at `in-progress/<id>.md`. To keep the daemon's local view in sync with `origin/master`, dark-factory performs a **post-push mirror**: after the push succeeds and the isolated working tree is destroyed, the daemon calls `MoveToCompleted` against the original repo's `in-progress/` path from the original repo's CWD. This is a filesystem-only operation (no git commit, no remote fetch/push). If the file is already at `completed/<id>.md` in the original repo (e.g., operator pulled in the meantime), the mirror is a no-op. If the rename fails (e.g., original repo is on a different branch), dark-factory logs `clone-sync-mismatch` at WARN level and returns success — the remote is already correct, and the operator can recover with `git pull`.

## Implementation notes

- `workflow: direct` + `workflow: branch` share the same parent repo at `/workspace` — container has a real `.git/`
- `workflow: clone` creates `/tmp/dark-factory/<project>-<prompt>/` on every prompt; cleaned after commit
- `workflow: worktree` creates a worktree under the same `/tmp/dark-factory/` path but via `git worktree add`; cleaned via `git worktree remove`
- All host-side git operations (fetch, branch create, commit, push, tag) use the parent repo for `direct`/`branch`, the clone for `clone`, the worktree path for `worktree`

## `.git` masking (`hideGit`)

By default, `workflow: worktree` always masks the worktree's `.git` pointer file inside the container. The mask prevents `git` from following a dangling gitdir pointer, so the container no longer crashes with `fatal: not a git repository`. For other workflows (`direct`, `branch`, `clone`), the container sees the host's real `.git` directory by default.

To opt in to the same masking for non-worktree workflows, set `hideGit: true` in `.dark-factory.yaml`:

```yaml
workflow: direct
hideGit: true
```

**Mount shape** — the mask is chosen by inspecting the project root's `.git` entry at container launch time:

| `.git` on host | Docker flag added |
|----------------|-------------------|
| Directory (normal repo, clone) | `-v /workspace/.git` — anonymous volume hides host directory contents |
| File (worktree pointer, submodule) | `-v /dev/null:/workspace/.git` — bind hides the pointer |
| Missing | no flag — nothing to hide |

`hideGit: true` is strictly additive isolation — it reduces what the container can see, never expands it. The host's `.git/config` (which may contain tokens) becomes invisible to the container.

**Go projects** — without `.git`, Go 1.18+ prints `error obtaining VCS status` during builds. The build still succeeds, but the warning is noisy. Suppress it by adding to `.dark-factory.yaml`:

```yaml
env:
  GOFLAGS: "-buildvcs=false"
```
