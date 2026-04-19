---
status: active
---

# Direct workflow commits, tags, and pushes

Validates that a prompt executes on master, creates a commit with version tag, and pushes to remote.

Test repo: copy of `~/Documents/workspaces/dark-factory-sandbox`

## Setup

```bash
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .
WORK_DIR=$(mktemp -d)
cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/dark-factory-sandbox"
cd "$WORK_DIR/dark-factory-sandbox"
cat > .dark-factory.yaml << 'YAML'
workflow: direct
autoRelease: true
YAML
# Redirect push to local bare repo (avoid polluting real remote)
git init --bare "$WORK_DIR/remote.git"
git remote set-url origin "$WORK_DIR/remote.git"
```

- [ ] Repo has `workflow: direct` in `.dark-factory.yaml`
- [ ] Repo has `CHANGELOG.md` with at least one version entry
- [ ] Remote points to local bare repo (not GitHub)

## Action

- [ ] Create `prompts/toggle-comment.md` with content that toggles `// dark-factory-sandbox: scenario test marker` in `math_abs.go` (add if missing, remove if present)
- [ ] `/tmp/new-dark-factory prompt approve toggle-comment`
- [ ] Start dark-factory: `/tmp/new-dark-factory run`

## Expected

- [ ] Prompt executed successfully (check log in `prompts/log/`)
- [ ] Prompt moved to `prompts/completed/` with `status: completed`
- [ ] New commit on master with code changes
- [ ] New version tag created (patch increment)
- [ ] Changes pushed to remote
- [ ] No clone or worktree created

## Cleanup

```bash
rm -rf "$WORK_DIR"
```
