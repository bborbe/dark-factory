---
status: idea
---

# Auto-approve generated prompts: audit failure stops further auto-approvals for that spec

Validates that with `autoApprovePrompts: true`, when the audit of a generated prompt fails, the daemon stops auto-approving further prompts produced for the same spec; the spec stays in `prompted` state for human intervention; prompts already approved continue to execute.

Test repo: copy of `~/Documents/workspaces/dark-factory-sandbox`

Note: this scenario requires a deterministic way to fail audit for the *first* generated prompt while *still* producing at least one more prompt for the same spec. The trigger is left as `idea` until either:
1. `/dark-factory:audit-prompt` exposes a deterministic-fail mode (e.g. an env var or a magic-string in prompt content), OR
2. We accept LLM-driven flakiness and write a deliberately weak prompt that the audit slash command consistently rejects.

## Setup (tentative)

```bash
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .
WORK_DIR=$(mktemp -d)
cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/sandbox"
cd "$WORK_DIR/sandbox"
cat > .dark-factory.yaml << 'YAML'
workflow: direct
autoRelease: false
autoApprovePrompts: true
maxContainers: 999
YAML
git init --bare "$WORK_DIR/remote.git" >/dev/null 2>&1
git remote set-url origin "$WORK_DIR/remote.git"

# Spec describes two distinct changes so the generator emits ≥2 prompts.
# First prompt should be structurally weak enough that the audit consistently fails;
# second prompt should be sound. Exact wording TBD.
```

## Action (tentative)

- [ ] Approve the spec
- [ ] Start daemon
- [ ] Wait for daemon to log audit failure for prompt 1
- [ ] Stop daemon

## Expected (tentative)

- [ ] Daemon log contains `auto-approve: audit FAILED for generated prompt` for prompt 1
- [ ] Daemon log does NOT contain `auto-approve: auditing generated prompt prompt=…` for prompt 2 (auditing stopped after first failure)
- [ ] Both generated prompts remain in `prompts/` inbox (neither was approved)
- [ ] Spec frontmatter `status` is still `prompted` (NOT `verifying`, NOT `completed`)
- [ ] Prompts from any unrelated approved spec continue to execute

## Open questions before promoting to `draft`

1. Deterministic-fail mechanism for `/dark-factory:audit-prompt` — env var, magic content, separate audit binary, or accept flakiness?
2. Should we lock down "prompts from other specs unaffected" with a second spec in the same setup, or is single-spec coverage sufficient?
3. How long do we wait for the daemon to surface the failure? (LLM audit latency varies.)

## What this scenario would lock down (when finished)

| Failure | Symptom this scenario would catch |
|---------|-----------------------------------|
| Stop-the-spec logic broken: audit fail still auto-approves prompt N+1 | `auto-approve: auditing` line for prompt 2 appears in log |
| Audit failure incorrectly propagates as a fatal error from `finalizePrompted` | Spec status flips to `failed` instead of staying `prompted` |
| Failure handler not surfaced clearly (no log, no spec status hint) | Operator has no visible signal to intervene |
