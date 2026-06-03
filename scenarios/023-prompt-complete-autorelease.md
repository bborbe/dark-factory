---
status: draft
---

# Scenario 023: prompt complete honours autoRelease + branch context + --release

Validates that the fix from spec 092 prompt 1 makes `dark-factory prompt complete <id>` honour `autoRelease` end-to-end and add a branch-context safety default (non-master defaults to commit-only unless `--release` is passed).

## Setup

- [ ] Build the freshly-modified binary: `go build -C /workspace -o /tmp/new-dark-factory .`
- [ ] Create sandbox (master branch): `WORK_DIR=$(mktemp -d) && cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/sandbox" && cd "$WORK_DIR/sandbox"`
- [ ] `.dark-factory.yaml` set to `workflow: direct` + `autoRelease: true` + `validationCommand: ""` + `validationPrompt: ""`
- [ ] Sandbox repo has a `CHANGELOG.md` with at least one prior version entry
- [ ] One trivial prompt approved in `prompts/in-progress/` (e.g. updates a README line)

## Case A — master + autoRelease=true → release fires (regression check)

This case asserts that today's master+`autoRelease=true` behaviour is preserved. If this case fails, scenarios 001 and 016 will also fail.

### Action

- [ ] Confirm current branch is `master`: `git rev-parse --abbrev-ref HEAD` returns `master`
- [ ] Capture state: `BEFORE_TAG=$(git describe --tags --abbrev=0)` and `BEFORE_HEAD=$(git rev-parse HEAD)`
- [ ] Run: `/tmp/new-dark-factory prompt complete <id>`

### Expected

- [ ] Exit code 0
- [ ] `git rev-parse HEAD` is different from `BEFORE_HEAD` — a new commit landed
- [ ] `git describe --tags --abbrev=0` is different from `BEFORE_TAG` — a new tag was created
- [ ] CHANGELOG.md no longer contains `## Unreleased` (it was rewritten to `## vX.Y.Z`)

## Case B — feature branch + autoRelease=true + no --release → no release (the new safety default)

This case is the core fix. Without it, every multi-prompt feature branch cuts an orphan tag.

### Action

- [ ] Create a feature branch: `git checkout -b dark-factory/test-no-release`
- [ ] Approve one more prompt: `/tmp/new-dark-factory prompt approve <id2>`
- [ ] Wait for daemon completion, or run directly: `/tmp/new-dark-factory prompt complete <id2>`
- [ ] Capture state: `BEFORE_TAG=$(git describe --tags --abbrev=0)` and `BEFORE_HEAD=$(git rev-parse HEAD)`

### Expected

- [ ] Exit code 0
- [ ] INFO log line on stderr contains: `branch is "dark-factory/test-no-release", pass --release to force release`
- [ ] `git rev-parse HEAD` is different from `BEFORE_HEAD` — a new commit landed
- [ ] `git describe --tags --abbrev=0` is EQUAL to `BEFORE_TAG` — NO new tag was created
- [ ] CHANGELOG.md STILL contains `## Unreleased` (it was NOT rewritten)

## Case C — feature branch + --release → release fires (explicit opt-in)

This case confirms `--release` overrides the branch default.

### Action

- [ ] Stay on the feature branch: `git rev-parse --abbrev-ref HEAD` returns `dark-factory/test-no-release`
- [ ] Approve one more prompt: `/tmp/new-dark-factory prompt approve <id3>`
- [ ] Run with the flag: `/tmp/new-dark-factory prompt complete <id3> --release`
- [ ] Capture state: `BEFORE_TAG=$(git describe --tags --abbrev=0)` and `BEFORE_HEAD=$(git rev-parse HEAD)`

### Expected

- [ ] Exit code 0
- [ ] `git rev-parse HEAD` is different from `BEFORE_HEAD` — a new commit landed
- [ ] `git describe --tags --abbrev=0` is different from `BEFORE_TAG` — a new tag was created (override worked)
- [ ] CHANGELOG.md no longer contains `## Unreleased`

## Cleanup

```bash
cd ~
rm -rf "$WORK_DIR"
```
