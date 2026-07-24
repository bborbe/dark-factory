---
status: active
---

# Auto-approve generated prompts: happy path queues prompts without manual approve

Validates that with `autoApprovePrompts: true`, the daemon audits each prompt produced by `generate-prompts` for an approved spec and, when the audit passes, auto-approves the prompt so it lands in the queue without manual intervention.

Test repo: copy of `~/Documents/workspaces/dark-factory-sandbox`

Note: this scenario performs real LLM calls (1 generate + 1 audit per generated prompt). Budget **up to ~8 minutes** — the generate+audit phase alone has been observed at ~6 min (2026-07-24, claude-yolo v0.14.0), well past the 60–120 s this scenario originally claimed. Size any wait loop accordingly: killing the daemon at the moment `auto-approve: approved generated prompt` lands leaves the prompt queued-but-unexecuted, which looks like the historical "executor never picked up the prompt" bug but is just a premature kill. If that happens, re-run `dark-factory run` in the same sandbox to finish the queued prompt.

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

## Provider Dependency

This scenario is **provider-agnostic**: it hardcodes no LLM provider and runs against whatever the operator's global config (`~/.config/dark-factory/config.yaml`) routes to.

Verified passing:
- **Anthropic claude** (default).
- **MiniMax** model via a local base-URL proxy (`MiniMax-M2.7-highspeed` through `host.docker.internal:8788`) — verified end-to-end on `v0.191.0` (2026-07-05): audit → approve → `found queued prompt` → `NOTES.md` created → `spec awaiting verification`.

History: on `v0.187.11` this failed on minimax — audit+approve succeeded but the executor never picked up the approved prompt. Root cause was the pre-`pkg/promptstate` queue-scan/pickup path, fixed by the spec-101 state-machine refactor (not provider-specific). The direct `https://api.minimax.io/anthropic` endpoint has not been re-tested end-to-end since the fix; the proxy-routed run above is the minimax evidence. See the Personal-vault Dark Factory Debug Guide, "Auto-approve succeeds but executor never picks up the prompt".
