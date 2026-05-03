---
status: draft
---

# Scenario 014: invalid autoRelease+PR+autoMerge combo is rejected at startup

Validates that starting dark-factory with `pr: true + autoMerge: false + autoRelease: true` exits non-zero before processing any prompt.

Test repo: copy of `~/Documents/workspaces/dark-factory-sandbox`

## Setup

```bash
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .
WORK_DIR=$(mktemp -d)
cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/dark-factory-sandbox"
cd "$WORK_DIR/dark-factory-sandbox"
cat > .dark-factory.yaml << 'YAML'
workflow: branch
pr: true
autoMerge: false
autoRelease: true
YAML
```

- [ ] `.dark-factory.yaml` contains `pr: true`, `autoMerge: false`, `autoRelease: true`
- [ ] No prompts approved in inbox (the rejection must fire before any prompt is processed)

## Action

```bash
/tmp/new-dark-factory daemon > daemon.log 2>&1
echo "exit code: $?"
```

## Expected

- [ ] Command exits non-zero immediately
- [ ] `daemon.log` contains `"autoRelease: true with pr: true and autoMerge: false is invalid"`
- [ ] `daemon.log` contains `"autoMerge: true"` (first resolution)
- [ ] `daemon.log` contains `"autoRelease: false"` (second resolution)
- [ ] `daemon.log` contains `"pr: false"` (third resolution)
- [ ] No prompt is executed (daemon exits before processing queue)

```bash
grep "autoRelease.*invalid\|autoMerge: true\|autoRelease: false\|pr: false" daemon.log
```

## Cleanup

```bash
rm -rf "$WORK_DIR"
```
