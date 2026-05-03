---
status: committing
spec: [062-cli-set-workflow-pr-automerge]
summary: Updated docs/configuration.md, docs/config-layering.md, and scenarios/013-config-layering.md to document the three new --set keys (workflow, pr, autoMerge) added in spec 062.
container: dark-factory-363-spec-062-docs
dark-factory-version: v0.143.0-5-g73d1db8
created: "2026-05-03T09:00:00Z"
queued: "2026-05-03T09:13:08Z"
started: "2026-05-03T09:23:49Z"
branch: dark-factory/spec-062
---

<summary>
- `docs/configuration.md` `--set` table gains three new rows: `workflow` (enum), `pr` (bool), `autoMerge` (bool) with example values
- The `docs/configuration.md` CLI flags section adds a note that `workflow: pr` is a yaml-only legacy value and is rejected at the `--set` layer
- `docs/configuration.md` bash code block gains two new example invocations (`--set workflow=branch --set pr=true` and `--set autoMerge=true`)
- `docs/config-layering.md` Phase 1 retrospective section is updated to note that three project-shape fields (`workflow`, `pr`, `autoMerge`) are now accepted by `--set`, along with the rationale for intentionally violating the original "project-only forever" rule
- `scenarios/013-config-layering.md` gains four new sub-scenarios covering the three new keys: workflow override, workflow+pr combination, validation rejection (workflow=direct+pr=true), and validation rejection (autoMerge without pr)
</summary>

<objective>
Update documentation and the config-layering scenario to reflect the three new `--set` keys added in spec 062. This prompt is purely documentation — no Go code changes. The code changes are in the companion prompt `1-spec-062-implementation.md`.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read in full before editing:
- `docs/configuration.md` — focus on the `--set key=value` section (~line 391–415) containing the supported-keys table
- `docs/config-layering.md` — focus on the `### Phase 1: Establish global expansion (done)` section (~line 117–135) and the `### B. Project-shape (project-only)` section (~line 41–52)
- `scenarios/013-config-layering.md` — read the full file to understand the existing scenario structure; new sub-scenarios must follow the same pattern (bash blocks + checked items under `### Expected` headings)

The spec this implements: `specs/in-progress/062-cli-set-workflow-pr-automerge.md`
</context>

<requirements>

## 1. Update `docs/configuration.md`

### 1a. Extend the `--set` supported-keys table

Locate the table under `Supported keys and types:` (~line 404). It currently has five rows. Add three rows for the new keys:

```markdown
| Key | Type | Example |
|-----|------|---------|
| `hideGit` | bool (`true` or `false`) | `--set hideGit=true` |
| `autoRelease` | bool (`true` or `false`) | `--set autoRelease=false` |
| `dirtyFileThreshold` | int ≥ 0 | `--set dirtyFileThreshold=5` |
| `model` | string (must match `^[a-zA-Z0-9._:/-]{1,256}$`) | `--set model=claude-opus-4-7` |
| `maxContainers` | int ≥ 1 | `--set maxContainers=2` |
| `workflow` | enum (`direct` \| `branch` \| `worktree` \| `clone`) | `--set workflow=branch` |
| `pr` | bool (`true` or `false`) | `--set pr=true` |
| `autoMerge` | bool (`true` or `false`) | `--set autoMerge=false` |
```

### 1b. Add legacy-value note after the table

Immediately after the paragraph "Bool fields accept only `true` or `false` (case-sensitive). Values like `1`, `0`, `yes`, `no` are rejected. Unknown keys exit non-zero with an error listing the supported keys." add:

```markdown
The `workflow: pr` legacy yaml value is **not** accepted via `--set`. Use `--set workflow=clone --set pr=true` instead (the yaml loader maps the legacy value at load time; the arg layer intentionally does not reproduce that mapping).
```

### 1c. Add workflow+pr examples to the `--set` code block

Locate the bash code block (~line 393–398) that already has `--set hideGit=true`, `--set dirtyFileThreshold=5`, etc. Add two new lines:

```bash
dark-factory run --set workflow=branch --set pr=true
dark-factory run --set autoMerge=true
```

These lines belong at the end of the existing bash code block, before the closing backtick fence.

## 2. Update `docs/config-layering.md`

### 2a. Update the `--set` supported-keys list in the Phase 1 section

Locate the `### Phase 1: Establish global expansion (done)` section (~line 117). It contains this bullet:

```
- `--set key=value` — generic per-invocation override; supported keys: `hideGit`, `autoRelease`, `dirtyFileThreshold`, `model`, `maxContainers`. Bool values must be `true` or `false` (no 1/0/yes/no).
```

Replace it with:

```
- `--set key=value` — generic per-invocation override; supported keys: `hideGit`, `autoRelease`, `dirtyFileThreshold`, `model`, `maxContainers`, `workflow`, `pr`, `autoMerge`. Bool values must be `true` or `false` (no 1/0/yes/no). Enum (`workflow`) accepts `direct`, `branch`, `worktree`, `clone`.
```

### 2b. Add project-shape retrospective note to Phase 1

After the updated `--set` bullet and before the closing of the Phase 1 section (the `There is no --hide-git flag...` line), add a new paragraph:

```markdown
**Retrospective note (spec 062):** `workflow`, `pr`, and `autoMerge` are category B (project-shape) fields and were originally declared project-only forever. Spec 062 intentionally adds them to `--set` — the ergonomics win (one-shot delivery overrides without editing `.dark-factory.yaml`) outweighs the surface-area cost. The global config layer (layer 2) does NOT gain these fields; they remain project-only at the yaml layer. Only the arg layer (layer 5) is extended.
```

### 2c. Update category B description

In the `### B. Project-shape (project-only)` section, update the note about `workflow`/`pr`:

The existing line reads:
```
- `workflow` / `pr` / `worktree` — depends on the repo's review culture
```

Update to:
```
- `workflow` / `pr` / `autoMerge` / `worktree` — depends on the repo's review culture; also available as `--set` arg overrides (per-invocation only; global config does not include them)
```

## 3. Update `scenarios/013-config-layering.md`

Append four new sub-scenarios before the `## Cleanup` section at the end of the file. The current file ends at Scenario I (verify with `grep "^## Scenario" scenarios/013-config-layering.md` — must show `A` through `I` and nothing else; if `J` already exists, abort and notify). New scenarios are J, K, L, M.

**State isolation:** Each new scenario starts with an explicit reset block that strips any prior `workflow:` / `pr:` lines from `.dark-factory.yaml` (so scenarios run independently of each other's order). The trailing-reset pattern used by older scenarios is unreliable when scenarios run in arbitrary order.

Each follows the existing pattern: `## Scenario X: description`, a reset block, a bash block, `### Expected X` heading, checked-item list, and grep verification commands.

### Scenario J: `--set workflow=branch --set pr=true` overrides project `workflow: direct`

```markdown
## Scenario J: --set workflow and pr override project workflow

Reset project config to baseline (no workflow/pr lines):

```bash
grep -v "^workflow:\|^pr:\|^autoMerge:" .dark-factory.yaml > .dark-factory.yaml.tmp && mv .dark-factory.yaml.tmp .dark-factory.yaml
```

Run with the new keys:

```bash
timeout 15s /tmp/new-dark-factory run --set workflow=branch --set pr=true > run-j.log 2>&1 || true
```

### Expected J

- [ ] `run-j.log` contains `workflow=branch`
- [ ] `run-j.log` contains `workflowSource=arg`
- [ ] `run-j.log` contains `pr=true`
- [ ] `run-j.log` contains `prSource=arg`

```bash
grep -E "workflow=branch|workflowSource=arg" run-j.log
grep -E "pr=true|prSource=arg" run-j.log
```
```

### Scenario K: `--set autoMerge=true` on project with `pr: true`

```markdown
## Scenario K: --set autoMerge=true marks source=arg

Reset to baseline, then update project config to have `pr: true` and `workflow: branch`:

```bash
grep -v "^workflow:\|^pr:\|^autoMerge:" .dark-factory.yaml > .dark-factory.yaml.tmp && mv .dark-factory.yaml.tmp .dark-factory.yaml
cat >> .dark-factory.yaml << 'YAML'
workflow: branch
pr: true
YAML
timeout 15s /tmp/new-dark-factory run --set autoMerge=true > run-k.log 2>&1 || true
```

### Expected K

- [ ] `run-k.log` contains `autoMerge=true`
- [ ] `run-k.log` contains `autoMergeSource=arg`
- [ ] `run-k.log` does NOT contain `autoMergeSource=project` (the value came from arg, not yaml)

```bash
grep -E "autoMerge=true|autoMergeSource=arg" run-k.log
```
```

### Scenario L: `--set workflow=direct --set pr=true` is rejected by the combination validator

```markdown
## Scenario L: workflow=direct + pr=true combination is rejected

Reset to baseline (validator must reject regardless of yaml, but clean state makes the test deterministic):

```bash
grep -v "^workflow:\|^pr:\|^autoMerge:" .dark-factory.yaml > .dark-factory.yaml.tmp && mv .dark-factory.yaml.tmp .dark-factory.yaml
/tmp/new-dark-factory run --set workflow=direct --set pr=true > run-l.log 2>&1 || true
echo "exit: $?"
```

### Expected L

- [ ] Command exited non-zero
- [ ] `run-l.log` contains `incompatible` (the existing workflow+pr combination error message)

```bash
grep -i "incompatible" run-l.log
```
```

### Scenario M: `--set autoMerge=true` without `pr: true` is rejected

```markdown
## Scenario M: autoMerge=true without pr=true is rejected

Reset to baseline (no `pr: true` anywhere):

```bash
grep -v "^workflow:\|^pr:\|^autoMerge:" .dark-factory.yaml > .dark-factory.yaml.tmp && mv .dark-factory.yaml.tmp .dark-factory.yaml
/tmp/new-dark-factory run --set autoMerge=true > run-m.log 2>&1 || true
echo "exit: $?"
```

### Expected M

- [ ] Command exited non-zero
- [ ] `run-m.log` contains `autoMerge requires pr: true`

```bash
grep -i "autoMerge requires pr" run-m.log
```
```

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do NOT change any Go source files — this prompt is documentation only
- Do NOT change the scenario Setup section or any existing scenario (A–I)
- New scenarios must use `/tmp/new-dark-factory` (not the bare `dark-factory` binary)
- Scenario bash blocks must use `timeout` and `|| true` consistently with existing scenarios
- Error messages in verification grep commands must match the EXACT strings produced by `Config.Validate` in `pkg/config/config.go`: `"incompatible"` (from `validateWorkflowPR`) and `"autoMerge requires pr: true"` (from the autoMerge validator)
- Existing tests must still pass — running `make precommit` after these doc changes must exit 0
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Additional spot checks:
1. `grep -n "workflow=branch\|--set pr=true\|--set autoMerge\|workflow: pr.*legacy" docs/configuration.md` — new table rows, bash examples, and legacy-rejection note all match (≥ 4 lines)
2. `grep -n "Retrospective note\|spec 062\|workflow.*pr.*autoMerge" docs/config-layering.md` — retrospective note present
3. `grep -c "^## Scenario [JKLM]:" scenarios/013-config-layering.md` — must equal 4
4. `grep -c "incompatible\|autoMerge requires" scenarios/013-config-layering.md` — must be ≥ 2 (one per rejection scenario)
</verification>
