---
status: committing
spec: [060-config-layering-phase-1]
summary: Added Global Config section and CLI flag docs to docs/configuration.md, created scenarios/013-config-layering.md, and added CHANGELOG Unreleased entries for all changes.
container: dark-factory-359-spec-060-docs-and-scenario
dark-factory-version: dev
created: "2026-05-01T09:00:00Z"
queued: "2026-05-01T09:19:24Z"
started: "2026-05-01T09:50:49Z"
branch: dark-factory/config-layering-phase-1
---

<summary>
- `docs/configuration.md` gains a new "Global Config" section documenting `~/.dark-factory/config.yaml` and the 4 new layered fields (`hideGit`, `autoRelease`, `dirtyFileThreshold`, `model`) with precedence rules and examples
- The "CLI Flags" section in `docs/configuration.md` is updated to document `--hide-git`, `--no-hide-git`, and `--model NAME` with examples and safety notes
- A new scenario `scenarios/013-config-layering.md` provides a manual verification checklist for global-to-project precedence, global-to-arg override, project-winning-over-global, and the effective-config log sources
</summary>

<objective>
Document the global config layering feature (added by prompts 1 and 2 of this spec) in `docs/configuration.md` and provide a scenario that exercises the full precedence chain end-to-end.

**Precondition:** Prompts 1 and 2 (`1-spec-060-global-config-schema-and-merge.md` and `2-spec-060-cli-flags.md`) have been executed successfully. The binary at `/tmp/new-dark-factory` (built by the scenario) must reflect these changes.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before editing:
- `docs/configuration.md` — full file; understand the current "Per-Project Container Limit" section (~line 193), the "Preflight Baseline Check" section (~line 262), and the "CLI Flags" section (~line 284) to understand where to insert the new content
- `docs/config-layering.md` — design reference; cross-reference in the new section but do NOT duplicate its content inline
- `scenarios/010-preflight-baseline-gate.md` — scenario structure to model the new scenario after (Setup / Action / Expected / Failure modes / Cleanup)
- `scenarios/001-workflow-direct.md` — frontmatter convention (`status: active`, nothing else)

The spec this implements: `specs/in-progress/060-config-layering-phase-1.md` (Behaviors 7, 8 and all Acceptance Criteria checks).
</context>

<requirements>

## 1. Add "Global Config" section to `docs/configuration.md`

Insert the following section BEFORE the existing `## Per-Project Container Limit` section (the one that starts "Override the global `maxContainers` limit..."). Keep all existing sections unchanged.

```markdown
## Global Config

Machine-wide user preferences live in `~/.dark-factory/config.yaml`. This file is optional — when absent, all defaults apply and no behavior changes.

```yaml
# ~/.dark-factory/config.yaml
maxContainers: 5
model: claude-opus-4-7
hideGit: true
autoRelease: true
dirtyFileThreshold: 20
```

### Precedence

For user-pref fields, the effective value follows this chain (last writer wins):

```
default  ←  global config  ←  project config  ←  CLI flag
```

- **Default**: hardcoded in `config.Defaults()` (e.g. `model: claude-sonnet-4-6`)
- **Global**: `~/.dark-factory/config.yaml` — applies to all projects on this machine
- **Project**: `.dark-factory.yaml` in the repo root — overrides global for this project
- **CLI flag**: `--model NAME`, `--hide-git`, `--no-hide-git` — overrides yaml for this invocation only

Field absent at a layer means that layer is skipped — it never silently zeroes an upstream value.

For a full description of the 5-layer model and which fields belong at which layer, see [docs/config-layering.md](config-layering.md).

### Layered fields (phase 1)

The following fields are currently eligible for global config:

| Field | Default | Description |
|-------|---------|-------------|
| `maxContainers` | `3` | System-wide container concurrency limit |
| `model` | `claude-sonnet-4-6` | Claude model for all projects on this machine |
| `hideGit` | `false` | Hide git status from YOLO container by default |
| `autoRelease` | `false` | Auto-push commits and tag releases |
| `dirtyFileThreshold` | `0` (disabled) | Skip prompts when dirty file count exceeds this |

All values in `~/.dark-factory/config.yaml` are optional. Set only the ones you want to override.

### Validation

Global config is validated at startup. Invalid values fail startup with a clear error:

```
error: globalconfig: validate: globalconfig: dirtyFileThreshold must not be negative, got -1
```

The global config file itself is not validated before that — errors in the file (invalid YAML, unknown fields ignored by yaml.v3) surface at startup.

Model values must match `^[a-zA-Z0-9._:/-]{1,256}$`. Shell metacharacters (spaces, semicolons, pipes, dollar signs, etc.) are rejected because the model name flows to container args.

### Source tracing

The `effective config` log line emitted at startup includes a `*Source` field for each layered field:

```
msg="effective config" model=claude-opus-4-7 modelSource=global hideGit=true hideGitSource=global ...
```

Possible values: `default`, `global`, `project`, `arg`.
```

## 2. Update the "CLI Flags" section in `docs/configuration.md`

Find the existing `### CLI Flags` section (around line 284). It currently documents `--max-containers` and `--skip-preflight`. Append the following blocks AFTER the `--skip-preflight` block, before the next `##` heading:

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

**`--model NAME`**

```bash
dark-factory run --model claude-haiku-4-5
dark-factory daemon --model claude-opus-4-7
dark-factory run --model docker.io/bborbe/claude-yolo:v0.6.1
```

Overrides the model for this invocation. Beats both global and project config.

`NAME` must match `^[a-zA-Z0-9._:/-]{1,256}$`. Values with spaces, semicolons, pipes, or other shell metacharacters are rejected.

Priority: `--model` arg > project config > global config > default.
```

## 3. Create `scenarios/013-config-layering.md`

Create this file. Model the structure on `scenarios/010-preflight-baseline-gate.md` (Setup / Action blocks / Expected / Failure modes / Cleanup):

```markdown
---
status: active
---

# Config layering: global → project → CLI precedence

Validates the config layering feature introduced in spec 060. Checks:
1. Global config sets model; project config is silent → global wins
2. Project config explicitly sets model → project beats global
3. CLI `--model` flag → arg beats both
4. CLI `--no-hide-git` flag with global `hideGit: true` → arg beats global
5. Effective-config log shows the correct `*Source` for each scenario

Test repo: copy of `~/Documents/workspaces/dark-factory-sandbox`

## Setup

```bash
go build -C ~/Documents/workspaces/dark-factory -o /tmp/new-dark-factory .
WORK_DIR=$(mktemp -d)
cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/dark-factory-sandbox"
cd "$WORK_DIR/dark-factory-sandbox"

# Baseline project config (no model, no hideGit set explicitly)
cat > .dark-factory.yaml << 'YAML'
pr: false
worktree: false
maxContainers: 999
YAML

git init --bare "$WORK_DIR/remote.git"
git remote set-url origin "$WORK_DIR/remote.git"

# Global config: set model and hideGit
mkdir -p ~/.dark-factory
cat > ~/.dark-factory/config.yaml << 'YAML'
model: claude-opus-4-7
hideGit: true
YAML
```

- [ ] `~/.dark-factory/config.yaml` exists with `model: claude-opus-4-7` and `hideGit: true`
- [ ] `.dark-factory.yaml` does NOT set `model` or `hideGit`
- [ ] `/tmp/new-dark-factory` binary is freshly built

## Scenario A: global model applies when project is silent

Run `dark-factory run` (no prompts — it will exit immediately with nothing to do; we only care about the startup log):

```bash
timeout 15s /tmp/new-dark-factory run > run-a.log 2>&1 || true
```

### Expected A

- [ ] `run-a.log` contains `model=claude-opus-4-7`
- [ ] `run-a.log` contains `modelSource=global`
- [ ] `run-a.log` contains `hideGit=true`
- [ ] `run-a.log` contains `hideGitSource=global`

```bash
grep -E "model=claude-opus-4-7|modelSource=global" run-a.log
grep -E "hideGit=true|hideGitSource=global" run-a.log
```

## Scenario B: project model beats global model

```bash
cat >> .dark-factory.yaml << 'YAML'
model: claude-sonnet-4-6
YAML
timeout 15s /tmp/new-dark-factory run > run-b.log 2>&1 || true
```

### Expected B

- [ ] `run-b.log` contains `model=claude-sonnet-4-6`
- [ ] `run-b.log` contains `modelSource=project`
- [ ] `run-b.log` contains `hideGit=true` (not overridden by project)
- [ ] `run-b.log` contains `hideGitSource=global`

```bash
grep -E "model=claude-sonnet-4-6|modelSource=project" run-b.log
grep -E "hideGitSource=global" run-b.log
```

Reset project model:
```bash
# Remove the model line from .dark-factory.yaml
grep -v "^model:" .dark-factory.yaml > .dark-factory.yaml.tmp && mv .dark-factory.yaml.tmp .dark-factory.yaml
```

## Scenario C: CLI --model flag beats both

```bash
timeout 15s /tmp/new-dark-factory run --model claude-haiku-4-5 > run-c.log 2>&1 || true
```

### Expected C

- [ ] `run-c.log` contains `model=claude-haiku-4-5`
- [ ] `run-c.log` contains `modelSource=arg`

```bash
grep -E "model=claude-haiku-4-5|modelSource=arg" run-c.log
```

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

## Scenario E: invalid global config fails startup

```bash
cat > ~/.dark-factory/config.yaml << 'YAML'
dirtyFileThreshold: -5
YAML
timeout 10s /tmp/new-dark-factory run > run-e.log 2>&1 || true
```

### Expected E

- [ ] Command exited non-zero
- [ ] `run-e.log` contains `dirtyFileThreshold` in the error message
- [ ] `run-e.log` contains `globalconfig` in the error message (names the file's context)

```bash
grep -i "dirtyFileThreshold" run-e.log
grep -i "globalconfig" run-e.log
```

Restore valid global config:
```bash
cat > ~/.dark-factory/config.yaml << 'YAML'
model: claude-opus-4-7
hideGit: true
YAML
```

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

## Scenario G: model with shell metachar rejected

```bash
timeout 5s /tmp/new-dark-factory run --model 'claude;rm -rf /' > run-g.log 2>&1 || true
```

### Expected G

- [ ] Command exited non-zero
- [ ] `run-g.log` contains `invalid characters` or similar validation error

```bash
grep -i "invalid" run-g.log
```

## Scenario H: no global config file → defaults apply (no behavior change)

```bash
rm ~/.dark-factory/config.yaml
timeout 15s /tmp/new-dark-factory run > run-h.log 2>&1 || true
```

### Expected H

- [ ] Command did NOT exit with a config error
- [ ] `run-h.log` contains `model=claude-sonnet-4-6` (the default)
- [ ] `run-h.log` contains `modelSource=default`

```bash
grep -E "model=claude-sonnet-4-6|modelSource=default" run-h.log
```

## Failure modes this catches

| Failure | Symptom |
|---------|---------|
| Global config not loaded | `modelSource=default` even when `~/.dark-factory/config.yaml` sets `model` |
| Project override not detected | Project sets `model: claude-sonnet-4-6` but `modelSource=global` appears in log |
| Arg override not applied | `--model claude-haiku-4-5` but log shows different model |
| Invalid global config not rejected | `dirtyFileThreshold: -5` in global file but startup succeeds |
| Contradictory flags not rejected | `--hide-git --no-hide-git` succeeds silently |
| Shell metachar in --model not rejected | `--model 'claude;rm -rf /'` succeeds without validation error |
| Missing global file crashes daemon | Deleting `~/.dark-factory/config.yaml` causes a startup error instead of silently using defaults |

## Cleanup

```bash
rm -f ~/.dark-factory/config.yaml
rm -rf "$WORK_DIR"
```
```

## 4. Run `make precommit`

```bash
cd /workspace && make precommit
```

Must exit 0. No Go code is changed in this prompt; precommit validates markdown and linting only.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT modify Go source files — this prompt is documentation and scenario only
- The new "Global Config" section must be inserted BEFORE the existing `## Per-Project Container Limit` section — do NOT reorder existing sections
- The new CLI flag documentation must be appended INSIDE the existing `### CLI Flags` section — do NOT create a new `###` heading
- Scenario frontmatter must follow the pattern of existing scenarios: only `status: active`, no other fields
- The scenario must use `/tmp/new-dark-factory` (not the installed `dark-factory` binary)
- The CHANGELOG entries were written in prompts 1 and 2 — do NOT add duplicate entries
- Do not add scenario numbers to the filename (`scenarios/013-...` already has the number)
- Markdown code blocks inside the scenario that contain yaml must use YAML fencing (` ```yaml `) — but the outer scenario file already uses backtick fences consistently
- The global config file cleanup in the scenario MUST remove `~/.dark-factory/config.yaml` to leave the operator's machine in a clean state
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:
1. `grep -n "Global Config\|global config\|globalconfig" docs/configuration.md` — should find the new section heading and references
2. `grep -n "hide-git\|no-hide-git\|--model" docs/configuration.md` — should find the new CLI flag documentation in the CLI Flags section
3. `grep -n "config-layering" docs/configuration.md` — should find the cross-reference to `docs/config-layering.md`
4. `ls scenarios/013-*.md` — file exists
5. `grep -n "modelSource\|hideGitSource\|autoReleaseSource" scenarios/013-config-layering.md` — should find multiple references in the Expected sections
6. `grep -n "Cleanup" scenarios/013-config-layering.md` — should find the cleanup section that removes `~/.dark-factory/config.yaml`
7. `grep -n "mutually exclusive" scenarios/013-config-layering.md` — should find the contradictory-flags check
</verification>
