---
status: completed
spec: [063-bug-autorelease-overrides-pr-workflow]
summary: Updated docs/workflows.md Invalid section with new pr+autoMerge+autoRelease rejection, created scenarios 014/015/016 for spec 063, and added CHANGELOG.md Unreleased entry.
container: dark-factory-366-spec-063-docs-and-scenarios
dark-factory-version: v0.145.1-3-g93401a1
created: "2026-05-03T12:00:00Z"
queued: "2026-05-03T11:27:21Z"
started: "2026-05-03T11:46:37Z"
completed: "2026-05-03T11:49:40Z"
---

<summary>
- `docs/workflows.md` Combinations table gains an explicit "Invalid" row documenting `pr: true + autoMerge: false + autoRelease: true` alongside the existing `workflow: direct + pr: true` rejection
- Three new scenario files are created: `scenarios/014-spec-063-config-validation.md`, `scenarios/015-spec-063-branch-pr-path.md`, and `scenarios/016-spec-063-direct-autorelease-regression.md`
- Scenario 014 verifies the daemon fails non-zero at startup with the invalid combo before processing any prompt
- Scenario 015 verifies the happy path: `branch + pr=true + autoMerge=true + autoRelease=true` produces a feature branch, a real PR, an auto-merge, and a release tag on master (not a linear direct commit)
- Scenario 016 verifies the regression case: `workflow: direct + autoRelease: true` continues to commit + tag directly on master (the existing valid behavior must not break)
- No Go code is changed in this prompt
</summary>

<objective>
Document the new invalid-combo rejection in `docs/workflows.md` and write three end-to-end scenario files that prove the bugs introduced in spec 063 are fixed and existing behavior is preserved: (1) the config-load rejection fires before any prompt is processed, (2) the branch+PR+autoMerge+autoRelease happy path produces a real merge commit on master (not a linear direct commit), and (3) the existing `direct + autoRelease: true` flow continues to produce a linear direct commit + tag on master (regression test).
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Read these files in full before editing:
- `docs/workflows.md` — full file; the Combinations table is at ~line 73 and the "Invalid" note is at ~line 87
- `docs/scenario-writing.md` — full file; learn the required format: frontmatter (`status: draft`), title format ("# Scenario NNN: …"), Setup → Action → Expected sections with checkboxes
- `scenarios/002-workflow-pr.md` — read for the sandbox setup pattern (`go build … -o /tmp/new-dark-factory`, `WORK_DIR=$(mktemp -d)`, `cp -r ~/Documents/workspaces/dark-factory-sandbox`) — follow this pattern for both new scenarios
- `scenarios/013-config-layering.md` — read for the multi-scenario-in-one-file format; but note that spec 063 requires TWO separate files per scenario-writing rule 3 ("One journey per file")

The spec this implements: `specs/in-progress/063-bug-autorelease-overrides-pr-workflow.md`
Preconditions: prompts `1-spec-063-config-validation.md` and `2-spec-063-dispatch-fix.md` have been executed.
</context>

<requirements>

## 1. Update `docs/workflows.md` — extend the Invalid note

Locate the "Invalid" note at ~line 87 (directly after the Combinations table):

```
Invalid: `workflow: direct` with `pr: true` (no feature branch exists to open a PR from — validation rejects).
```

Replace it with:

```
Invalid:
- `workflow: direct` with `pr: true` — no feature branch exists to open a PR from; validation rejects.
- `pr: true` + `autoMerge: false` + `autoRelease: true` for any non-direct workflow — `autoRelease` requires tagging the merged commit on master, but `autoMerge: false` means the branch is never merged automatically; validation rejects with three actionable resolutions: set `autoMerge: true`, or set `autoRelease: false`, or set `pr: false`.
```

Do NOT change any other part of the file.

## 2. Create `scenarios/014-spec-063-config-validation.md`

```markdown
---
status: draft
---

# Scenario 014: invalid autoRelease+PR+autoMerge combo is rejected at startup

Validates that starting dark-factory with `pr: true + autoMerge: false + autoRelease: true` exits non-zero before processing any prompt.

Test repo: copy of `~/Documents/workspaces/dark-factory-sandbox`

## Setup

```bash
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .
WORK_DIR=$(mktemp -d)
cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/dark-factory-sandbox"
cd "$WORK_DIR/dark-factory-sandbox"
cat > .dark-factory.yaml << 'YAML'
workflow: branch
pr: true
autoMerge: false
autoRelease: true
YAML
```

- [ ] `.dark-factory.yaml` contains `pr: true`, `autoMerge: false`, `autoRelease: true`
- [ ] No prompts approved in inbox (the rejection must fire before any prompt is processed)

## Action

```bash
/tmp/new-dark-factory daemon > daemon.log 2>&1
echo "exit code: $?"
```

## Expected

- [ ] Command exits non-zero immediately
- [ ] `daemon.log` contains `"autoRelease: true with pr: true and autoMerge: false is invalid"`
- [ ] `daemon.log` contains `"autoMerge: true"` (first resolution)
- [ ] `daemon.log` contains `"autoRelease: false"` (second resolution)
- [ ] `daemon.log` contains `"pr: false"` (third resolution)
- [ ] No prompt is executed (daemon exits before processing queue)

```bash
grep "autoRelease.*invalid\|autoMerge: true\|autoRelease: false\|pr: false" daemon.log
```

## Cleanup

```bash
rm -rf "$WORK_DIR"
```
```

## 3. Create `scenarios/015-spec-063-branch-pr-path.md`

```markdown
---
status: draft
---

# Scenario 015: branch+PR+autoMerge+autoRelease produces a merge commit, not a direct commit

Validates that `workflow: branch + pr: true + autoMerge: true + autoRelease: true` creates a feature branch, opens a PR, auto-merges it, and tags master — producing a merge commit in git history, not a linear direct commit.

Test repo: copy of `~/Documents/workspaces/dark-factory-sandbox`

**Requires:** GitHub remote access for `gh pr create` and `gh pr merge`. Run in a branch with push rights.

## Setup

```bash
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .
WORK_DIR=$(mktemp -d)
cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/dark-factory-sandbox"
cd "$WORK_DIR/dark-factory-sandbox"
cat > .dark-factory.yaml << 'YAML'
workflow: branch
pr: true
autoMerge: true
autoRelease: true
YAML
# Ensure CHANGELOG.md exists so autoRelease can tag
touch CHANGELOG.md
git add CHANGELOG.md && git commit -m "test: add changelog for scenario 015"
git push
```

- [ ] `.dark-factory.yaml` contains `workflow: branch`, `pr: true`, `autoMerge: true`, `autoRelease: true`
- [ ] `CHANGELOG.md` exists (autoRelease only tags when changelog present)
- [ ] No stale `dark-factory/*` branches on remote: `git branch -r | grep dark-factory`
- [ ] GitHub remote accessible: `gh auth status`

## Action

- [ ] Create a minimal prompt in `prompts/toggle-marker.md` that adds/removes a comment in any source file
- [ ] Approve: `/tmp/new-dark-factory prompt approve toggle-marker`
- [ ] Run: `/tmp/new-dark-factory run`
- [ ] Wait for completion

## Expected

- [ ] Exit code 0
- [ ] A `dark-factory/*` feature branch was created on remote: `git branch -r | grep dark-factory`
- [ ] A PR was opened on GitHub: `gh pr list --state all | grep dark-factory`
- [ ] PR was merged (status: `MERGED`): `gh pr list --state merged | grep dark-factory`
- [ ] Master has a **merge commit** in its history (not a linear commit): `git log --oneline --graph -5` shows `Merge branch` or similar — NOT a linear sequence
- [ ] A release tag exists on master pointing at the merge commit: `git tag --list | sort -V | tail -3`
- [ ] Prompt moved to `prompts/completed/` in the sandbox repo

```bash
git log --oneline --graph -5
git tag --list | sort -V | tail -3
gh pr list --state merged
```

## Cleanup

```bash
# Delete the feature branch from remote if not already deleted by autoMerge
git branch -r | grep dark-factory | sed 's/origin\///' | xargs -I{} git push origin --delete {} 2>/dev/null || true
rm -rf "$WORK_DIR"
```
```

## 4. Create `scenarios/016-spec-063-direct-autorelease-regression.md`

Regression scenario per spec 063 acceptance criterion: `workflow: direct + autoRelease: true` must continue to commit + tag directly on master after the dispatch fix in prompt 2. This scenario protects the existing valid behavior from being broken by the branch-creation fix.

```markdown
---
status: draft
---

# Scenario 016: workflow direct + autoRelease still commits + tags on master (regression)

Validates that the dispatch fix in spec 063 prompt 2 does NOT regress the existing `workflow: direct + autoRelease: true` flow. The change must only affect non-direct workflows.

## Setup

- [ ] Build the freshly-modified binary: `go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .`
- [ ] Create sandbox: `WORK_DIR=$(mktemp -d) && cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/sandbox" && cd "$WORK_DIR/sandbox"`
- [ ] `.dark-factory.yaml` set to `workflow: direct` + `autoRelease: true` + `validationCommand: ""` + `validationPrompt: ""`
- [ ] Sandbox repo has a `CHANGELOG.md` with at least one prior version entry
- [ ] One trivial prompt approved in `prompts/in-progress/` (e.g. updates a README line)
- [ ] Capture latest tag and master HEAD before run: `BEFORE_TAG=$(git describe --tags --abbrev=0)`, `BEFORE_HEAD=$(git rev-parse master)`

## Action

- [ ] Start daemon: `/tmp/new-dark-factory run` (one-shot)
- [ ] Wait for prompt to complete (poll `dark-factory status` or wait for exit)

## Expected

- [ ] `git log --oneline --graph master -3` shows linear history — NO merge commit appeared (regression check: direct workflow must not have introduced a branch)
- [ ] `git rev-parse master` is different from `BEFORE_HEAD` — a new commit landed
- [ ] `git describe --tags --abbrev=0` is different from `BEFORE_TAG` — a new tag was created
- [ ] No `dark-factory/*` branch exists locally or on remote (`git branch -a | grep dark-factory` returns empty)
- [ ] `gh pr list --state all` does NOT contain a new PR opened by this run
- [ ] Prompt status is `completed`, file moved to `prompts/completed/`

## Cleanup

```bash
rm -rf "$WORK_DIR"
```
```

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT change any Go source files — this prompt is documentation only
- Scenario files must use `/tmp/new-dark-factory` (not the bare `dark-factory` binary), per the scenario-writing guide rule 2a
- Scenario files must have `status: draft` frontmatter — they are newly created and not yet verified
- Scenario 014 and 015 are separate files (one journey per file, per scenario-writing rule 3)
- The `docs/workflows.md` edit must be a targeted replacement of the existing Invalid note — do NOT replace any other part of the file
- Existing tests must still pass — running `make precommit` after these doc changes must exit 0
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -n "autoMerge.*false.*autoRelease\|autoRelease.*invalid" docs/workflows.md` — new invalid combo documented
2. `ls scenarios/014-spec-063-config-validation.md scenarios/015-spec-063-branch-pr-path.md scenarios/016-spec-063-direct-autorelease-regression.md` — all three files exist
3. `grep "status: draft" scenarios/01[456]-spec-063-*.md` — all three have draft status
4. `grep "/tmp/new-dark-factory" scenarios/01[456]-spec-063-*.md` — all three use the freshly-built binary
5. `grep "merge commit\|Merge branch\|not a linear" scenarios/015-spec-063-branch-pr-path.md` — happy-path scenario explicitly checks for merge commit vs linear
6. `grep "linear history\|NO merge commit\|regression" scenarios/016-spec-063-direct-autorelease-regression.md` — regression scenario explicitly asserts linear history is preserved
</verification>
