---
status: prompted
approved: "2026-04-16T11:47:58Z"
generating: "2026-04-16T11:47:58Z"
prompted: "2026-04-16T12:00:37Z"
branch: dark-factory/workflow-enum-with-worktree-mode
---

## Summary

- Widen the existing `workflow` enum in `.dark-factory.yaml` from two values (`direct`, `pr`) to four (`direct`, `branch`, `worktree`, `clone`) so it expresses isolation without ambiguity, and make it the canonical field for isolation — replacing the legacy `worktree: bool`.
- `pr: bool` stays as-is — it is NOT legacy. It becomes a truly orthogonal delivery flag that opens a pull request at the end, stacking on ANY `workflow` value (today it only has effect in clone mode, which is a bug this spec fixes).
- Add a NEW `workflow: worktree` value that uses real `git worktree add` (instead of the current full `git clone`) so huge repos like billomat can be set up in seconds instead of tens of minutes.
- In `worktree` mode, the container does not get access to `.git` — dark-factory handles all git operations on the host, and prompts must not rely on git inside the container.
- Wire PR creation into all non-direct workflows (today it is hard-coded into clone mode; move it to a `pr`-gated step that runs after any isolation mode that produced a feature branch).
- Wire `workflow: branch` so it creates an in-place feature branch, commits, and optionally pushes + opens a PR (today this combination silently does nothing because the `pr: true, branch: ""` path has no PR code).
- The legacy `worktree: bool` and the legacy 2-value `workflow: pr` enum value keep working via deprecation mapping so no existing config breaks.
- Reference doc `docs/workflows.md` describes the full workflow model (separation + delivery dimensions) and is the authoritative design document.

## Problem

Dark-factory expresses isolation today via two booleans — `pr` and `worktree` — plus a thin legacy `workflow` enum with only two values (`direct`, `pr`) that maps to the bools. The `pr` boolean was intended as two concerns glued together: (1) whether to create a feature branch (isolation), and (2) whether to open a pull request at the end (delivery). The current implementation is inconsistent with that intent: PR creation is hard-coded inside `handleCloneWorkflow` (`pkg/processor/processor.go:1498`) and runs unconditionally when `worktree: true`, while `pr: true, worktree: false` only triggers in-place branch switching — it NEVER creates a PR. That mismatch, plus `worktree: true` doing a clone rather than a worktree, produces four problems:

1. **Huge-repo setup time.** `worktree: true` actually runs `git clone <parentRepo> /tmp/…`, which on repos like `sm-octopus/billomat` (multi-GB vendor directories) takes tens of minutes per prompt. Real `git worktree add` would be seconds.
2. **Misleading name.** The `worktree` boolean does a clone, not a worktree.
3. **Combinatorial gaps.** Boolean pairs cannot distinguish "real git worktree" from "full clone" — both are `worktree=true` in today's model.
4. **Broken orthogonality.** `pr: bool` only has effect in clone mode today. `pr: true, worktree: false, branch: ""` silently behaves like `pr: false`. `pr: true, worktree: false, branch: "foo"` switches to the branch, commits, but never pushes and never opens a PR. `pr: false, worktree: true` still opens a PR. None of these match the documented orthogonality promise.

The fix separates the two concerns cleanly: a `workflow` enum owns isolation (`direct`/`branch`/`worktree`/`clone`), `pr: bool` owns delivery (open a PR when the working branch differs from default), and PR creation is lifted out of `handleCloneWorkflow` into a shared post-execution step that runs whenever `pr: true` AND a feature branch exists. The `worktree: bool` is deprecated. The legacy enum value `pr` maps forward to `clone + pr: true` (its current boolean-equivalent behavior).

## Goal

After this work, projects can opt into a fast real-worktree isolation mode by setting `workflow: worktree` in `.dark-factory.yaml`. For huge repos this cuts per-prompt setup from tens of minutes to seconds. The `pr: bool` flag works identically in every workflow mode — set it to create a PR, leave it unset to stay local. Projects that currently use `worktree: true` continue to work unchanged (mapped to `workflow: clone, pr: true`; see Compatibility). The `docs/workflows.md` document serves as the canonical reference for which mode to pick.

## Assumptions

- `git worktree add` is available on any host running dark-factory (git ≥ 2.5, universally true on supported platforms).
- Docker supports `--tmpfs <path>` to overmount a tree inside a container, which is how `.git` is hidden inside the container in `worktree` mode. This is native to Docker 1.10+ on all platforms.
- Projects opting into `workflow: worktree` are responsible for verifying that their own `make precommit` and prompt-level commands do NOT require git inside the container. Known-safe: Go projects whose precommit is `gofmt/vet/golangci-lint/go test` with no pre-commit hooks shelling to `git`. Known-unsafe: projects whose precommit runs `git status`, `git diff`, repository-level lint-on-changed-files, or signs commits inside the container. Unsafe projects should stay on `workflow: clone`. (The container agent's own `git status` tool calls will fail too, but that is tolerated — the agent continues without the information.)
- The parent repo's `.git/` directory is stable and shared between concurrent worktrees on the same host. `git worktree` is designed for exactly this.
- Existing users of `worktree: true, pr: false` are rare/nonexistent in practice — today that combination already creates a PR (hard-coded), so no one has come to rely on "clone without PR" as a stable behavior. The compatibility mapping preserves today's de-facto behavior by mapping `worktree: true, pr: false` → `workflow: clone, pr: true`. Any project that genuinely wants clone-without-PR opts in explicitly via `workflow: clone, pr: false` — which is now a real supported combination (was not expressible before).

## Non-goals

- Removing or renaming `pr: bool` — it stays as an orthogonal delivery flag.
- Removing the legacy `worktree: bool` field — it is deprecated but keeps working via mapping to the new enum, with a warning when set.
- Changing `autoMerge` or `autoRelease` semantics beyond what orthogonality requires — they remain delivery flags that stack on PR-producing workflows.
- Per-prompt workflow override in prompt frontmatter — project-level config only.
- Solving Docker bind-mount performance on macOS for huge repos — orthogonal problem, out of scope.
- Rewriting the existing `handleCloneWorkflow` clone mechanics in `pkg/git/cloner.go` — only the PR-creation step inside it moves.

## Desired Behavior

1. Setting `workflow: direct` in `.dark-factory.yaml` means no branch, no clone, no worktree. The container mounts the parent repo at `/workspace` and commits to the current branch. `pr: true` combined with `workflow: direct` is rejected at config-load time (no feature branch exists to open a PR from).

2. Setting `workflow: branch` means dark-factory creates a feature branch from `origin/<defaultBranch>` in the parent repo and checks it out in place before running the container. The container mounts the parent repo. Branch name is taken from the prompt's `branch:` frontmatter field if set, otherwise auto-generated as `dark-factory/<promptBaseName>` (same convention as clone mode uses today). On completion: commit. If `pr: true`, push and open a PR. If `pr: false`, stay on the feature branch locally (no push, no PR).

3. Setting `workflow: worktree` means dark-factory runs `git worktree add /tmp/dark-factory/<project>-<prompt> <promptBranch>` in the parent repo, mounts that worktree at `/workspace`, and adds `--tmpfs /workspace/.git` to the Docker command so `.git` is invisible inside the container. Branch name resolution is identical to `workflow: branch`. On completion: commit on the host against the worktree path. If `pr: true`, push and open a PR. Always clean up via `git worktree remove`.

4. Setting `workflow: clone` means dark-factory runs `git clone <parentRepo> /tmp/dark-factory/<project>-<prompt>`, sets the real origin URL, creates `<promptBranch>` (same naming as above), mounts the clone at `/workspace`. Container sees a full working `.git/`. On completion: commit on the host. If `pr: true`, push and open a PR. Always clean up via `rm -rf`.

5. **PR creation is orthogonal to workflow.** PR creation happens exactly once, after code commit, in every non-`direct` workflow IFF `pr: true`. Today PR creation is hard-coded into `handleCloneWorkflow` at `pkg/processor/processor.go:1498` and runs unconditionally; this spec moves it into a shared helper that all three non-direct workflows invoke, gated on `p.pr`. The existing `findOrCreatePR` function is reused as-is.

6. When `.dark-factory.yaml` contains the legacy `worktree: bool` field (with or without `pr: bool`) but no `workflow` field, dark-factory derives the workflow at load time and logs an `slog.Info` deprecation notice for `worktree`. `pr: bool` is NOT legacy and is preserved as-is, EXCEPT for one case where preservation would silently change today's behavior (see row 4 below). Mapping:
   - Row 1: `worktree: false, pr: false` → `workflow: direct, pr: false`
   - Row 2: `worktree: false, pr: true` → `workflow: branch, pr: true` (behavior improves: PR is now actually created, which was the intent)
   - Row 3: `worktree: true, pr: true` → `workflow: clone, pr: true`
   - Row 4: `worktree: true, pr: false` → `workflow: clone, pr: true` (compatibility override: today's code hard-codes PR creation in clone mode, so this mapping preserves today's de-facto behavior. An `slog.Warn` notes the override and tells the user to set `pr: true` explicitly to silence it.)

7. When `.dark-factory.yaml` uses the legacy 2-value enum value `workflow: pr`, dark-factory maps it forward and logs an `slog.Info` deprecation notice:
   - `workflow: pr` → `workflow: clone, pr: true` (matches today's `pr: true, worktree: true` boolean pair)
   - `workflow: direct` → unchanged (same meaning in old and new enum)

8. When `.dark-factory.yaml` contains BOTH `workflow` AND the legacy `worktree: bool`, `workflow` wins for isolation and dark-factory logs an `slog.Warn` naming `worktree`. `pr: bool` alongside `workflow` is NOT a conflict.

9. `dark-factory config` output shows the resolved `workflow` value (after any legacy mapping) and `pr: bool` separately so operators can see both dimensions.

10. The dead-code branch at `pkg/processor/processor.go:1152` (`featureBranch != "" && !p.pr`) is wired to handle the new `workflow: branch, pr: false` case: after commit on the feature branch, call `handleBranchCompletion` which (a) checks if this is the last queued prompt on the branch, (b) if yes, merges the feature branch back to default and triggers a release if applicable, (c) if no, leaves the branch local for later prompts. This matches the function's existing intent (see code comments at `pkg/processor/processor.go:1617`).

## Compatibility Matrix

Legacy config → new resolved config:

| Legacy input | Resolved workflow | Resolved pr | Log level | Behavior change |
|--------------|-------------------|-------------|-----------|-----------------|
| `worktree: false, pr: false` | `direct` | `false` | Info | None |
| `worktree: false, pr: true` | `branch` | `true` | Info | PR is now actually created (was broken: silent no-PR) |
| `worktree: true, pr: false` | `clone` | `true` | Warn (override) | None (today already creates PR hard-coded) |
| `worktree: true, pr: true` | `clone` | `true` | Info | None |
| `workflow: direct` (2-value) | `direct` | as-set | None | None |
| `workflow: pr` (2-value) | `clone` | `true` | Info | None |
| `workflow: <new value>` alongside `worktree: bool` | `<new value>` | as-set | Warn | `worktree` ignored |
| `workflow: <new value>` alongside `pr: bool` | `<new value>` | as-set | None | Coexist, no warning |

The behavior change for `worktree: false, pr: true` is intentional — it corrects a bug where the documented `pr: true` flag had no effect. Anyone currently relying on the broken behavior was already not getting a PR they asked for.

## Constraints

- Existing `.dark-factory.yaml` files in sibling projects (billomat, mdm, commerce, and every other project using `worktree: true` or `workflow: direct`/`workflow: pr`) must continue to work without edits. The current 2-value enum validation at `pkg/config/workflow.go` is replaced by 4-value enum validation.
- The reference document is `docs/workflows.md`. Any behavioral change must stay consistent with that doc; if the doc is wrong, fix the doc first.
- Config validation: `autoMerge: true` requires `pr: true`; `autoRelease: true` is independent of `pr` (release runs post-merge or on default-branch commits, see `pkg/config/config.go` Validate).
- `workflow: direct` with `pr: true` must be rejected (no feature branch exists to open a PR from).
- The YOLO container image is unchanged — no new runtime requirements inside the container.
- No changes to the prompt file format, the prompt status lifecycle, the spec file format, or the daemon loop structure.
- **FREEZE** `pkg/git/cloner.go` — clone mode keeps using it as-is; no edits permitted in this work.
- **FREEZE** all docker args in `pkg/executor/executor.go` except exactly ONE new conditional: when `workflow: worktree`, append `--tmpfs /workspace/.git`. Every other mount line stays byte-for-byte identical.
- Worktree cleanup (`git worktree remove`) must run in the same phase as the existing clone cleanup (`cloner.Remove`). If cleanup fails, log a warning and proceed — never block the queue.
- All new Go code must follow project conventions: `github.com/bborbe/errors` wrapping (no `fmt.Errorf`, no bare `return err`), Ginkgo/Gomega tests, Counterfeiter fakes for new interfaces, `libtime` where time is involved.
- Concurrent prompts on the same project using `worktree` mode must produce distinct worktree paths (the existing `<project>-<promptBaseName>` suffix already guarantees this).
- A new `Worktreer` interface in `pkg/git` mirrors `Cloner` (Add/Remove) and is injected into the processor; it does not call into `Cloner`.
- PR-creation logic is extracted from `handleCloneWorkflow` into a helper invoked by branch, worktree, and clone workflows. The helper is a no-op when `p.pr == false`.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `git worktree add` fails because the branch is already a linked worktree elsewhere | Abort with wrapped error; prompt stays in queue, marked `failed` | Human runs `git worktree prune` or removes stale worktree manually |
| `git worktree add` fails because the branch is already checked out in the parent repo (not as a worktree) | Abort with wrapped error; prompt marked `failed` | Human checks out a different branch in the parent, or switches that project to `workflow: clone` |
| `git worktree remove` fails during cleanup | Log warning, continue; worktree dir remains on disk | Human cleans up `/tmp/dark-factory/<name>` and runs `git worktree prune` |
| Docker start fails with `--tmpfs /workspace/.git` flag | Abort with wrapped error; prompt marked `failed` | Operator verifies Docker supports tmpfs (all supported Docker versions do) |
| Container tries to run `git` and fails (`.git` tmpfs is empty) | Prompt's own error handling — typically `make precommit` reports the failure | Prompt should be rewritten to not require git inside, OR switch that project to `workflow: clone` |
| `workflow: branch, pr: true` and `git push` fails | Abort with wrapped error; feature branch exists locally, prompt marked `failed` | Human pushes manually or deletes the branch |
| `findOrCreatePR` fails in non-clone workflow | Abort with wrapped error; branch is pushed but PR not open, prompt marked `failed` | Human opens PR manually via `gh pr create` |
| Legacy config with both `workflow` and `worktree: bool` set | `workflow` wins; warning logged | Human removes `worktree:` from `.dark-factory.yaml` |
| `.dark-factory.yaml` contains unknown workflow value (typo) | Validation error at daemon startup with list of valid values | Human fixes the typo |
| Parent repo's `.git` directory is corrupt | `git worktree add` fails; abort with wrapped error | Operator fixes the parent repo's git state |
| `workflow: direct, pr: true` in config | Validation error at load time | Human picks a non-direct workflow or sets `pr: false` |
| Two queued prompts target the same `branch:` name under `workflow: worktree` or `workflow: branch` | First prompt wins; second fails `git worktree add` (branch already checked out) and is marked `failed` | Human changes one prompt's branch name; paths are already distinct via `<project>-<promptBaseName>` but branches are not |

## Do-Nothing Option

Without this change, huge repos (billomat, mdm, commerce) remain stuck with the slow full-clone setup: every prompt pays tens of minutes of clone time before any work starts. For daemons running dozens of prompts per day, this is hours of wasted compute and wall-clock. Operators work around it by reducing parallelism or disabling dark-factory on those repos, both of which defeat the tool's purpose.

Leaving the boolean surface in place also leaves PR-creation orthogonality broken: `pr: true, worktree: false` users silently get no PR, which is invisible drift. Every new isolation mode would need another boolean and widen the gap.

## Security

- `git worktree add` operates on paths under `/tmp/dark-factory/` with names derived from `projectName + "-" + promptBaseName`. The same uniqueness guarantee that today prevents clone-path collisions (one container per prompt, prompt filenames are unique per project) also prevents worktree-path collisions.
- `--tmpfs /workspace/.git` is a read-write ephemeral mount visible only to the container; contents are discarded on container exit. No host mutation.
- `git worktree remove` only operates on paths dark-factory created; existing path validation applies.
- Extracting PR creation into a shared helper does not change what `findOrCreatePR` sends or who sees it — the GitHub API call is identical.
- No new network endpoints, no new secrets, no new user input.

## Acceptance Criteria

- [ ] `workflow: worktree` in `.dark-factory.yaml` runs real `git worktree add` (verified by integration test or manual E2E on billomat) and the container sees an empty `.git` directory (verified by test asserting `--tmpfs /workspace/.git` is in the docker args).
- [ ] The docker-args diff between today and after this change is exactly ONE line: the `--tmpfs /workspace/.git` flag added only when `workflow: worktree`. Verified by a unit test that snapshots `buildDockerCommand` output for each workflow value and asserts byte-equality against today's baseline for `direct`/`branch`/`clone`.
- [ ] `workflow: branch, pr: true` commits on a new in-place feature branch, pushes it, and opens a PR — verified by unit test with faked brancher + PR helper.
- [ ] `workflow: branch, pr: false` commits on a new in-place feature branch and leaves the branch local (no push, no PR) — verified by unit test.
- [ ] `workflow: clone, pr: false` commits in the clone and pushes the branch but does NOT open a PR — verified by unit test asserting `findOrCreatePR` is not called.
- [ ] `workflow: clone, pr: true` behavior is byte-for-byte unchanged from today (commit in clone, push, PR) — regression tested.
- [ ] PR creation is invoked from a single shared helper; unit test confirms the helper is a no-op when `p.pr == false` and calls `findOrCreatePR` exactly once when `p.pr == true`.
- [ ] Dead code path at `pkg/processor/processor.go:1152` (`featureBranch != "" && !p.pr`) is wired to handle `workflow: branch, pr: false` including `handleBranchCompletion` (last-prompt auto-merge to default, release if applicable).
- [ ] Unit test covers all mapping rows from the Compatibility Matrix (legacy `worktree: bool` × `pr: bool` → new workflow + resolved pr) and the legacy-enum mapping.
- [ ] The `worktree: true, pr: false` override (Row 4) logs an `slog.Warn` naming both legacy fields and telling the user to set `pr: true` explicitly.
- [ ] Setting both `workflow` and the legacy `worktree: bool` produces an `slog.Warn` naming `worktree` as ignored. Setting `workflow` alongside `pr: bool` produces NO warning.
- [ ] Unknown workflow value in config produces a validation error at startup listing valid values (`direct`, `branch`, `worktree`, `clone`). Verified by unit test AND by manual `dark-factory daemon` startup with a bad value.
- [ ] `workflow: direct, pr: true` is rejected at config load with a clear error message. Verified by unit test.
- [ ] `dark-factory config` output contains a line `workflow: <resolved>` and a line `pr: <bool>` after any legacy mapping. Verified by unit test snapshotting CLI output for each of the four legacy inputs in the Compatibility Matrix.
- [ ] `make precommit` passes in `dark-factory` repo.
- [ ] `docs/workflows.md` stays consistent with implementation — any deviation is a bug in the doc.
- [ ] No changes to `.dark-factory.yaml` files in sibling projects are required for them to keep working. Verified by loading each of the following sibling configs through the new loader and asserting resolved behavior matches today's de-facto behavior (per Compatibility Matrix): `~/Documents/workspaces/sm-octopus/billomat/.dark-factory.yaml`, `~/Documents/workspaces/sm-octopus/mdm/.dark-factory.yaml`, `~/Documents/workspaces/sm-octopus/commerce/.dark-factory.yaml`.

## Verification

```
make precommit
```

Also verify end-to-end by running prompts in a sibling project:

1. `workflow: worktree, pr: true` in billomat: no `git clone` output, instead `git worktree add`; `/tmp/dark-factory/<name>` exists as a linked worktree (`git worktree list` in parent); after completion directory is gone; PR opened on GitHub.
2. `workflow: branch, pr: true` on a medium repo: new branch created in-place, commit on it, PR opened; after completion branch still exists locally (or was merged by autoMerge).
3. `workflow: branch, pr: false` on a medium repo: new branch, commit, stay on branch, no push. Operator manually merges or continues work.
4. `workflow: clone, pr: false`: clone created, code committed, branch pushed, NO PR. Clone removed after.

## Reference Docs

- `docs/workflows.md` — complete workflow model (separation + delivery dimensions), mode-by-mode behavior, combinations, migration from legacy bools. Authoritative design document. Prompts must reference it for all behavioral questions.
