---
status: active
---

# PR workflow creates clone, opens PR, cleans up

Validates that a prompt creates a clone, executes in isolation, pushes a feature branch, and opens a PR.

Test repo: copy of `~/Documents/workspaces/dark-factory-sandbox`

## Setup

**Note:** PR workflow requires a real GitHub remote for `gh pr create`. This scenario pushes to the real sandbox repo — clean up the PR and branch after the run.

```bash
WORK_DIR=$(mktemp -d)
cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/dark-factory-sandbox"
cd "$WORK_DIR/dark-factory-sandbox"
echo "workflow: pr" > .dark-factory.yaml
```

- [ ] Repo has `workflow: pr` in `.dark-factory.yaml`
- [ ] No stale clones from previous runs (`ls $TMPDIR/dark-factory/`)

## Action

- [ ] Create `prompts/toggle-comment.md` with content that toggles `// dark-factory-sandbox: scenario test marker` in `math_abs.go` (add if missing, remove if present)
- [ ] `dark-factory prompt approve toggle-comment`
- [ ] Start dark-factory: `dark-factory run`

## Expected

- [ ] Clone created during execution at `$TMPDIR/dark-factory/dark-factory-sandbox-*`
- [ ] Feature branch `dark-factory/*` pushed
- [ ] PR opened on GitHub
- [ ] Clone removed after completion
- [ ] Prompt moved to `prompts/completed/` in original repo
- [ ] Master branch has no new commits (changes are on the feature branch only)

## Cleanup

```bash
rm -rf "$WORK_DIR"
```
