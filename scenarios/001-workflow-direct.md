---
status: active
---

# Direct workflow commits, tags, and pushes

Validates that a prompt executes on master, creates a commit with version tag, and pushes to remote.

Test repo: copy of `~/Documents/workspaces/dark-factory-sandbox`

## Setup

```bash
source ~/Documents/workspaces/dark-factory/scenarios/helper/lib.sh
build_binary
setup_sandbox_copy "$(printf 'workflow: direct\nautoRelease: true\nmaxContainers: 999\n')" dark-factory-sandbox
```

`setup_sandbox_copy` sets `WORK_DIR`, copies the sandbox into `$WORK_DIR/dark-factory-sandbox`, writes `.dark-factory.yaml`, creates a local bare remote at `$WORK_DIR/remote.git`, redirects `origin` to it, and `cd`s into the sandbox. An EXIT trap removes `$WORK_DIR` when the shell exits.

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

Automatic — the EXIT trap registered by `setup_sandbox_copy` removes `$WORK_DIR` when the shell exits. To clean up explicitly mid-session:

```bash
cleanup_sandbox
```
