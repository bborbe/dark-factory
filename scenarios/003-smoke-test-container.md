---
status: active
---

# Pre-release container smoke test

Validates that the claude-yolo container boots, Claude starts, edits files, and produces log output. Run before releasing a new dark-factory version that bumps the container image.

Test repo: copy of `~/Documents/workspaces/dark-factory-sandbox`

## Setup

```bash
WORK_DIR=$(mktemp -d)
cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/dark-factory-sandbox"
cd "$WORK_DIR/dark-factory-sandbox"
printf 'pr: false\nworktree: false\n' > .dark-factory.yaml
# Redirect push to local bare repo (avoid polluting real remote)
git init --bare "$WORK_DIR/remote.git"
git remote set-url origin "$WORK_DIR/remote.git"
```

- [ ] Repo has `.dark-factory.yaml` with `pr: false`, `worktree: false`
- [ ] Repo has `CHANGELOG.md` with at least one version entry
- [ ] Remote points to local bare repo (not GitHub)

## Action

Create a minimal prompt that makes a trivial code edit:

```bash
cat > prompts/smoke-test.md << 'PROMPT'
---
status: draft
---

<summary>
- Add a comment marker to math_abs.go for smoke test verification
</summary>

<objective>
Add the comment `// smoke-test-marker` to the end of math_abs.go.
</objective>

<context>
Read `math_abs.go` — add a single comment line at the end of the file.
</context>

<requirements>
1. Append `// smoke-test-marker` as the last line of `math_abs.go`
</requirements>

<constraints>
- Only modify `math_abs.go`
- Do NOT commit
</constraints>

<verification>
```bash
grep -q "smoke-test-marker" math_abs.go
```
</verification>
PROMPT
```

- [ ] `dark-factory prompt approve smoke-test`
- [ ] Start dark-factory: `dark-factory run`

## Expected

### Container starts
- [ ] Log file created in `prompts/log/` (not empty)
- [ ] Log shows `Starting headless session...` as first line
- [ ] No `root/sudo privileges` error in log

### Claude runs and edits files
- [ ] Log shows `[read]` entries (Claude read files)
- [ ] Log shows `[edit]` entries (Claude edited files)
- [ ] `math_abs.go` contains `// smoke-test-marker`

### Prompt completes
- [ ] Prompt moved to `prompts/completed/` with `status: completed`
- [ ] Exit code 0

### Log output works
- [ ] Log contains timestamped entries (`[HH:MM:SS]` format)
- [ ] Log is not truncated (contains completion report or final output)

## Failure modes this catches

| Failure | Symptom |
|---------|---------|
| UID remapping to root | `root/sudo privileges` error, log shows only `Starting headless session...` |
| Prompt quoting broken | `Permission denied` errors, shell tries to execute Go code |
| Container image not found | `docker: Error response from daemon: not found` |
| File write permissions | Claude outputs code as text instead of using edit tools |
| Stream formatter crash | Log file empty after `Starting headless session...` |

## Cleanup

```bash
rm -rf "$WORK_DIR"
```
