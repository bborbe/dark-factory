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
