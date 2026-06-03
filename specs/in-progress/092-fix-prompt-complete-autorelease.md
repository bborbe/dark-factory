---
status: prompted
approved: "2026-06-03T05:48:03Z"
generating: "2026-06-02T23:08:04Z"
prompted: "2026-06-03T06:01:09Z"
branch: dark-factory/fix-prompt-complete-autorelease
---

## Summary

- The one-shot `dark-factory prompt complete <id>` CLI currently rewrites `## Unreleased` to `## vX.Y.Z`, creates an annotated tag, and pushes — even on a feature branch and even when the operator passed `--set autoRelease=false`.
- The daemon already honours `autoRelease=false`; the direct CLI path does not. The override is silently ignored.
- This spec makes the CLI honour `autoRelease` end-to-end and adds a branch-context safety default: on any non-`master` branch, completion commits but does NOT release, regardless of `autoRelease`.
- Operators who genuinely want to release from a non-`master` branch pass `--release` explicitly.
- After this spec ships, a multi-prompt feature branch produces zero per-prompt releases, and master continues to cut one version after merge as today.

## Problem

`dark-factory prompt complete <id>` calls the releaser unconditionally whenever a `CHANGELOG.md` exists. The `autoRelease` config field is wired only through the daemon's executor path; the CLI builds its releaser without consulting `cfg.AutoRelease`. As a result, every feature-branch `prompt complete` invocation rewrites `## Unreleased` to `## vX.Y.Z`, tags the commit, and pushes the tag — colliding with master's version progression and forcing 20–30 minutes of CHANGELOG / tag conflict resolution per multi-prompt PR. The operator's documented safety net (`--set autoRelease=false`) does not save them. Recent incident: spec 092 work cut three orphan tags (`v0.174.4`, `v0.174.5`, `v0.174.6`) on a feature branch, one of which collided with master's own `v0.174.4` and was silently overwritten on `git fetch`.

## Goal

After this spec ships:

- `dark-factory prompt complete <id>` honours `autoRelease` end-to-end: `autoRelease=false` (via config or `--set`) means commit only — no `## Unreleased` rewrite, no tag, no push.
- On any branch other than `master`, `prompt complete` defaults to commit-only behaviour regardless of `autoRelease`, unless the operator passes `--release`.
- On `master` with `autoRelease=true` (today's default in the dark-factory repo), behaviour is unchanged: the CHANGELOG rewrite, tag, and push still happen.
- The `--release` flag forces release on any branch, overriding both the branch-context default and `autoRelease=false`. Explicit flag beats config, matching `--set` precedence. It is the explicit opt-in for the rare case where the operator genuinely wants to release from a non-`master` branch.
- The new semantics are documented in `docs/configuration.md`, `docs/running.md`, and the `prompt complete --help` output.

## Non-goals

- Do NOT change daemon behaviour. The daemon already honours `autoRelease=false`; the executor path stays as-is.
- Do NOT change `prompt approve`, `prompt requeue`, `prompt cancel`, or `spec approve` — none of those cut releases.
- Do NOT auto-rewind a release that was created accidentally on a feature branch. If the operator runs `--release` by mistake, they recover manually.
- Do NOT add a `--no-release` flag or any other way to disable release on `master` beyond setting `autoRelease=false`. One opt-out path (config / `--set`), one opt-in flag (`--release`).
- Do NOT make the `master` branch name configurable via this spec. If a project uses a different default branch, that is a separate spec.
- Do NOT inherit daemon in-memory state via lock file or sidecar config. The CLI reads `.dark-factory.yaml` + `--set` as today.

## Desired Behavior

1. `prompt complete` reads `autoRelease` from the same config layering chain as the daemon: default `false` ← global config ← project `.dark-factory.yaml` ← `--set autoRelease=…`.
2. `prompt complete` detects the current git branch. If the branch is not `master`, the command defaults to commit-only behaviour: no CHANGELOG rewrite, no tag, no push.
3. The `--release` CLI flag forces the release path on any branch. When present, behaviour matches today's master-branch-with-`autoRelease=true` path.
4. When `autoRelease=false` (config or `--set`), `prompt complete` is commit-only on every branch, including `master`. `--release` overrides this only when the operator explicitly passes it.
5. When `autoRelease=true` and the branch is `master` and `--release` is not passed, behaviour is unchanged from today: commit, rewrite `## Unreleased` to `## vX.Y.Z`, tag, push.
6. When the operator is on a non-`master` branch and the command takes the commit-only path because of the branch default, the command prints a one-line INFO log naming the reason: branch is `<name>`, pass `--release` to force release.
7. `prompt complete --help` documents the `--release` flag and the branch-context default in one or two sentences.

## Constraints

- `NewPromptCompleteCommand`'s exported signature MAY grow new parameters (this is an internal constructor used only by `pkg/factory`). Existing callers in `pkg/factory/factory.go` must be updated in the same change.
- The releaser library `pkg/git/git.go` `NewReleaser` does NOT need to grow an `autoRelease` parameter. The gating decision lives in the command layer (`completeDirectWorkflow`), not in the releaser. This keeps the releaser focused on "how to release" and the command focused on "whether to release".
- Existing scenarios 001 (`workflow-direct`) and 016 (`direct-autorelease-regression`) MUST continue to pass without modification. They run on `master` with `autoRelease=true` and expect a tag + push — today's behaviour is preserved.
- The daemon executor path (`pkg/factory/factory.go` around line 434) must NOT be touched by this spec. Its `autoRelease` handling is already correct; double-gating risks regressing it.
- Config layering documented in `docs/config-layering.md` is the source of truth for how `autoRelease` is resolved; reference it from the spec / prompts rather than re-deriving precedence rules.
- The branch name compared against is the literal string `master`. Repos with `main` as their default branch are out of scope for this spec.
- Branch detection uses `git rev-parse --abbrev-ref HEAD` (subprocess), matching the rest of `pkg/git/git.go`'s subprocess discipline. The CLI does NOT read `.git/HEAD` directly.

## Failure Modes

| Trigger | Detection | Expected behavior | Reversibility | Recovery |
|---------|-----------|-------------------|---------------|----------|
| `git rev-parse --abbrev-ref HEAD` fails (detached HEAD, no `.git`, corrupt repo) | Command stderr returns non-zero | `prompt complete` exits non-zero before any commit/push; error message names the underlying git failure | Reversible (nothing happened) | Operator fixes the git state (checkout a branch, repair `.git`) and re-runs |
| Operator is on `master` with `autoRelease=false` and forgets to pass `--release` | No tag created; `git describe --tags --abbrev=0` unchanged | Commit-only path executes. INFO log notes `autoRelease=false`. No CHANGELOG rewrite, no tag, no push | Reversible (no remote artifact) | Operator passes `--release` on the next invocation, or runs the release manually |
| Operator is on a feature branch with `--release` passed | Tag + push happen | Release path executes: CHANGELOG rewrite, tag, push to remote. This is the documented opt-in | Irreversible once pushed (tag exists on remote) | Operator manually deletes the orphan tag (`git push origin :refs/tags/vX.Y.Z` + `git tag -d vX.Y.Z`) and resets `## Unreleased`. Same recovery as today's accidental case |
| `autoRelease=true` on a feature branch without `--release` | No tag created on the branch | Commit-only path executes. INFO log notes branch is `<name>`, pass `--release` to force release | Reversible (no remote artifact) | None needed — this is the new default that prevents the incident in the task source |
| Branch name detection returns `master` on a worktree that is actually a feature branch (operator confused) | Tag created when not intended | Release path executes as if on master. This matches operator's apparent intent (HEAD says master) | Irreversible once pushed | Operator inspects worktrees, deletes orphan tag, resets `## Unreleased`. Same recovery as today |
| `--release` is passed but `autoRelease=false` in config | Tag + push happen | `--release` wins. CLI semantics: explicit flag overrides config. Same precedence as `--set` overriding config | Irreversible once pushed | Operator deletes orphan tag if unintended; same recovery as above |

## Security / Abuse Cases

Not applicable — this command runs locally on the operator's machine, takes no untrusted input, and changes only git state under the operator's control.

## Acceptance Criteria

- [ ] `make precommit` exits 0 from the repo root — evidence: exit code
- [ ] `dark-factory prompt complete --help` output contains the string `--release` and a one-line description naming "force release on non-master branch" — evidence: `dark-factory prompt complete --help 2>&1 | grep -c -- '--release'` returns ≥1
- [ ] `pkg/cmd/prompt_complete.go` reads `autoRelease` and a `--release` flag and gates `CommitAndRelease` accordingly — evidence: `grep -n 'autoRelease\|--release\|forceRelease' pkg/cmd/prompt_complete.go` returns ≥3 matching lines
- [ ] `pkg/factory/factory.go` `CreatePromptCompleteCommand` passes `cfg.AutoRelease` into the command constructor — evidence: `grep -n -A2 'CreatePromptCompleteCommand' pkg/factory/factory.go | grep -c 'AutoRelease'` returns ≥1
- [ ] Unit test covers the branch-context default: feature branch + `autoRelease=true` + no `--release` → `CommitAndRelease` is NOT called, `CommitOnly` IS called — evidence: `grep -n 'TestPromptComplete.*NonMaster\|TestPromptComplete.*Branch\|TestCompleteOnNonMasterBranch' pkg/cmd/prompt_complete_test.go` returns ≥1 and the test asserts on mock call counts
- [ ] Unit test covers the override: feature branch + `--release` → `CommitAndRelease` IS called — evidence: test name contains `release_flag` or equivalent and asserts on mock call counts
- [ ] Unit test covers the master + `autoRelease=false` path: `master` + `autoRelease=false` + no `--release` → `CommitAndRelease` is NOT called — evidence: test asserts on mock call counts
- [ ] Unit test covers the master + `autoRelease=true` regression path: `master` + `autoRelease=true` + no `--release` → `CommitAndRelease` IS called (today's behaviour preserved) — evidence: test asserts on mock call counts
- [ ] `docs/configuration.md` `autoRelease semantics` paragraph (around line 37) is updated to state that the CLI's `prompt complete` honours `autoRelease` and defaults to commit-only on non-`master` branches — evidence: `grep -n -A2 'autoRelease semantics' docs/configuration.md` includes the phrase "non-master" or equivalent
- [ ] `docs/running.md` documents the `--release` flag in the `prompt complete` section — evidence: `grep -n -- '--release' docs/running.md` returns ≥1
- [ ] `CHANGELOG.md` `## Unreleased` section has a bullet describing the fix — evidence: `grep -c 'autoRelease\|--release' CHANGELOG.md` returns ≥1 in the `## Unreleased` block (verified by `awk '/^## /{p=0} /^## Unreleased/{p=1} p' CHANGELOG.md | grep -c -- '--release'` returns ≥1)
- [ ] Scenario `scenarios/023-prompt-complete-autorelease.md` exists with `status: draft` (or `active` after first run) and covers three cases: master + autoRelease=true → release fires; feature branch + autoRelease=true (no `--release`) → no release fires; feature branch + `--release` → release fires — evidence: `grep -c 'master\|feature\|--release' scenarios/023-prompt-complete-autorelease.md` returns ≥3
- [ ] Existing scenario 016 (`016-spec-063-direct-autorelease-regression.md`) replays green without modification on `master` — evidence: walkthrough completes with every `## Expected` checkbox tickable

## Verification

```
cd /Users/bborbe/Documents/workspaces/dark-factory-fix-autorelease
make precommit
```

Then walk scenario `scenarios/023-prompt-complete-autorelease.md` (all three cases) and scenario `scenarios/016-spec-063-direct-autorelease-regression.md` (no regression).

## Do-Nothing Option

Cost of not doing this: every multi-prompt feature branch in dark-factory itself costs 20–30 minutes of CHANGELOG + tag conflict resolution after merge, and risks merging with the wrong version progression (orphan tag silently overwriting a master tag, as happened with `v0.174.4` on 2026-06-02). The operator's documented safety net (`--set autoRelease=false`) continues to lie. Fleet rollout multiplies this on every project that picks up dark-factory and runs multi-prompt feature branches. Not acceptable.

## Open Questions Resolved During Drafting

- **Default = branch-context** (non-master no-release) — chosen because it mirrors the Development Guide's "release happens once on master after merge" contract and removes the operator's need to remember a flag on every feature-branch run.
- **Override flag name = `--release`** — short, low-friction, matches the `--set autoRelease=…` config field name.
- **Single spec** — covers cmd + factory wiring + docs + scenario in one DB×AC unit. Two code layers (cmd, factory) and one scenario do not warrant a Suggested Decomposition table per the scope rules.
