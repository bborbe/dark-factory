---
status: active
---

# Skip-preflight flag bypasses baseline gate

Validates that `dark-factory run --skip-preflight` (and `daemon`) proceeds to execute queued
prompts even when the configured `preflightCommand` would fail. Asserts the startup log records
the skip, the prompt executes normally, and no baseline-failure report is emitted.

Test repo: copy of `~/Documents/workspaces/dark-factory-sandbox`

## Setup

```bash
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .
WORK_DIR=$(mktemp -d)
cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/dark-factory-sandbox"
cd "$WORK_DIR/dark-factory-sandbox"
cat > .dark-factory.yaml << 'YAML'
pr: false
worktree: false
preflightCommand: "sh -c 'echo BASELINE_BROKEN_MARKER >&2; exit 1'"
preflightInterval: "0s"
maxContainers: 999
YAML
git init --bare "$WORK_DIR/remote.git"
git remote set-url origin "$WORK_DIR/remote.git"
```

- [ ] `.dark-factory.yaml` sets a failing `preflightCommand` (guaranteed exit 1)
- [ ] Remote points to local bare repo

## Confirm baseline gate is active without flag

Queue a prompt and confirm the flag-less run is blocked (this validates the test setup):

```bash
cat > prompts/skip-preflight-canary.md << 'PROMPT'
---
status: draft
---

<summary>
- Appends a marker comment to math_abs.go
</summary>

<objective>
Append `// skip-preflight-canary` to math_abs.go.
</objective>

<requirements>
1. Append the comment to math_abs.go.
</requirements>

<verification>
```bash
grep -q "skip-preflight-canary" math_abs.go
```
</verification>
PROMPT

/tmp/new-dark-factory prompt approve skip-preflight-canary
timeout 20s /tmp/new-dark-factory run > run-blocked.log 2>&1 || true
```

- [ ] `run-blocked.log` contains `preflight: running baseline check` or `preflight: baseline check FAILED`
- [ ] `math_abs.go` does NOT contain `skip-preflight-canary` (prompt was blocked)
- [ ] Process exited non-zero (preflight failure exits dark-factory)

## Action — run with skip flag

```bash
timeout 60s /tmp/new-dark-factory run --skip-preflight > run-skip.log 2>&1 || true
```

- [ ] Command completed (exit 0 or 1 due to prompt execution, not preflight)
- [ ] `run-skip.log` exists

## Expected

### Startup log records skip
- [ ] `run-skip.log` contains `preflight: baseline check disabled for this invocation`

### No baseline-failure report emitted
- [ ] `run-skip.log` does NOT contain `preflight: baseline check FAILED`
- [ ] `run-skip.log` does NOT contain `preflight: running baseline check`
- [ ] `run-skip.log` does NOT contain `BASELINE_BROKEN_MARKER` outside the `effective config` line (the literal string appears verbatim in the `preflightCommand=...` config dump, which is config inspection, not a failure event). Detection: `grep BASELINE_BROKEN_MARKER run-skip.log | grep -v 'effective config'` returns zero lines.

### Prompt executes through normal flow
- [ ] `run-skip.log` contains evidence of container launch (e.g. `starting container` or `executing prompt`)
- [ ] Prompt moves out of `prompts/` inbox (moved to `prompts/in-progress/` or `prompts/completed/` or `prompts/failed/`)

### Position-agnostic flag
```bash
# Re-queue the canary prompt: move from completed/in-progress back to inbox if needed
mv prompts/completed/skip-preflight-canary.md prompts/skip-preflight-canary.md 2>/dev/null \
  || mv prompts/in-progress/skip-preflight-canary.md prompts/skip-preflight-canary.md 2>/dev/null \
  || true
# Reset content (the prior run modified math_abs.go)
git -C "$WORK_DIR/dark-factory-sandbox" checkout -- math_abs.go 2>/dev/null || true
/tmp/new-dark-factory prompt approve skip-preflight-canary
timeout 60s /tmp/new-dark-factory --skip-preflight run > run-skip2.log 2>&1 || true
```
- [ ] `run-skip2.log` also contains `preflight: baseline check disabled for this invocation`

## Failure modes this catches

| Failure | Symptom |
|---------|---------|
| Flag not extracted in ParseArgs | `--skip-preflight` is treated as a positional arg → "unknown argument" error or args validation failure |
| Flag not threaded to factory | Preflight checker still created; `BASELINE_BROKEN_MARKER` appears in log |
| Flag not position-agnostic | `--skip-preflight run` fails or doesn't set skip |
| Startup log missing | No "baseline check disabled" line in run-skip.log |
| Baseline-failure report emitted despite skip | `BASELINE_BROKEN_MARKER` appears in log |

## Cleanup

```bash
rm -rf "$WORK_DIR"
```
