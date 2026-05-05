---
status: draft
---

# Scenario 018: PR body contains prompt summary and spec reference

Validates that `workflow: branch + pr: true` creates a PR whose body includes the prompt's `<summary>` block, the `Spec:` reference from frontmatter, and ends with `Automated by dark-factory`.

Test repo: copy of `~/Documents/workspaces/dark-factory-sandbox`

**Requires:** GitHub remote access for `gh pr create`. Run in a branch with push rights.

## Setup

```bash
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .
WORK_DIR=$(mktemp -d)
cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/dark-factory-sandbox"
cd "$WORK_DIR/dark-factory-sandbox"
cat > .dark-factory.yaml << 'YAML'
workflow: branch
pr: true
YAML
```

- [ ] `.dark-factory.yaml` contains `workflow: branch`, `pr: true`
- [ ] No stale `dark-factory/*` branches on remote: `git branch -r | grep dark-factory`
- [ ] GitHub remote accessible: `gh auth status`

## Action

- [ ] Create a prompt `prompts/scenario-018-test.md` with the following content:

```markdown
---
status: draft
spec:
  - 069-pr-body-rich-content
---

<summary>
dark-factory-scenario-018-marker: this PR tests rich body generation.
</summary>

# Test prompt for scenario 018

Add a comment `# scenario-018` to any source file.
```

- [ ] Approve: `/tmp/new-dark-factory prompt approve scenario-018-test`
- [ ] Run: `/tmp/new-dark-factory run`
- [ ] Wait for completion

## Expected

- [ ] Exit code 0
- [ ] A `dark-factory/*` feature branch was created on remote: `git branch -r | grep dark-factory`
- [ ] A PR was opened on GitHub: `gh pr list --state all | grep dark-factory`
- [ ] PR body contains the marker text: `gh pr view --json body -q .body | grep "dark-factory-scenario-018-marker"`
- [ ] PR body contains the spec reference: `gh pr view --json body -q .body | grep "Spec: 069-pr-body-rich-content"`
- [ ] PR body ends with `Automated by dark-factory` (last line check):

```bash
gh pr view --json body -q .body | tail -1 | grep "Automated by dark-factory"
```

## Cleanup

```bash
# Delete the feature branch from remote if not already deleted
git branch -r | grep dark-factory | sed 's/origin\///' | xargs -I{} git push origin --delete {} 2>/dev/null || true
rm -rf "$WORK_DIR"
```
