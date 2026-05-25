---
status: active
---

# Auto-approve generated prompts: happy path queues prompts without manual approve

Validates that with `autoApprovePrompts: true`, the daemon audits each prompt produced by `generate-prompts` for an approved spec and, when the audit passes, auto-approves the prompt so it lands in the queue without manual intervention.

Test repo: copy of `~/Documents/workspaces/dark-factory-sandbox`

Note: this scenario performs real LLM calls (1 generate + 1 audit per generated prompt). Expect 60–120 s total runtime.

## Setup

```bash
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .
WORK_DIR=$(mktemp -d)
cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/sandbox"
cd "$WORK_DIR/sandbox"
cat > .dark-factory.yaml << 'YAML'
workflow: direct
autoRelease: false
autoGeneratePrompts: true
autoApprovePrompts: true
maxContainers: 999
YAML
git init --bare "$WORK_DIR/remote.git" >/dev/null 2>&1
git remote set-url origin "$WORK_DIR/remote.git"

# Spec deliberately tiny → generator should emit a single small prompt.
mkdir -p specs
cat > specs/add-notes-marker.md << 'SPEC'
---
status: draft
---

## Summary
- Add a one-line `NOTES.md` file at repo root containing the literal string `auto-approve scenario marker`.

## Goal
A new file `NOTES.md` exists at repo root with the marker line so future readers know this commit was produced via the auto-approve path.

## Desired Behavior
1. A file `NOTES.md` exists at the repo root.
2. Its first line is exactly `auto-approve scenario marker`.

## Constraints
- Do not modify any other file.
- Do not commit — dark-factory handles git.

## Acceptance Criteria
- [ ] `NOTES.md` exists at repo root.
- [ ] First line of `NOTES.md` is `auto-approve scenario marker`.
- [ ] `make precommit` exits 0.
SPEC

/tmp/new-dark-factory spec approve add-notes-marker
```

- [ ] `.dark-factory.yaml` contains `autoApprovePrompts: true`
- [ ] `specs/in-progress/NNN-add-notes-marker.md` exists with `status: approved`
- [ ] `prompts/` inbox is empty before the run
- [ ] No daemon is running

## Action

```bash
cd "$WORK_DIR/sandbox"
timeout 360s /tmp/new-dark-factory daemon > daemon.log 2>&1 &
DAEMON_PID=$!

# Wait for the spec to reach 'verifying' (= generation+audit+approve+exec all done) or timeout
for i in $(seq 1 72); do
    grep -q "spec awaiting verification" daemon.log 2>/dev/null && break
    sleep 5
done

kill $DAEMON_PID 2>/dev/null || true
wait $DAEMON_PID 2>/dev/null || true
```

- [ ] Daemon exited cleanly (or was killed after the verification log line appeared)
- [ ] `daemon.log` contains the line `spec awaiting verification` for `add-notes-marker`

## Expected

### Generated prompt was auto-approved (no manual `prompt approve` ran)

```bash
cd "$WORK_DIR/sandbox"
ls prompts/
ls prompts/in-progress/ prompts/completed/
grep -E "auto-approve: auditing|auto-approve: approved" daemon.log
```

- [ ] `prompts/` inbox is empty (no prompt was left waiting for manual approve)
- [ ] `prompts/completed/` contains the generated prompt
- [ ] Daemon log contains `auto-approve: auditing generated prompt`
- [ ] Daemon log contains `auto-approve: approved generated prompt`

### Effective config logged the new field

```bash
grep "autoApprovePrompts=true" daemon.log
grep "autoApprovePromptsSource=project" daemon.log
```

- [ ] Daemon startup log shows `autoApprovePrompts=true`
- [ ] Daemon startup log shows `autoApprovePromptsSource=project`

### Generated prompt actually executed

```bash
cd "$WORK_DIR/sandbox"
cat NOTES.md
```

- [ ] `NOTES.md` exists at repo root
- [ ] First line of `NOTES.md` is `auto-approve scenario marker`

## Cleanup

```bash
rm -rf "$WORK_DIR"
rm -f /tmp/new-dark-factory
```

## What this scenario locks down

| Failure | Symptom this scenario would catch |
|---------|-----------------------------------|
| `autoApprovePrompts` config never reaches generator | `auto-approve: auditing` line missing from daemon log |
| Audit invocation never fires inside YOLO | Same as above |
| Audit passes but approve step is skipped | Generated prompt remains in `prompts/` inbox after the run |
| Audit invocation uses a new mechanism instead of the existing `executor.Execute` path | Container with name `dark-factory-audit-*` not visible / executor instrumentation differs |
| `autoApprovePromptsSource` not reported in effective-config log | Operators cannot tell which layer set the value |
