---
status: draft
---

# Scenario 017: CLI flexible ID matching and project-root walk-up

Validates that `dark-factory spec` and `dark-factory prompt` accept four `<id>` formats, that ambiguous numeric IDs error with a list of candidates, and that running from a subdirectory or a non-project directory behaves correctly.

## Setup

```bash
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .
WORK_DIR=$(mktemp -d)
cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/project"
cd "$WORK_DIR/project"
```

- [ ] Binary built: `/tmp/new-dark-factory --version` prints a version string
- [ ] At least one approved spec exists in `specs/in-progress/`: `/tmp/new-dark-factory spec list`
- [ ] Note the spec ID used below as `SPEC_NUM` (e.g. `063`) and `SPEC_SLUG` (e.g. `063-bug-foo-bar`)

---

## Part A: Four ID formats for `spec show`

Use `spec show` (idempotent read-only operation) to test all four formats.
Set `SPEC_NUM` and `SPEC_SLUG` to a real spec from `spec list` output.

```bash
SPEC_NUM=063   # replace with an actual number from spec list
SPEC_SLUG=063-bug-autorelease-overrides-pr-workflow  # replace with actual slug (no .md)
```

### Action

```bash
/tmp/new-dark-factory spec show $SPEC_NUM
/tmp/new-dark-factory spec show ${SPEC_NUM#0*([0])}   # strip leading zeros for unpadded, e.g. 63
/tmp/new-dark-factory spec show $SPEC_SLUG
/tmp/new-dark-factory spec show ${SPEC_SLUG}.md
```

### Expected

- [ ] All four commands succeed (exit 0) and print the same spec content
- [ ] No command errors with "spec not found"

---

## Part B: Four ID formats for `prompt show`

Use `prompt show` (idempotent read-only) to test all four formats.
Set `PROMPT_NUM` and `PROMPT_SLUG` to a real prompt from `prompt list` output.

```bash
PROMPT_NUM=365   # replace with an actual number from prompt list
PROMPT_SLUG=365-spec-063-dispatch-fix  # replace with actual slug (no .md)
```

### Action

```bash
/tmp/new-dark-factory prompt show $PROMPT_NUM
/tmp/new-dark-factory prompt show ${PROMPT_NUM#0}   # unpadded (strip leading zero if present)
/tmp/new-dark-factory prompt show $PROMPT_SLUG
/tmp/new-dark-factory prompt show ${PROMPT_SLUG}.md
```

### Expected

- [ ] All four commands succeed (exit 0) and print the same prompt content
- [ ] No command errors with "prompt not found"

---

## Part C: Ambiguity detection

```bash
AMBIG_DIR=$(mktemp -d)
echo 'workflow: direct' > "$AMBIG_DIR/.dark-factory.yaml"
mkdir -p "$AMBIG_DIR/specs/in-progress"
printf -- '---\nstatus: verifying\n---\n' > "$AMBIG_DIR/specs/in-progress/001-foo.md"
printf -- '---\nstatus: verifying\n---\n' > "$AMBIG_DIR/specs/in-progress/001-bar.md"
cd "$AMBIG_DIR"
```

### Action

```bash
/tmp/new-dark-factory spec show 001
echo "exit code: $?"
```

### Expected

- [ ] Command exits non-zero
- [ ] Error message contains `"ambiguous spec id 001"`
- [ ] Error message lists both `001-foo.md` and `001-bar.md`

```bash
/tmp/new-dark-factory spec show 001 2>&1 | grep -E "ambiguous|001-foo|001-bar"
```

---

## Part D: Project-root walk-up (from subdirectory)

```bash
cd "$WORK_DIR/project/pkg"
```

### Action

```bash
/tmp/new-dark-factory spec list
```

### Expected

- [ ] Command succeeds (exit 0) and lists specs from the parent project root
- [ ] Output is identical to running `spec list` from `$WORK_DIR/project` directly

---

## Part E: Non-project directory (no .dark-factory.yaml)

```bash
mkdir -p /tmp/df-no-project-test
cd /tmp/df-no-project-test
# Confirm no .dark-factory.yaml anywhere up to $HOME:
ls ~/.dark-factory.yaml 2>/dev/null && echo "WARNING: $HOME has .dark-factory.yaml" || true
```

### Action

```bash
/tmp/new-dark-factory spec status
echo "exit code: $?"
```

### Expected

- [ ] Command exits non-zero
- [ ] Error message contains `"not a dark-factory project"`
- [ ] Error message contains `".dark-factory.yaml"`
- [ ] Output does NOT contain `"Specs: 0 total"` (silent zero eliminated)

```bash
/tmp/new-dark-factory spec status 2>&1 | grep "not a dark-factory project"
```

---

## Part E2: $HOME boundary stop (walk does not escape $HOME)

This part proves the walk-up stops at `$HOME` and does NOT recurse to `/` even if `.dark-factory.yaml` exists higher up.

```bash
# Create a sentinel project file outside $HOME (must be writable; in CI/sandbox you may need sudo or skip)
SENTINEL_DIR=$(mktemp -d /tmp/df-sentinel-XXXX)
echo 'workflow: direct' > "$SENTINEL_DIR/.dark-factory.yaml"
mkdir -p "$SENTINEL_DIR/nested/deep"
cd "$SENTINEL_DIR/nested/deep"
```

### Action

```bash
# This subdir IS inside the sentinel project, so walk-up SHOULD find it
/tmp/new-dark-factory spec status
echo "exit code (should be 0): $?"

# Now move outside the sentinel tree but still outside $HOME
mkdir -p /tmp/df-outside-home
cd /tmp/df-outside-home
/tmp/new-dark-factory spec status
echo "exit code (should be non-zero): $?"
```

### Expected

- [ ] First command (`$SENTINEL_DIR/nested/deep`) succeeds — walk-up traverses two levels and finds `.dark-factory.yaml` in the sentinel project
- [ ] Second command (`/tmp/df-outside-home`) fails with "not a dark-factory project" — walk-up does NOT escape to `/` even though `/` may have a system-wide config
- [ ] Walk-up boundary is `$HOME`, not `/` — verified by absence of any false-positive resolution above `$HOME`

---

## Part F: Help text mentions formats

```bash
cd "$WORK_DIR/project"
```

### Action

```bash
/tmp/new-dark-factory spec --help 2>&1 | grep -E "padded|unpadded|number|basename|formats"
/tmp/new-dark-factory prompt --help 2>&1 | grep -E "padded|unpadded|number|basename|formats"
```

### Expected

- [ ] Both commands produce at least one matching line
- [ ] The line mentions both "padded" and "unpadded" (or equivalent)

---

## Cleanup

```bash
rm -rf "$WORK_DIR" "$AMBIG_DIR" "$SENTINEL_DIR" /tmp/df-no-project-test /tmp/df-outside-home
```
