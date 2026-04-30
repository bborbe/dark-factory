---
status: approved
spec: [059-skip-preflight-cli-flag]
created: "2026-04-30T19:30:00Z"
queued: "2026-04-30T19:31:53Z"
branch: dark-factory/skip-preflight-cli-flag
---

<summary>
- `docs/configuration.md` gains a CLI Flags section under `run` and `daemon` documenting `--skip-preflight` with usage guidance and safety implications
- The existing "Preflight Baseline Check" section in `docs/configuration.md` references the skip flag so operators reading about preflight learn about the escape hatch
- A new scenario file `scenarios/012-skip-preflight-flag.md` provides a manual checklist to verify `--skip-preflight` bypasses a configured failing preflight command
- The scenario asserts the prompt executes normally despite a failing `preflightCommand`, the startup log records the skip, and no baseline-failure report is emitted
</summary>

<objective>
Document the `--skip-preflight` flag added in prompt 1 of this spec (1-spec-059-flag-and-factory.md) and provide a scenario that exercises it end-to-end against a project whose `preflightCommand` is guaranteed to fail.

**Precondition:** Prompt 1 (`1-spec-059-flag-and-factory.md`) has been executed successfully. The `--skip-preflight` flag exists in the binary.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before editing:
- `docs/configuration.md` — find the "Preflight Baseline Check" section (~line 262) and the "CLI Override" section (~line 282) to understand existing conventions; the new flag docs belong in both locations
- `scenarios/010-preflight-baseline-gate.md` — existing preflight scenario to model the new one after; note structure: Setup / Action / Expected / Failure modes / Cleanup sections
- `scenarios/001-workflow-direct.md` — read the first few lines for scenario frontmatter conventions

The spec this implements: `specs/in-progress/059-skip-preflight-cli-flag.md` (acceptance criteria live there).
</context>

<requirements>

## 1. Update `docs/configuration.md` — reference skip flag in the Preflight section

Find the "Preflight Baseline Check" section. At the end of that section (after the `**On failure:**` paragraph), add a one-sentence cross-reference:

```markdown
**Override:** Pass `--skip-preflight` to `run` or `daemon` to bypass preflight for a single invocation — see [CLI Flags](#cli-flags) below.
```

## 2. Update `docs/configuration.md` — replace `### CLI Override` with `### CLI Flags`

Find the existing `### CLI Override` subsection (~line 282) that documents `--max-containers`. **Delete that entire subsection** (heading + body — everything from `### CLI Override` up to but not including the next `###` or `##` heading). Then **insert the block below** at the same location.

The new section preserves all `--max-containers` content from the deleted section and appends the `--skip-preflight` block:

```markdown
### CLI Flags

Override settings for a single run without editing config:

**`--max-containers N`**

```bash
dark-factory run --max-containers 5
dark-factory daemon --max-containers 1
```

Priority: CLI arg > project config > global config > default (3).

**`--skip-preflight`**

```bash
dark-factory run --skip-preflight
dark-factory daemon --skip-preflight
```

Bypasses the preflight baseline check for this invocation. When set:

- The configured `preflightCommand` is not executed.
- No preflight cache is read or written.
- No baseline-failure report is emitted.
- Prompts proceed directly to normal execution.
- A startup log line records that preflight was skipped.

The flag is position-agnostic: `dark-factory --skip-preflight run` and `dark-factory run --skip-preflight` are equivalent.

**Safety note:** Prompts may run on a broken baseline when this flag is used. The startup log line provides an audit trail. Use only when the baseline is knowingly broken (e.g., transient CVE, upstream flake) and the prompt must execute urgently.

The flag does not persist: the next invocation without the flag runs preflight as configured. It has no effect when `preflightCommand` is empty (already disabled).
```

Verify after editing: `grep -c "^### CLI " docs/configuration.md` should return `1` (only `CLI Flags`, no leftover `CLI Override`).

## 3. Create `scenarios/012-skip-preflight-flag.md`

Read `scenarios/010-preflight-baseline-gate.md` for structure reference. Write the new scenario file:

```markdown
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
- [ ] `run-skip.log` does NOT contain `BASELINE_BROKEN_MARKER`
- [ ] `run-skip.log` does NOT contain `preflight: baseline check FAILED`
- [ ] `run-skip.log` does NOT contain `preflight: running baseline check`

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
```

## 4. Run `make precommit`

```bash
cd /workspace && make precommit
```

Must exit 0. No Go code is changed in this prompt; precommit validates that the markdown files are well-formed and no linting regressions exist.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT modify Go source files — this prompt is documentation and scenarios only
- The `### CLI Override` section in `docs/configuration.md` must be replaced (not duplicated) by `### CLI Flags`; keep all existing `--max-containers` content
- Scenario frontmatter must follow the pattern of existing scenarios: only `status: active`, no other fields
- The scenario must use `/tmp/new-dark-factory` (not the installed `dark-factory` binary) — per CLAUDE.md scenario conventions
- Do not add scenario numbers to the filename (`scenarios/012-...` already has the number)
- The CHANGELOG entry was written in prompt 1 — do not add a duplicate entry
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
1. `grep -n "skip-preflight\|skip_preflight" docs/configuration.md` — should find multiple entries: in the Preflight section cross-reference and in the new CLI Flags section
2. `grep -n "CLI Flags\|CLI Override" docs/configuration.md` — should find `CLI Flags`, must NOT find `CLI Override`
2b. `grep -c "^### CLI " docs/configuration.md` — must return `1`
3. `ls scenarios/012-*.md` — file exists
4. `grep -n "skip-preflight-canary\|BASELINE_BROKEN_MARKER" scenarios/012-skip-preflight-flag.md` — both present in the scenario
</verification>
