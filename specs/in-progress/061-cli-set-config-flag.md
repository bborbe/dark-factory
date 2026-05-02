---
status: prompted
approved: "2026-05-02T10:37:31Z"
generating: "2026-05-02T10:40:19Z"
prompted: "2026-05-02T10:47:24Z"
branch: dark-factory/cli-set-config-flag
---

## Summary

- Add a generic `--set key=value` CLI flag to `run` and `daemon` that overrides any yaml-backed config field for one invocation.
- Supported keys mirror yaml keys: `hideGit`, `autoRelease`, `dirtyFileThreshold`, `model`, `maxContainers`. Type coercion (bool / int / string) per field, with the same validators used in yaml loading.
- Multiple `--set` flags allowed in one invocation; last value wins for the same key. Different keys compose freely.
- Remove the now-redundant `--hide-git` / `--no-hide-git` dedicated flag pair (added by spec 060). Their use case is fully covered by `--set hideGit=true|false`. BREAKING for any operator currently typing those flags; CHANGELOG provides the migration line.
- Source tracking shows `Source=arg` in the effective-config log for every field set via `--set`.
- Closes the surface-bloat trajectory: phase 3 fields (containerImage, claudeDir, verificationGate, etc.) won't need their own dedicated flags. One generic flag handles all current and future yaml-backed user-pref fields.

## Problem

Spec 060 added per-invocation CLI flags for two of four user-pref fields (`hideGit`, `model`). The remaining two (`autoRelease`, `dirtyFileThreshold`) need overrides too, and phase 3 will add 4-6 more user-pref fields. The naive "one or two dedicated flags per field" path produces an exponentially growing CLI surface — 4 fields × pair-pattern = 8 flags today, 16+ flags after phase 3. Help text becomes noise. Operators must learn two name spaces (yaml keys + CLI flag names that don't always match: yaml `hideGit` ↔ CLI `--hide-git` / `--no-hide-git`). A generic `--set key=value` collapses the surface to one flag, reuses the yaml key as the single authoritative name, and removes the need for per-field code on every new addition. The dedicated `--hide-git` / `--no-hide-git` flags introduced in spec 060 become a one-off legacy artifact unless removed alongside the generic flag's introduction.

## Goal

When the operator passes `dark-factory run --set hideGit=false --set model=claude-haiku-4-5 --set autoRelease=false`, all three fields override yaml for that invocation; the effective-config log shows `hideGitSource=arg modelSource=arg autoReleaseSource=arg`; `dark-factory run --hide-git` no longer works (returns `unknown flag`). The arg layer is the top-priority layer (default ← global ← project ← arg). Adding a new yaml-backed user-pref field to `Config` in a future phase requires only a single line addition to the supported-keys table — no new CLI flag, no new help text, no new tests beyond the per-key validation.

## Non-goals

- Removing other dedicated flags. `--max-containers N`, `--model NAME`, `--skip-preflight`, `--auto-approve`, and `-debug` all stay. `--max-containers` and `--model` predate spec 060 or are hot-path enough to keep ergonomic shortcuts (revisit later if scope creeps); the others are per-invocation only and not yaml-backed, so `--set` does not apply.
- Free-form `--set extraMounts=...` for nested / complex types. Initial scope is scalar fields only (bool, int, string). Nested struct support is deferred.
- Migration of remaining user-pref fields (`containerImage`, `claudeDir`, `verificationGate`) to layered precedence. Still phase 3.
- Env layer (`DF_<FIELD>`). Still phase 2.
- Secrets registry / auto-redaction. Still phase 4.
- Removing the `hideGit` field from `Config`. The yaml field stays — only the dedicated CLI flag pair is removed.

## Desired Behavior

1. **`--set key=value` accepted on `run` and `daemon`.** May appear multiple times in one invocation; each occurrence sets one field. Other commands (`status`, `list`, `prompt`, `spec`, etc.) reject `--set` with `unknown flag: --set`.

2. **Supported keys mirror yaml keys** exactly. Initial set: `hideGit`, `autoRelease`, `dirtyFileThreshold`, `model`, `maxContainers`. Unknown keys are rejected with `unknown config key: <name> (supported: ...)` — the error message lists the supported keys so operators self-discover.

3. **Type coercion per field, strict:**
   - `hideGit`, `autoRelease` → bool: accept `true` / `false` (lowercase only); reject `1`/`0`/`yes`/`no` and any other variant with `--set <key>=<value>: invalid bool, expected true or false`.
   - `dirtyFileThreshold` → non-negative int: accept integer literals; reject negative / non-numeric with explicit messages.
   - `maxContainers` → positive int: accept `>= 1`; reject `< 1` with the same range error already used at the yaml layer.
   - `model` → string: pass through after the same regex validation used everywhere else (`^[a-zA-Z0-9._:/-]{1,256}$`).

4. **Last value wins for the same key.** `--set hideGit=true --set hideGit=false` results in `hideGit: false` for that invocation. Logged at debug level so operators can troubleshoot.

5. **Different keys compose.** `--set autoRelease=false --set dirtyFileThreshold=5` overrides both fields independently. Order does not matter for distinct keys.

6. **Source tracking shows `arg`.** The effective-config log emits `<field>Source=arg` for every field set via `--set`. Source values for fields not set via `--set` continue to fall through to default / global / project as today.

7. **Validation happens at parse time.** Bad keys, bad types, or out-of-range values fail before any prompt executes — operator sees the error fast. Validation reuses the yaml-layer validators (regex for `model`, range for ints) so behavior is consistent across layers.

8. **`--help` for `run` and `daemon` documents the flag, supported keys, and one example per scalar type.**

9. **`--hide-git` and `--no-hide-git` are removed.** Both flags exit non-zero with `unknown flag: --hide-git` (or `--no-hide-git`) — the same error path as any unrecognized flag. Help text no longer lists them. The contradictory-flag check (`slices.Contains` for both flags being passed in spec 060) is removed since the flags themselves are gone.

## Constraints

- The `--set` flag's parsing logic lives next to the existing `extractMaxContainers` / `extractModel` (`main.go`). It scans `rawArgs` for `--set` occurrences, collects them into a `map[string]string`, removes them from filtered args. Use `strings.SplitN(value, "=", 2)` to handle values that contain `=` (e.g. `--set model=docker.io/foo:v1`).
- Coercion + validation lives in a small helper (e.g. `applySetOverrides`) that takes the map plus `cfg *config.Config` and `sources *config.FieldSources`. Each supported key maps to a closure that parses, validates, and assigns. Adding a new supported key later is a single map entry.
- The supported-keys table is authoritative — adding a new yaml field to `Config` does NOT automatically expose it via `--set`. The implementer must extend the table. This is intentional: keeps the surface explicit and validated.
- Validation reuses existing functions where possible:
  - `model` → `globalconfig.ModelRegex.MatchString` and `globalconfig.ModelPattern` for the error message
  - `dirtyFileThreshold` → existing `>= 0` range check
  - `maxContainers` → existing `>= 1` range check
  - bool fields → strict `"true"` / `"false"` (NOT `strconv.ParseBool`, which accepts `1`/`0`/`yes`/`no` — rejected to avoid yaml/CLI semantic drift)
- Errors use `errors.Errorf(ctx, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf`. Error messages name the key + the problem.
- The contradictory-flags concept (e.g. `--hide-git` + `--no-hide-git`) does NOT apply to `--set` because last-value-wins resolves any apparent contradiction. Tests confirm.
- `--set` is parsed BEFORE per-command validation (`run` / `daemon` only) — same as the existing dedicated flags.
- Removal of `--hide-git` / `--no-hide-git`:
  - All `--hide-git` / `--no-hide-git` parsing in `ParseArgs` is removed
  - The `slices.Contains` contradictory-flag check is removed
  - The `applyArgOverrides` parameter for `hideGit *bool` is removed; replace with `--set` map handling
  - All tests for `--hide-git` parsing are removed (`TestParseArgsHideGit`, related Ginkgo blocks)
  - Help text in `printRunHelp` / `printDaemonHelp` / `printHelp` no longer mentions the removed flags
  - The `hideGit` field, layered precedence, and source tracking ALL stay
- `ParseArgs` signature can grow OR refactor to a struct if it's already too long after spec 060. Implementer's call. If extending the tuple, document the new return values in the doc comment.
- The `//nolint:funlen` lesson from spec 060 prompt 358 applies: extract helpers proactively rather than re-adding `nolint` annotations.
- Tests use Ginkgo/Gomega in external `_test` packages plus stdlib `testing` for table-driven cases.
- `scenarios/013-config-layering.md` is updated: sub-scenarios that previously exercised `--hide-git` / `--no-hide-git` are rewritten to use `--set hideGit=true` / `--set hideGit=false`. Scenario coverage adds at least one bool, one int, and one string field exercised via `--set`. Scenario numbering stays at 013 (extension) or moves to 014 (new) — implementer's call.
- `docs/configuration.md` is updated: the CLI Flags section drops `--hide-git` / `--no-hide-git` and adds `--set key=value` with examples for bool, int, and string fields. The "Common Patterns" section's manual-worktree example switches to `dark-factory daemon --set hideGit=true` (or to using global config, which is unchanged).
- CHANGELOG `## Unreleased` entry calls out the BREAKING change for `--hide-git` removal with a one-line migration hint: `Replace --hide-git with --set hideGit=true; replace --no-hide-git with --set hideGit=false.`

## Failure Modes

| Trigger | Expected Behavior | Recovery |
|---|---|---|
| `--set` with no value | CLI rejects: usage error naming the flag | Pass `--set key=value` |
| `--set foo` (no `=`) | CLI rejects: `--set value must be key=value` | Use the `key=value` form |
| `--set =value` (empty key) | CLI rejects: empty key not allowed | Pass a real key |
| `--set unknownKey=value` | CLI rejects: `unknown config key: unknownKey (supported: hideGit, autoRelease, dirtyFileThreshold, model, maxContainers)` | Use a supported key |
| `--set hideGit=yes` | CLI rejects: `invalid bool for hideGit: expected true or false` | Use `true` / `false` |
| `--set dirtyFileThreshold=abc` | CLI rejects: integer parse error naming the key | Pass an integer |
| `--set dirtyFileThreshold=-1` | CLI rejects: `dirtyFileThreshold must be >= 0` | Pass non-negative int |
| `--set maxContainers=0` | CLI rejects: `maxContainers must be >= 1` | Pass `>= 1` |
| `--set model='foo;rm -rf /'` | CLI rejects: regex pattern mismatch (security: blocks shell metachars) | Pass a valid model identifier |
| `--set hideGit=true --set hideGit=false` | Last wins (`hideGit=false`); debug log notes the override sequence | None — by design |
| `--set` passed to `status` / `list` / etc. | CLI rejects: `unknown flag: --set` | Pass to `run` or `daemon` |
| `--hide-git` (removed flag) | CLI rejects: `unknown flag: --hide-git` | Use `--set hideGit=true` |
| `--no-hide-git` (removed flag) | CLI rejects: `unknown flag: --no-hide-git` | Use `--set hideGit=false` |
| Existing CI scripts use `--hide-git` | Scripts fail with the unknown-flag error; CI catches it | Update scripts; CHANGELOG documents the migration |
| Yaml `hideGit: true` continues to work | Identical to today | None needed |

## Do-Nothing Option

The dual-syntax persists. Operators see two ways to override `hideGit` and pick one (or one of each). Phase 3 user-pref fields keep growing the dedicated-flag count exponentially, OR phase 3 ships without per-invocation overrides at all (forcing yaml edits for one-off needs). Help text continues bloating. Maintenance cost stays: tests, parsing, help text, the contradictory-flag check, and a fresh CLI prompt per new field. No incident — just permanent surface bloat and a slightly inconsistent CLI.

## Acceptance Criteria

### `--set` works
- [ ] `dark-factory run --set hideGit=false` overrides yaml `hideGit: true`; effective-config log shows `hideGit=false hideGitSource=arg`
- [ ] `dark-factory daemon --set autoRelease=false` overrides yaml `autoRelease: true`; log shows `autoReleaseSource=arg`
- [ ] `dark-factory run --set dirtyFileThreshold=5` overrides yaml; log shows `dirtyFileThreshold=5 dirtyFileThresholdSource=arg`
- [ ] `dark-factory run --set model=claude-opus-4-7` overrides yaml; log shows `modelSource=arg`
- [ ] `dark-factory run --set maxContainers=2` overrides yaml; log shows `maxContainersSource=arg`
- [ ] `dark-factory run --set hideGit=false --set autoRelease=false --set model=claude-haiku-4-5` overrides all three independently
- [ ] `dark-factory run --set hideGit=true --set hideGit=false` results in `hideGit=false` (last wins)

### `--set` rejects bad input
- [ ] `dark-factory run --set unknownKey=foo` exits non-zero with "unknown config key" error listing supported keys
- [ ] `dark-factory run --set hideGit=yes` exits non-zero with bool-format error
- [ ] `dark-factory run --set dirtyFileThreshold=-1` exits non-zero with range error
- [ ] `dark-factory run --set model='foo;rm -rf /'` exits non-zero with regex error (shell metacharacters blocked)
- [ ] `dark-factory run --set foo` (no `=`) exits non-zero with format error
- [ ] `dark-factory run --set =bar` (empty key) exits non-zero
- [ ] `dark-factory status --set hideGit=true` exits non-zero with `unknown flag: --set`

### `--hide-git` removed
- [ ] `dark-factory run --hide-git` exits non-zero with `unknown flag: --hide-git`
- [ ] `dark-factory run --no-hide-git` exits non-zero with `unknown flag: --no-hide-git`
- [ ] `dark-factory daemon --hide-git` exits non-zero with `unknown flag: --hide-git`
- [ ] `grep -rn '"--hide-git"\|"--no-hide-git"' main.go pkg/` finds no production references
- [ ] All previous tests for `--hide-git` parsing are removed; no test references the removed flags
- [ ] The `slices.Contains` contradictory-flag check for `--hide-git` + `--no-hide-git` is removed

### Documentation + scenarios
- [ ] `dark-factory run --help` lists `--set key=value` with supported keys and one example per scalar type, no `--hide-git` references
- [ ] `dark-factory daemon --help` same shape, no `--hide-git` references
- [ ] `docs/configuration.md` CLI Flags section drops `--hide-git` / `--no-hide-git`, adds `--set` examples
- [ ] `docs/configuration.md` "Common Patterns" manual-worktree example uses `--set hideGit=true` (not `--hide-git`)
- [ ] `scenarios/013-config-layering.md` exercises `--set` for at least one bool, one int, and one string field; no remaining `--hide-git` / `--no-hide-git` references

### Regression + release
- [ ] Yaml `hideGit: true` / `hideGit: false` continues to work — verified by an existing scenario
- [ ] Operator with no `--set` flag and no `--hide-git` flag sees identical behavior to today
- [ ] CHANGELOG `## Unreleased` entry calls out the BREAKING change with the migration hint
- [ ] Existing scenarios 001, 006, 010, 011, 012 still pass; scenario 013 passes with the new `--set` syntax
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
autoRelease: true
hideGit: false
YAML
git init --bare "$WORK_DIR/remote.git" >/dev/null
git remote set-url origin "$WORK_DIR/remote.git"

# --set works
timeout 15s /tmp/new-dark-factory run --set hideGit=true --set autoRelease=false > out-1.log 2>&1 || true
grep -E "hideGit=true.*hideGitSource=arg" out-1.log
grep -E "autoRelease=false.*autoReleaseSource=arg" out-1.log

# Last value wins
timeout 15s /tmp/new-dark-factory run --set hideGit=true --set hideGit=false > out-2.log 2>&1 || true
grep "hideGit=false hideGitSource=arg" out-2.log

# Removed flags rejected
/tmp/new-dark-factory run --hide-git 2>&1 | grep -i "unknown flag"
/tmp/new-dark-factory run --no-hide-git 2>&1 | grep -i "unknown flag"

# Bad input rejected
timeout 5s /tmp/new-dark-factory run --set unknownKey=foo > out-3.log 2>&1
echo "exit: $?"
grep "unknown config key" out-3.log

timeout 5s /tmp/new-dark-factory run --set hideGit=yes > out-4.log 2>&1
echo "exit: $?"
grep -i "invalid bool" out-4.log

# Help text clean
/tmp/new-dark-factory run --help 2>&1 | grep -E "hide-git|no-hide-git" && echo "FAIL: help still lists removed flags" || echo "PASS"
/tmp/new-dark-factory run --help 2>&1 | grep -i "set key=value" || echo "FAIL: --set not in help"

rm -rf "$WORK_DIR"
```

All assertions must pass.

## Reference

- Spec 060 (foundation, completed): `specs/completed/060-config-layering-phase-1.md`
- Layering design: `docs/config-layering.md`
- Existing flag patterns: `main.go` `ParseArgs`, `applyArgOverrides` (~line 615), `extractMaxContainers`
- Source tracking: `pkg/config.FieldSources`
- Model regex: `pkg/globalconfig.ModelRegex` / `ModelPattern`
- Existing scenario: `scenarios/013-config-layering.md`
- Real-world precedent: helm `--set`, terraform `-var`, ansible `--extra-vars`
