---
status: prompted
approved: "2026-05-03T08:50:30Z"
generating: "2026-05-03T08:50:30Z"
prompted: "2026-05-03T08:59:46Z"
branch: dark-factory/spec-062
---

## Summary

- Extend the `--set key=value` CLI flag (introduced by spec 061) to accept three project-shape fields: `workflow`, `pr`, `autoMerge`.
- Per-invocation override only — yaml semantics unchanged.
- Existing config-level validation runs after override is applied: `workflow: direct` + `pr: true` is still rejected; `autoMerge: true` still requires `pr: true`.
- Reverses one paragraph of spec 061's "non-goals" — `pr`/`autoMerge` and `workflow` were previously project-only by design. This spec accepts the surface-area cost in exchange for one-shot delivery overrides (e.g. force a PR for one prompt without editing `.dark-factory.yaml`).

## Problem

Today the only way to flip a project from `workflow: direct` to `workflow: branch` + `pr: true` for a single experimental prompt is to edit `.dark-factory.yaml`, run, and revert. Same for forcing `autoMerge: true` on a PR-only project that normally requires manual review. Operators end up with throwaway commits to the config file, or copy-pasted yaml diffs in shell history. Spec 061 deliberately left these out of `--set` because they're "project-shape" — but in practice all three are clean overrides at execution time (no schema changes, no isolation-mode incompatibility beyond the existing validator). The cost of including them is small; the operator ergonomics win is real.

## Goal

After this work, operators can pass `--set workflow=…`, `--set pr=…`, and `--set autoMerge=…` to override yaml per-invocation. Existing combination validators still fire (`workflow: direct` + `pr: true` rejected; `autoMerge: true` requires `pr: true`). The effective-config log attributes overridden fields to `arg`. Adding a future project-shape key follows the same one-entry-plus-coercion pattern.

## Non-goals

- Adding `worktree: bool` (legacy field) to `--set`. The legacy field is mapped at load time and zeroed; exposing it via `--set` would re-introduce the deprecated surface.
- Adding `autoReview`, `verificationGate`, or other PR-quality flags. Those have multi-field coupling (autoReview requires pr+autoMerge+reviewers) — handle in a future spec if needed.
- Adding `defaultBranch`, `validationCommand`, `preflightCommand`, etc. These are repo-shape fields with no per-invocation use case.
- Removing `workflow` / `pr` / `autoMerge` from `.dark-factory.yaml`. The yaml stays authoritative; `--set` is per-invocation only.
- Layering these fields globally (`~/.dark-factory/config.yaml`). They stay project-only at the yaml layer; only the arg layer is added. The global config gains nothing — operators don't have a personal "always use workflow=clone" preference across all repos.

## Desired Behavior

1. **Three new keys accepted by `--set`:**
   - `workflow` → string, must be a valid `Workflow` enum value (`direct` | `branch` | `worktree` | `clone`). Legacy `pr` enum value is rejected at the arg layer (yaml-only legacy mapping).
   - `pr` → bool, strict `true` / `false`.
   - `autoMerge` → bool, strict `true` / `false`.

2. **Validation reuses existing rules.** After `--set` is applied to the merged `Config`, the standard `Config.Validate` runs (already does today for spec 061 keys). The validators that catch combination errors are unchanged:
   - `workflow: direct` + `pr: true` → rejected with the existing "incompatible" error.
   - `autoMerge: true` without `pr: true` → rejected with the existing "autoMerge requires pr: true" error.
   - `autoReview` constraints (untouched — `autoReview` is not added to `--set`).

3. **Source tracking.** Effective-config log shows `workflowSource=arg`, `prSource=arg`, `autoMergeSource=arg` for fields set via `--set`. Source values for fields not set via `--set` continue to fall through to default / global / project as today.

4. **Help text updated.** `dark-factory run --help` and `dark-factory daemon --help` list all eight supported `--set` keys (the existing five plus the three new ones) and add one example for the new types: workflow, pr, autoMerge.

5. **Last value wins** for the same key, identical to spec 061 semantics.

6. **Other commands still reject `--set`.** No change to per-command gating — `--set` works on `run` and `daemon` only.

## Constraints

- Three new keys (`workflow`, `pr`, `autoMerge`) are recognized by `--set` parsing; unknown keys remain rejected.
- `workflow` accepts the four valid enum values; the legacy `pr` enum value is rejected at the arg layer with a message pointing to `--set workflow=clone --set pr=true` (the yaml-only legacy mapping must not be re-introduced through the arg layer).
- `pr` and `autoMerge` accept strict `true`/`false` only — same bool semantics as existing `--set` keys.
- Source attribution distinguishes `arg` vs `project` vs `global` vs `default` for each new key, using the same model as existing tracked fields.
- Existing combination validators run on the post-override config, so invalid combinations reject regardless of whether the value came from yaml or `--set`.
- Coercion + validation happens at parse time (same as spec 061). Bad input fails before any prompt executes.
- Errors use `errors.Errorf(ctx, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf`.
- Tests use Ginkgo/Gomega in external `_test` packages plus stdlib `testing` for table-driven cases (matches spec 061).
- `docs/configuration.md` is updated: the `--set` table adds three rows (workflow, pr, autoMerge) with example values. The legacy-field note (`workflow: pr` mapped to `clone` + `pr: true`) is repeated near the new `--set workflow=...` example to remind operators that the legacy enum value is yaml-only.
- `docs/config-layering.md` is updated: the "Phase 1 done" section gains a note that three additional project-shape fields are accepted by `--set`, intentionally violating the original "project-only forever" rule. Brief justification: per-invocation overrides for delivery flags have a real ergonomics win.
- `scenarios/013-config-layering.md` adds at least one sub-scenario for each new key:
  - `--set workflow=branch --set pr=true` on a project configured `direct` → branch is created, PR is opened, prompt runs.
  - `--set workflow=direct --set pr=true` → CLI rejects with the existing incompatibility error.
  - `--set autoMerge=true` on a project with `pr: false` → CLI rejects with the existing dependency error.
- CHANGELOG `## Unreleased` entry: single line, non-breaking addition: `--set now accepts workflow, pr, autoMerge.`

## Failure Modes

| Trigger | Expected Behavior | Recovery |
|---|---|---|
| `--set workflow=invalid` | CLI rejects: `unknown workflow "invalid", valid values: direct, branch, worktree, clone` | Use a valid enum value |
| `--set workflow=pr` (legacy) | CLI rejects: `legacy workflow value "pr" not accepted via --set; use --set workflow=clone --set pr=true` | Use the resolved pair |
| `--set pr=yes` | CLI rejects: `--set pr: invalid bool "yes", expected true or false` | Use `true` / `false` |
| `--set autoMerge=true` without `pr: true` (yaml or `--set`) | Validator rejects: `autoMerge requires pr: true` (existing error) | Also pass `--set pr=true` |
| `--set workflow=direct --set pr=true` | Validator rejects: `workflow 'direct' is incompatible with pr: true` (existing error) | Use `branch`/`clone`/`worktree` |
| `--set workflow=branch` only, project has `pr: true` | Override applies; merged config is `workflow: branch` + `pr: true` (project) → valid | None |
| `--set pr=false` on a project with `autoMerge: true` (yaml) | Validator rejects: `autoMerge requires pr: true` | Also pass `--set autoMerge=false` |
| `--set` passed to `status` / `list` / etc. | CLI rejects: `unknown flag: --set` (unchanged) | Pass to `run` or `daemon` |

## Do-Nothing Option

Operators continue to edit `.dark-factory.yaml` for one-shot delivery overrides. The friction is mild and survivable. The cost of doing nothing is zero — no incident, no security exposure, just permanent operator papercut.

## Acceptance Criteria

### `--set` works for new keys

- [ ] `dark-factory run --set workflow=branch --set pr=true` on a project configured `workflow: direct` runs with branch+PR; effective-config log shows `workflowSource=arg prSource=arg`
- [ ] `dark-factory run --set autoMerge=true` on a project with `pr: true` succeeds; log shows `autoMergeSource=arg`
- [ ] `dark-factory run --set workflow=clone --set pr=true --set autoMerge=true` runs with clone+PR+auto-merge; log shows all three `Source=arg`
- [ ] `dark-factory run --set workflow=branch --set workflow=clone` results in `workflow: clone` (last wins)

### Validation preserved

- [ ] `dark-factory run --set workflow=direct --set pr=true` exits non-zero with the existing incompatibility error
- [ ] `dark-factory run --set autoMerge=true` on a project with `pr: false` and no `--set pr=true` exits non-zero with `autoMerge requires pr: true`
- [ ] `dark-factory run --set workflow=invalid` exits non-zero listing the four valid values
- [ ] `dark-factory run --set workflow=pr` exits non-zero pointing to `--set workflow=clone --set pr=true`
- [ ] `dark-factory run --set pr=yes` exits non-zero with bool-format error
- [ ] `dark-factory run --set autoMerge=1` exits non-zero with bool-format error

### Documentation + scenarios

- [ ] `dark-factory run --help` lists all eight `--set` keys with one example per type group; mentions the legacy `workflow: pr` rejection
- [ ] `dark-factory daemon --help` matches
- [ ] `docs/configuration.md` `--set` table adds rows for workflow, pr, autoMerge with examples
- [ ] `docs/config-layering.md` Phase 1 retrospective notes the three project-shape additions and the rationale
- [ ] `scenarios/013-config-layering.md` covers: workflow override, workflow+pr combination, validation rejection (workflow=direct+pr=true), validation rejection (autoMerge without pr)

### Regression

- [ ] All existing `--set` keys (hideGit, autoRelease, dirtyFileThreshold, model, maxContainers) behave identically to spec 061
- [ ] Yaml `workflow: …`, `pr: …`, `autoMerge: …` continue to work — no schema change
- [ ] Operators with no `--set` flag see identical behavior to today
- [ ] CHANGELOG `## Unreleased` entry added (single-line, non-breaking)
- [ ] `make precommit` exits 0

## Verification

```bash
cd ~/Documents/workspaces/dark-factory
make generate
make precommit
```

Both must exit 0.

Manual smoke test:

```bash
go build -o /tmp/new-dark-factory .

WORK_DIR=$(mktemp -d)
cp -r ~/Documents/workspaces/dark-factory-sandbox "$WORK_DIR/sandbox"
cd "$WORK_DIR/sandbox"
cat > .dark-factory.yaml <<'YAML'
workflow: direct
autoRelease: false
YAML
git init --bare "$WORK_DIR/remote.git" >/dev/null
git remote set-url origin "$WORK_DIR/remote.git" 2>/dev/null || true

# New keys accepted
timeout 5s /tmp/new-dark-factory run --set workflow=branch --set pr=true > out-1.log 2>&1 || true
grep -E "workflow=branch.*workflowSource=arg" out-1.log
grep -E "pr=true.*prSource=arg" out-1.log

# Validation still fires
/tmp/new-dark-factory run --set workflow=direct --set pr=true 2>&1 | grep -i "incompatible"
/tmp/new-dark-factory run --set autoMerge=true 2>&1 | grep -i "autoMerge requires pr"

# Legacy enum rejected at arg layer
/tmp/new-dark-factory run --set workflow=pr 2>&1 | grep -i "legacy.*workflow.*pr"

# Bad input rejected
/tmp/new-dark-factory run --set workflow=invalid 2>&1 | grep -i "unknown workflow"
/tmp/new-dark-factory run --set pr=yes 2>&1 | grep -i "invalid bool"

# Help text lists new keys
/tmp/new-dark-factory run --help 2>&1 | grep -E "workflow|pr|autoMerge" || echo "FAIL"

rm -rf "$WORK_DIR"
```

All assertions must pass.

## Reference

- Spec 060 (foundation, completed): `specs/completed/060-config-layering-phase-1.md`
- Spec 061 (--set introduction, completed): `specs/completed/061-cli-set-config-flag.md`
- Existing `--set` dispatch and supported-keys registry: `main.go` (`applyOneSetOverride`, `supportedSetKeys`)
- Workflow validation: `pkg/config/workflow.go` (`Workflow.Validate`)
- Cross-field validators: `pkg/config/config.go` (`validateWorkflowPR`, autoMerge gate)
- Layering policy: `docs/config-layering.md` (this spec amends category B partially)
- Existing scenario: `scenarios/013-config-layering.md`
