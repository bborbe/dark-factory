---
status: completed
spec: [061-cli-set-config-flag]
summary: Updated docs/configuration.md to replace --hide-git/--no-hide-git with --set key=value documentation and updated scenarios/013-config-layering.md to rewrite Scenario D, replace Scenario F, update the failure modes table, and add Scenario I for removed-flag rejection.
container: dark-factory-361-spec-061-docs-and-scenario
dark-factory-version: v0.141.1-1-g4fd8246-dirty
created: "2026-05-02T11:00:00Z"
queued: "2026-05-02T11:07:13Z"
started: "2026-05-02T11:27:10Z"
completed: "2026-05-02T11:31:47Z"
branch: dark-factory/cli-set-config-flag
---

<summary>
- `docs/configuration.md` CLI Flags section removes all `--hide-git` / `--no-hide-git` references and adds `--set key=value` documentation with examples for bool, int, and string fields
- The "Common Patterns" manual-worktree example in `docs/configuration.md` is updated to use `--set hideGit=true` instead of `--hide-git`
- `scenarios/013-config-layering.md` Scenario D (previously `--no-hide-git`) is rewritten to use `--set hideGit=false`
- Scenario F (contradictory-flags check) is removed since `--hide-git`/`--no-hide-git` no longer exist
- New `--set` scenarios cover at least one bool field (`hideGit`), one int field (`dirtyFileThreshold`), and one string field (`model`)
- A new scenario verifies that unknown keys, invalid types, and out-of-range values are rejected
- A new scenario verifies that `--set` on a non-run/daemon command exits non-zero
- All remaining scenario checks use the new `--set` syntax and the existing `--model` shortcut continues to work
</summary>

<objective>
Update the documentation and verification scenario for the config layering feature after `--hide-git` / `--no-hide-git` removal (prompt 1 of spec 061). The docs must reflect the new `--set key=value` flag; the scenario must exercise the new flag end-to-end and confirm the removed flags are truly gone.

**Precondition:** Prompt 1 (`1-spec-061-implementation.md`) has been executed successfully. The binary built from the current workspace at `/tmp/new-dark-factory` must reflect the `--set` flag and the removed `--hide-git`/`--no-hide-git` flags.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read in full before editing:
- `docs/configuration.md` — the full file; pay close attention to the `### CLI Flags` section (~line 350), the `## Common Patterns` section (~line 412), and the existing `--hide-git` / `--no-hide-git` and `--model NAME` blocks (~lines 384–410)
- `scenarios/013-config-layering.md` — the full file; understand which scenarios reference `--hide-git`/`--no-hide-git` and which are unaffected
- `docs/config-layering.md` — design reference (read for context, do NOT modify)
- `scenarios/010-preflight-baseline-gate.md` — scenario structure reference (Setup / Action / Expected / Cleanup pattern)

The spec this implements: `specs/in-progress/061-cli-set-config-flag.md` (Constraints re docs/configuration.md, Acceptance Criteria docs+scenarios block, and Scenario 013 update requirements).
</context>

<requirements>

## 1. Update `docs/configuration.md` — CLI Flags section

### 1a. Remove the `--hide-git` / `--no-hide-git` block

Find and delete the following block in the `### CLI Flags` section (around line 384):

```markdown
**`--hide-git` / `--no-hide-git`**

```bash
dark-factory run --hide-git
dark-factory run --no-hide-git
dark-factory daemon --hide-git
```

Overrides the `hideGit` setting for this invocation. `--hide-git` forces hide-git on; `--no-hide-git` forces it off. Either flag beats both global and project config.

Passing both `--hide-git` and `--no-hide-git` in the same invocation exits non-zero with a usage error.

Priority: `--hide-git`/`--no-hide-git` > project config > global config > default.
```

### 1b. Add `--set key=value` block

In the same `### CLI Flags` section, add the following block AFTER the `**`--model NAME`**` block and BEFORE the next `##` heading:

````markdown
**`--set key=value`**

```bash
dark-factory run --set hideGit=true
dark-factory run --set dirtyFileThreshold=5
dark-factory run --set model=claude-opus-4-7
dark-factory daemon --set autoRelease=false --set model=claude-haiku-4-5
```

Overrides any supported config field for this invocation. The flag may appear multiple times; if the same key appears more than once, the last occurrence wins.

Supported keys and types:

| Key | Type | Example |
|-----|------|---------|
| `hideGit` | bool (`true` or `false`) | `--set hideGit=true` |
| `autoRelease` | bool (`true` or `false`) | `--set autoRelease=false` |
| `dirtyFileThreshold` | int ≥ 0 | `--set dirtyFileThreshold=5` |
| `model` | string (must match `^[a-zA-Z0-9._:/-]{1,256}$`) | `--set model=claude-opus-4-7` |
| `maxContainers` | int ≥ 1 | `--set maxContainers=2` |

Bool fields accept only `true` or `false` (case-sensitive). Values like `1`, `0`, `yes`, `no` are rejected. Unknown keys exit non-zero with an error listing the supported keys.

Priority: `--set` arg > project config > global config > default.
````

### 1b-bis. Update the stale `--hide-git` reference at ~line 217

`docs/configuration.md` line 217 (in the Global Config section's bullet list) currently reads:

```
- **CLI flag**: `--model NAME`, `--hide-git`, `--no-hide-git` — overrides yaml for this invocation only
```

Replace with:

```
- **CLI flag**: `--model NAME`, `--max-containers N`, or `--set <key>=<value>` — overrides yaml for this invocation only
```

Verify after with `grep -n "hide-git\|no-hide-git" docs/configuration.md` — should return zero matches once 1a, 1b-bis, and 1c are all done.

### 1c. Update the "Common Patterns" manual-worktree example (~line 416)

Find the daemon command in the "Run on an existing manual worktree" pattern. It currently reads:

```bash
dark-factory daemon
```

(with the hideGit set via global config and `--no-hide-git` overriding it for a single run)

Find the paragraph and inline override examples that reference `--no-hide-git` and `--hide-git`. Specifically, find and update:

Old line (inline example under the Common Patterns section):
```
dark-factory run --no-hide-git              # see git output for one run
```
New:
```
dark-factory run --set hideGit=false        # see git output for one run
```

Also verify the surrounding paragraph text does NOT reference `--hide-git` or `--no-hide-git`. If any other references remain, update them to use `--set hideGit=true` or `--set hideGit=false` as appropriate.

After editing, run:
```bash
grep -n "hide-git\|no-hide-git" docs/configuration.md
```
This must return zero results.

## 2. Update `scenarios/013-config-layering.md`

### 2a. Rewrite Scenario D to use `--set hideGit=false`

Scenario D currently exercises `--no-hide-git`. Replace the entire Scenario D section:

Old:
```markdown
## Scenario D: CLI --no-hide-git beats global hideGit=true

```bash
timeout 15s /tmp/new-dark-factory run --no-hide-git > run-d.log 2>&1 || true
```

### Expected D

- [ ] `run-d.log` contains `hideGit=false`
- [ ] `run-d.log` contains `hideGitSource=arg`

```bash
grep -E "hideGit=false|hideGitSource=arg" run-d.log
```
```

New:
```markdown
## Scenario D: `--set hideGit=false` beats global `hideGit=true`

```bash
timeout 15s /tmp/new-dark-factory run --set hideGit=false > run-d.log 2>&1 || true
```

### Expected D

- [ ] `run-d.log` contains `hideGit=false`
- [ ] `run-d.log` contains `hideGitSource=arg`

```bash
grep -E "hideGit=false|hideGitSource=arg" run-d.log
```
```

### 2b. Replace Scenario F (contradictory flags) with a `--set` int override scenario

Scenario F previously tested `--hide-git --no-hide-git` mutual exclusion, which no longer applies. Replace the entire Scenario F section with a new test for `--set dirtyFileThreshold`:

Old:
```markdown
## Scenario F: contradictory flags rejected

```bash
timeout 5s /tmp/new-dark-factory run --hide-git --no-hide-git > run-f.log 2>&1 || true
```

### Expected F

- [ ] Command exited non-zero
- [ ] `run-f.log` contains `mutually exclusive` (or similar)

```bash
grep -i "exclusive\|contradictory\|mutually" run-f.log
```
```

New:
```markdown
## Scenario F: `--set dirtyFileThreshold` int override

```bash
timeout 15s /tmp/new-dark-factory run --set dirtyFileThreshold=10 > run-f.log 2>&1 || true
```

### Expected F

- [ ] `run-f.log` contains `dirtyFileThreshold=10`
- [ ] `run-f.log` contains `dirtyFileThresholdSource=arg`

```bash
grep -E "dirtyFileThreshold=10|dirtyFileThresholdSource=arg" run-f.log
```
```

### 2c. Update intro checklist and description

At the top of the scenario file, the description lists numbered checks. Update it to reflect the new scenarios:

Old:
```markdown
Validates the config layering feature introduced in spec 060. Checks:
1. Global config sets model; project config is silent → global wins
2. Project config explicitly sets model → project beats global
3. CLI `--model` flag → arg beats both
4. CLI `--no-hide-git` flag with global `hideGit: true` → arg beats global
5. Effective-config log shows the correct `*Source` for each scenario
```

New:
```markdown
Validates the config layering feature introduced in specs 060 and 061. Checks:
1. Global config sets model; project config is silent → global wins
2. Project config explicitly sets model → project beats global
3. CLI `--model` flag → arg beats both
4. `--set hideGit=false` with global `hideGit: true` → arg beats global
5. `--set dirtyFileThreshold=10` → int override works with correct source
6. Effective-config log shows the correct `*Source` for each scenario
```

### 2d. Add Scenario G (bad input rejection) and Scenario H (removed flags rejected)

After the current Scenario G (model shell metachar) and Scenario H (no global file), add new scenarios.

**Replace the existing Scenario F "contradictory flags" failure mode row** in the failure modes table:

Old row:
```
| Contradictory flags not rejected | `--hide-git --no-hide-git` succeeds silently |
```

New row:
```
| `--set` bad input not rejected | `--set hideGit=yes` or `--set dirtyFileThreshold=-1` succeeds without error |
```

**Add a new scenario for removed flag rejection** after Scenario H:

```markdown
## Scenario I: removed flags exit non-zero

```bash
/tmp/new-dark-factory run --hide-git > run-i.log 2>&1 || true
echo "exit: $?"
/tmp/new-dark-factory run --no-hide-git >> run-i.log 2>&1 || true
echo "exit: $?"
```

### Expected I

- [ ] Both commands exit non-zero (unknown flag error)
- [ ] `run-i.log` contains `unknown flag` (or similar)
- [ ] `run-i.log` does NOT contain `hideGitSource=arg` (the flags had no effect)

```bash
grep -i "unknown.flag\|unrecognized" run-i.log
```

Migrate: replace `--hide-git` → `--set hideGit=true` and `--no-hide-git` → `--set hideGit=false`.
```

### 2e. Update the failure modes table

Add these rows to the failure modes table in the scenario:

```markdown
| `--set` bad bool value | `--set hideGit=yes` accepted silently instead of error |
| Removed `--hide-git` accepted | `--hide-git` flag works instead of exiting with unknown-flag error |
```

### 2f. Remove all remaining `--hide-git` / `--no-hide-git` references

After all edits, verify:
```bash
grep -n "hide-git\|no-hide-git" scenarios/013-config-layering.md
```
Must return zero results.

## 3. Run `make precommit`

```bash
cd /workspace && make precommit
```

Must exit 0. No Go code is changed in this prompt.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT modify any Go source files — this prompt is documentation and scenario only
- The `## Global Config` section in `docs/configuration.md` is NOT changed — only the `### CLI Flags` section and the `## Common Patterns` section
- Scenario frontmatter must remain `status: active` only — do not add other fields
- The scenario must continue to use `/tmp/new-dark-factory` (not the installed `dark-factory` binary)
- The CHANGELOG entries were written in prompt 1 — do NOT add duplicate entries
- Scenario numbering stays at `013` (the file already has the number in its name); new sub-scenarios are appended to the same file
- After edits: `grep -n "hide-git\|no-hide-git" docs/configuration.md scenarios/013-config-layering.md` must return zero results
- Do NOT change `docs/config-layering.md` — it is a design reference document, not user-facing docs
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
1. `grep -n "hide-git\|no-hide-git" docs/configuration.md` — zero results
2. `grep -n "hide-git\|no-hide-git" scenarios/013-config-layering.md` — zero results
3. `grep -n "\-\-set" docs/configuration.md` — multiple results (the new block is present)
4. `grep -n "set key=value\|set hideGit\|set dirtyFileThreshold\|set model" docs/configuration.md` — results in CLI Flags and Common Patterns sections
5. `grep -n "Scenario D\|Scenario F\|Scenario I" scenarios/013-config-layering.md` — all three present
6. `grep -n "dirtyFileThresholdSource=arg" scenarios/013-config-layering.md` — found in Scenario F Expected section
7. `grep -n "hideGitSource=arg" scenarios/013-config-layering.md` — found in Scenario D Expected section
8. `grep -n "mutually exclusive\|contradictory" scenarios/013-config-layering.md` — zero results
</verification>
