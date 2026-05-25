---
status: active
---

# Preflight baseline gate

Validates that `preflightCommand` gates prompt execution: when the configured baseline command exits non-zero, dark-factory reports the failure with captured output, keeps the prompt queued, and does not execute it. The gate runs on a cadence (`preflightInterval`) without busy-looping.

Test repo: copy of `~/Documents/workspaces/dark-factory-sandbox`

## Setup

```bash
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .
WORK_DIR=$(mktemp -d)
cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/dark-factory-sandbox"
cd "$WORK_DIR/dark-factory-sandbox"

# preflightCommand validation rejects shell metacharacters (security hardening),
# so the command must be a path to an executable, not an inline shell snippet.
cat > "$WORK_DIR/baseline-fail.sh" << 'SCRIPT'
#!/bin/sh
echo BASELINE_BROKEN_MARKER >&2
exit 1
SCRIPT
chmod +x "$WORK_DIR/baseline-fail.sh"

cat > .dark-factory.yaml << YAML
pr: false
worktree: false
preflightCommand: "$WORK_DIR/baseline-fail.sh"
preflightInterval: "0s"
maxContainers: 999
YAML
git init --bare "$WORK_DIR/remote.git"
git remote set-url origin "$WORK_DIR/remote.git"
```

- [ ] `.dark-factory.yaml` sets a failing `preflightCommand` (path to `baseline-fail.sh`) and `preflightInterval: 0s` (no cache)
- [ ] `$WORK_DIR/baseline-fail.sh` exists and is executable
- [ ] Remote points to local bare repo

## Action

Queue a trivial prompt and run the daemon for a bounded window:

```bash
cat > prompts/preflight-canary.md << 'PROMPT'
---
status: draft
---

<summary>
- Appends a marker comment to math_abs.go
</summary>

<objective>
Append `// preflight-canary` to math_abs.go.
</objective>

<requirements>
1. Append the comment to math_abs.go.
</requirements>

<verification>
```bash
grep -q "preflight-canary" math_abs.go
```
</verification>
PROMPT

/tmp/new-dark-factory prompt approve preflight-canary
timeout 30s /tmp/new-dark-factory daemon > daemon.log 2>&1 || true
```

- [ ] Daemon exited (fail-fast on baseline failure — typically within a few seconds, not a full 30s timeout)
- [ ] `daemon.log` exists

## Expected

### Baseline check runs
- [ ] `daemon.log` contains `preflight: running baseline check`
- [ ] `daemon.log` contains `preflight: baseline check FAILED`

### Failure report includes command and captured output
- [ ] `daemon.log` contains the configured command path (the `baseline-fail.sh` path from `$WORK_DIR`)
- [ ] `daemon.log` contains the captured stderr token `BASELINE_BROKEN_MARKER` as part of the FAILED report (not just inside the config-dump line)

### Daemon exits fail-fast (does NOT busy-loop, does NOT keep retrying)
- [ ] `daemon.log` contains `preflight baseline broken — dark-factory exiting`
- [ ] `daemon.log` contains exactly ONE occurrence of `baseline check FAILED` (no retry loop)
- [ ] `daemon.log` total line count < 50 (fail-fast exit, not cadence loop)

### Prompt stays queued, does not execute
- [ ] `math_abs.go` does NOT contain `preflight-canary`
- [ ] Prompt file is still in `prompts/in-progress/` with status `approved` (not moved to `completed/` or `failed/`)
- [ ] No container launched (no `[read]` / `[edit]` entries under `prompts/log/`)

## Green path (optional extension)

Flip `preflightCommand` to `"true"` and repeat:
- [ ] `daemon.log` contains `preflight: baseline check passed`
- [ ] Prompt executes, `math_abs.go` contains `preflight-canary`
- [ ] Prompt moves to `prompts/completed/`

## Failure modes this catches

| Failure | Symptom |
|---------|---------|
| Yaml value silently dropped by loader | No `preflight: running` lines; prompt executed |
| Busy-loop on preflight skip | `daemon.log` grows faster than the configured interval |
| Failure report missing output | No `BASELINE_BROKEN_MARKER` in log |
| Failure report missing command | Command string absent from failure log |
| Prompt executed despite red baseline | `math_abs.go` contains `preflight-canary` |

## Cleanup

```bash
rm -rf "$WORK_DIR"
```
