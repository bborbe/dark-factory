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

### `worktree`

- dark-factory runs `git worktree add /tmp/dark-factory/<project>-<prompt> <promptBranch>` in the parent repo
- No object-store copy — worktree shares `.git/objects` with the parent (fast even for huge repos)
- The worktree's `.git` is a FILE pointing at `<parentRepo>/.git/worktrees/<name>`, which is NOT accessible inside the container
- Container mounts the worktree at `/workspace` with `--tmpfs /workspace/.git` — `.git` is hidden inside the container
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

## Delivery: orthogonal flags

These flags stack on top of any isolation mode.

| Flag | Default | Purpose | Requires |
|------|---------|---------|----------|
| `pr` | `false` | Push the branch and open a pull request at the end | `workflow: branch \| clone \| worktree` (direct has no feature branch) |
| `autoMerge` | `false` | Merge the PR automatically once checks pass | `pr: true` |
| `autoRelease` | `false` | Create git tag + bump CHANGELOG `## Unreleased` → `## vX.Y.Z` after merge to `<defaultBranch>` | any |

## Combinations

| workflow | pr | autoMerge | autoRelease | Effect |
|----------|----|-----------|-------------|--------|
| direct | false | - | false | commit to current branch, do nothing else |
| direct | false | - | true | commit to current branch, tag + release |
| branch | false | - | false | create branch, commit, stay local |
| branch | true | false | false | create branch, commit, open PR, await manual merge |
| branch | true | true | true | create branch, commit, open PR, merge, tag + release |
| worktree | false | - | false | worktree, commit, stay local — fast setup for huge repos, no push/PR |
| worktree | true | true | true | worktree, commit, push, PR, merge, tag + release — fast setup for huge repos |
| clone | false | - | false | clone, commit, push branch — no PR |
| clone | true | true | true | clone, commit, push, PR, merge, tag + release |

Invalid: `workflow: direct` with `pr: true` (no feature branch to open a PR for — validation rejects).

## Container semantics

| workflow | `/workspace` contents | `.git` inside container | Container can run git? |
|----------|-----------------------|--------------------------|------------------------|
| direct | parent repo | real `.git/` directory | yes |
| branch | parent repo (new branch checked out) | real `.git/` directory | yes |
| clone | fresh clone | real `.git/` directory (cloned) | yes |
| worktree | worktree files | `--tmpfs` overmount (empty) | NO — prompts must avoid git |

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

## Implementation notes

- `workflow: direct` + `workflow: branch` share the same parent repo at `/workspace` — container has a real `.git/`
- `workflow: clone` creates `/tmp/dark-factory/<project>-<prompt>/` on every prompt; cleaned after commit
- `workflow: worktree` creates a worktree under the same `/tmp/dark-factory/` path but via `git worktree add`; cleaned via `git worktree remove`
- All host-side git operations (fetch, branch create, commit, push, tag) use the parent repo for `direct`/`branch`, the clone for `clone`, the worktree path for `worktree`
