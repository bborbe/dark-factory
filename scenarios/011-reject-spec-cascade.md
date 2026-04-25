---
status: draft
---

# Spec reject cascades to linked prompts and is excluded from default lists

Validates that `dark-factory spec reject` rejects a spec and all its linked prompts atomically, writes audit metadata, places files in `rejected/` directories, and that list commands hide rejected items by default.

Test repo: copy of `~/Documents/workspaces/dark-factory-sandbox`

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
git init --bare "$WORK_DIR/remote.git"
git remote set-url origin "$WORK_DIR/remote.git"

# Place spec directly in in-progress (skip approve flow — we want a deterministic state)
mkdir -p specs/in-progress prompts/in-progress
cat > specs/in-progress/099-throwaway-feature.md << 'SPEC'
---
status: approved
approved: "2026-04-25T12:00:00Z"
---

## Summary
- Throwaway spec for the reject scenario.
SPEC

# Place two linked prompts directly in in-progress with `spec:` reference
cat > prompts/in-progress/990-throwaway-feature-a.md << 'EOF'
---
status: approved
spec: [099-throwaway-feature]
created: "2026-04-25T12:00:00Z"
---

<summary>
- Throwaway prompt A.
</summary>
EOF

cat > prompts/in-progress/991-throwaway-feature-b.md << 'EOF'
---
status: approved
spec: [099-throwaway-feature]
created: "2026-04-25T12:00:00Z"
---

<summary>
- Throwaway prompt B.
</summary>
EOF
```

- [ ] `specs/in-progress/099-throwaway-feature.md` exists with `status: approved`
- [ ] `prompts/in-progress/990-throwaway-feature-a.md` and `991-throwaway-feature-b.md` exist with `status: approved` and `spec: [099-throwaway-feature]` (note: daemon migrates `spec:` references to the full-slug form on prompt generation; cascading reject finds prompts via this exact slug)
- [ ] No daemon is running

## Action

```bash
cd "$WORK_DIR/sandbox"
/tmp/new-dark-factory spec reject 099 --reason "scenario regression test"
```

- [ ] Command exits 0 and prints a confirmation referencing both linked prompts

## Expected

### Spec is rejected with audit metadata

```bash
cd "$WORK_DIR/sandbox"
ls specs/in-progress/ specs/rejected/
cat specs/rejected/099-throwaway-feature.md | head -10
```

- [ ] `specs/in-progress/` does NOT contain `099-throwaway-feature.md`
- [ ] `specs/rejected/099-throwaway-feature.md` exists (numeric prefix preserved)
- [ ] Spec frontmatter has `status: rejected`
- [ ] Spec frontmatter has `rejected:` matching regex `^rejected: 2[0-9]{3}-[01][0-9]-[0-3][0-9]T`
- [ ] Spec frontmatter has `rejected_reason: scenario regression test`

### Linked prompts cascaded

```bash
cd "$WORK_DIR/sandbox"
ls prompts/in-progress/ prompts/rejected/
```

- [ ] `prompts/in-progress/` does NOT contain either throwaway prompt
- [ ] `prompts/rejected/` contains both `990-throwaway-feature-a.md` and `991-throwaway-feature-b.md`
- [ ] Each rejected prompt's frontmatter has `status: rejected`, `rejected:` (RFC3339), and `rejected_reason: scenario regression test`

### List commands hide rejected by default; --all shows them

```bash
cd "$WORK_DIR/sandbox"
/tmp/new-dark-factory spec list
/tmp/new-dark-factory spec list --all
/tmp/new-dark-factory prompt list
/tmp/new-dark-factory prompt list --all
```

- [ ] `spec list` (no flag) does NOT mention `throwaway-feature`
- [ ] `spec list --all` DOES mention `throwaway-feature` with status `rejected`
- [ ] `prompt list` (no flag) does NOT mention either throwaway prompt
- [ ] `prompt list --all` DOES mention both throwaway prompts with status `rejected`

## Cleanup

```bash
rm -rf "$WORK_DIR"
rm -f /tmp/new-dark-factory
```
