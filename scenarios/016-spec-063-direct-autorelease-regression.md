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
