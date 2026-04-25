---
status: active
---

# Spec auto-transitions to verifying after last linked prompt completes

Validates that when the last prompt linked to a spec finishes, the spec auto-transitions from `prompted` to `verifying` without requiring a daemon restart. Regression guard for the order-of-operations bug fixed in `fix-stuck-prompted-specs.md` (workflow_executor_direct phase 2/3 swap) and the periodic sweep ticker.

Test repo: copy of `~/Documents/workspaces/dark-factory-sandbox`

Note: dark-factory enforces sequential prompt execution — prompt N requires all prompts 1..N-1 to be in `prompts/completed/`. The sandbox already has `001-test-plugin-resolution.md` in completed, so this scenario uses prompt number `002`.

## Setup

```bash
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .
WORK_DIR=$(mktemp -d)
cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/sandbox"
cd "$WORK_DIR/sandbox"
cat > .dark-factory.yaml << 'YAML'
workflow: direct
autoRelease: false
YAML
git init --bare "$WORK_DIR/remote.git" >/dev/null 2>&1
git remote set-url origin "$WORK_DIR/remote.git"

# Pre-place a spec at prompted state with one linked prompt approved+ready.
# Skip the daemon's spec-generation step (which requires an LLM call) — the
# auto-transition lives downstream of generation, so we test it directly.
mkdir -p specs/in-progress prompts/in-progress
cat > specs/in-progress/099-tiny-spec.md << 'SPEC'
---
status: prompted
approved: "2026-04-25T12:00:00Z"
prompted: "2026-04-25T12:01:00Z"
---

## Summary
- Tiny spec with one linked prompt for the auto-transition regression test.

## Acceptance Criteria
- [ ] Prompt completes
- [ ] Spec auto-transitions to verifying
SPEC

cat > prompts/in-progress/002-tiny-prompt.md << 'PROMPT'
---
status: approved
spec: [099-tiny-spec]
created: "2026-04-25T12:00:00Z"
---

<summary>
- Append a marker line to math_abs.go.
</summary>

<requirements>
1. Append the line `// spec-lifecycle-marker` to `math_abs.go` if it is not already present.
</requirements>

<verification>
```bash
grep -q "spec-lifecycle-marker" math_abs.go
```
</verification>
PROMPT
```

- [ ] `specs/in-progress/099-tiny-spec.md` exists with `status: prompted`
- [ ] `prompts/in-progress/002-tiny-prompt.md` exists with `status: approved` and `spec: [099-tiny-spec]`
- [ ] Remote points to local bare repo
- [ ] No daemon is running

## Action

```bash
cd "$WORK_DIR/sandbox"
timeout 180s /tmp/new-dark-factory run > daemon.log 2>&1 || true
```

- [ ] `dark-factory run` exits within 180 seconds (one-shot mode)
- [ ] `daemon.log` exists

## Expected

### Prompt executes and lands in completed/

```bash
cd "$WORK_DIR/sandbox"
ls prompts/completed/ prompts/in-progress/
grep -q "spec-lifecycle-marker" math_abs.go && echo "marker present"
```

- [ ] `prompts/in-progress/` does NOT contain `002-tiny-prompt.md`
- [ ] `prompts/completed/002-tiny-prompt.md` exists
- [ ] `math_abs.go` contains `spec-lifecycle-marker`

### Spec auto-transitions to verifying

```bash
cd "$WORK_DIR/sandbox"
head -10 specs/in-progress/099-tiny-spec.md
```

- [ ] `specs/in-progress/099-tiny-spec.md` frontmatter has `status: verifying`
- [ ] Frontmatter has a `verifying:` timestamp
- [ ] Daemon log contains the line `spec awaiting verification` referencing `099-tiny-spec`

### Regression guards

This combination would have failed before `fix-stuck-prompted-specs`:

- [ ] Auto-transition does NOT require a daemon restart (the test ran a single one-shot daemon and checked status afterwards — no second `dark-factory run` invocation needed)
- [ ] Daemon log shows the post-completion auto-complete call: grep for `spec auto-complete` or `spec awaiting verification` in `daemon.log`

## Cleanup

```bash
rm -rf "$WORK_DIR"
rm -f /tmp/new-dark-factory
```

## What this scenario locks down

| Failure | Symptom this scenario would catch |
|---------|-----------------------------------|
| Phase ordering bug (CheckAndComplete before MoveToCompleted) | Spec stays at `status: prompted` after `dark-factory run` exits |
| Sweep ticker silently disabled | Same as above (sweep would have auto-corrected within 60s if active) |
| `pf.Specs()` discovery breaks | Daemon log lacks `spec auto-complete` line |
| Auto-transition timestamp missing | `verifying:` field absent from frontmatter |
