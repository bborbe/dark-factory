---
status: committing
spec: [064-cli-flexible-id-matching]
summary: Added <id> format documentation to printPromptHelp and printSpecHelp in main.go, added ID Formats and Project Detection sections to docs/running.md, created scenarios/017-spec-064-flexible-id-matching.md, and added CHANGELOG Unreleased entry.
container: dark-factory-369-spec-064-docs-scenario
dark-factory-version: v0.145.1-3-g93401a1
created: "2026-05-03T13:00:00Z"
queued: "2026-05-03T12:51:47Z"
started: "2026-05-03T13:10:25Z"
branch: dark-factory/cli-flexible-id-matching
---

<summary>
- `dark-factory spec --help` mentions the four accepted `<id>` formats: padded number (063), unpadded number (63), full basename (063-foo-bar), or basename with .md extension (063-foo-bar.md)
- `dark-factory prompt --help` lists the same four formats
- `docs/running.md` documents the new walk-up behavior and the four ID formats so users discover the feature without running `--help`
- New scenario `scenarios/017-spec-064-flexible-id-matching.md` walks all four format resolutions, the ambiguity case, walk-up from a subdirectory, and the `$HOME` boundary stop against a freshly built binary
- No Go source code is changed in this prompt — only help strings in `main.go`, `docs/running.md` content, and the new scenario file
</summary>

<objective>
Document the four accepted `<id>` formats in the CLI help text (`printPromptHelp`, `printSpecHelp` in `main.go`) and write an end-to-end scenario that verifies all acceptance criteria for spec 064: four ID formats, ambiguity detection, and project-root walk-up (both from a subdirectory and from a non-project directory).
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `docs/scenario-writing.md` — full file; learn the required frontmatter, title format, Setup → Action → Expected structure with checkboxes, and sandbox conventions.
Read `scenarios/014-spec-063-config-validation.md` — for the single-check setup style.
Read `scenarios/013-config-layering.md` — for multi-step action/expected layout.

Read these files before editing:
- `main.go` — focus on `printPromptHelp()` (~line 1007) and `printSpecHelp()` (~line 1024); update the `approve <id>` and `complete <id>` lines to mention accepted formats
- `docs/scenario-writing.md` — full read required to produce a compliant scenario

Preconditions: prompts `1-spec-064-id-resolver.md` and `2-spec-064-project-detection.md` have been executed.
The spec this implements: `specs/in-progress/064-cli-flexible-id-matching.md`
</context>

<requirements>

## 0. Update `docs/running.md` — document the new behavior

Locate the CLI Reference table at ~line 180 and the section about `<spec-id>` / `<name>` arguments. Add a new subsection right before "## CLI Reference":

```markdown
## ID Formats

`dark-factory spec` and `dark-factory prompt` subcommands taking an `<id>` argument accept four equivalent formats:

| Format | Example | Notes |
|--------|---------|-------|
| Padded number | `063` | Matches the format shown in `spec list` / `prompt list` output |
| Unpadded number | `63` | Quick typing |
| Full basename | `063-bug-foo-bar` | Tab-completion friendly |
| With `.md` extension | `063-bug-foo-bar.md` | Convenient when copy-pasting from `ls` |

When two specs share a number (defensive case — the daemon assigns unique numbers, so this should never occur in practice), the CLI errors with `ambiguous spec id <input>: <list-of-paths>` and exits non-zero.

## Project Detection

When run outside a project root (no `.dark-factory.yaml` in the current directory), `dark-factory` walks up the directory tree to find one — same convention as `git`. The walk stops at `$HOME`. If no `.dark-factory.yaml` is found, the CLI exits non-zero with `not a dark-factory project: no .dark-factory.yaml in <cwd> or any parent directory`.

This means you can run `dark-factory spec list` from any subdirectory of a project — no need to `cd` to the root first.
```

Do NOT change any other part of the file.

## 1. Update `printPromptHelp()` in `main.go`

Find `printPromptHelp()` (around line 1007). It prints lines like:

```
  approve <id>    Approve a prompt (move from inbox to queue)
  ...
  cancel <id>     Cancel an approved or executing prompt
  ...
  complete <id>   Complete a prompt (triggers commit/push)
  unapprove <id>  Unapprove a prompt (move back to inbox, reset to draft)
  reject <id> --reason <text>  Reject a prompt (move to rejected/, terminal state)
  show <id>       Show details for a single prompt
```

Add a line immediately after the subcommand table explaining accepted formats. Insert it just before the closing `\n` of the format string:

```
  <id> formats: padded number (063), unpadded number (63), full basename (063-foo-bar), or basename with .md extension\n
```

The final function should produce output that when grep'd with `-E "padded|unpadded|number|basename|formats"` returns at least one match.

## 2. Update `printSpecHelp()` in `main.go`

Find `printSpecHelp()` (around line 1024). Apply the same addition — add the `<id> formats:` line at the end of the subcommand list:

```
  <id> formats: padded number (063), unpadded number (63), full basename (063-foo-bar), or basename with .md extension\n
```

## 3. Create `scenarios/017-spec-064-flexible-id-matching.md`

Create the scenario file with the following content (adapt numbering/path if the scenario writing guide requires it):

```markdown
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
```

## 4. Add CHANGELOG entry

In `CHANGELOG.md`, append to the existing `## Unreleased` section (created by prompt 2):

```markdown
- docs: add accepted `<id>` format note to `spec` and `prompt` help text (padded/unpadded number, basename, .md extension)
```

Do NOT create a duplicate `## Unreleased` section.

## 5. Run `make precommit`

```bash
cd /workspace && make precommit
```

Must exit 0 (this prompt makes no Go code changes, so no unit tests to run separately).

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT change any Go source code other than the two help-text functions in `main.go`
- Scenario file must have `status: draft` frontmatter — it is newly created and not yet verified
- Scenario file must use `/tmp/new-dark-factory` (not the bare `dark-factory` binary), per the scenario-writing guide
- Scenario file must follow the Setup → Action → Expected structure with markdown checkboxes
- The `<id> formats:` line added to `printPromptHelp` and `printSpecHelp` must contain the words "padded", "unpadded", "number", and "basename" so the verification grep finds it
- Do NOT modify any existing scenario files
- Do NOT modify any Go logic — this prompt is documentation only
- Existing tests must still pass — running `make precommit` after these doc changes must exit 0
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `/tmp/new-dark-factory spec --help 2>&1 | grep -E "padded|unpadded|number|basename|formats"` — must return at least one line (requires building binary first)
2. `/tmp/new-dark-factory prompt --help 2>&1 | grep -E "padded|unpadded|number|basename|formats"` — same
3. `ls scenarios/017-spec-064-flexible-id-matching.md` — scenario file exists
4. `grep "status: draft" scenarios/017-spec-064-flexible-id-matching.md` — correct status
5. `grep "/tmp/new-dark-factory" scenarios/017-spec-064-flexible-id-matching.md` — uses freshly built binary
6. `grep -A2 "## Unreleased" CHANGELOG.md` — shows docs: entry
</verification>
