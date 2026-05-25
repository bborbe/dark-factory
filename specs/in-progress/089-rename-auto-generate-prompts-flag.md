---
status: approved
tags:
    - dark-factory
    - spec
    - config
approved: "2026-05-25T19:41:45Z"
branch: dark-factory/rename-auto-generate-prompts-flag
---

## Summary

- Rename the existing config flag `disableAutoGeneratePrompts` to `autoGeneratePrompts` everywhere (Go struct fields, YAML tags, CLI `--set` key, log annotations, README, docs, tests).
- Invert the meaning so the positive form matches the name: `autoGeneratePrompts: true` enables auto-generation; `autoGeneratePrompts: false` (or unset) disables it.
- Flip the default: today the daemon auto-generates prompts on every approve unless the operator opts out. After this spec, the daemon does NOT auto-generate unless the operator opts in.
- No migration path: the old key `disableAutoGeneratePrompts` is removed outright. Configs and code that previously set it must be rewritten. The operator's own `~/.dark-factory/config.yaml` and any project `.dark-factory.yaml` in this repo must be audited as part of the work.
- All other config-layering precedence, watcher gating, and effective-config logging behavior from spec 088 stays in place — only the name, polarity, and default change.

## Problem

Spec 088 chose the negative phrasing (`disableAutoGeneratePrompts`) so the Go zero-value preserved the then-current behavior (auto-gen ON). After living with the flag, the operator wants the opposite default: auto-generation should be off by default and opt-in per project, because most spec approvals in practice want a manual `/dark-factory:generate-prompts-for-spec` invocation with custom args, not a fresh container fired immediately. The negative name is also harder to read at a glance — `disableAutoGeneratePrompts: false` is a double-negative that operators consistently mis-parse.

This is a single-operator repo with no external consumers of the config schema, so the cleanest fix is to rename and invert in one mechanical pass, with no compatibility shim.

## Goal

A single boolean config flag named `autoGeneratePrompts` controls whether the spec watcher auto-fires the generator container. The flag is `false` by default; the watcher does nothing on approve unless an operator-supplied layer (global, project, or `--set`) sets it to `true`. The old name `disableAutoGeneratePrompts` no longer exists in code, config schemas, documentation, tests, or help text.

## Non-goals

- Per-spec frontmatter override (`autoGenerate: true` on individual spec files) — tracked at `specs/ideas/per-spec-disable-auto-generate.md`. Do NOT add — separate concern, requires Frontmatter changes outside this rename.
- Backward-compatible acceptance of the old `disableAutoGeneratePrompts` key. Do NOT add — single-operator repo, no deprecation window needed; silently accepting the old key would mask un-migrated configs.
- Deprecation warnings on the old key. Do NOT add — same reason; removal is total.
- Any change to the layering precedence, the merge logic, or the effective-config log format other than the field/source rename.
- Any change to `commands/generate-prompts-for-spec.md` or the generator package itself.
- Renaming the source annotation field beyond the matching rename (`disableAutoGeneratePromptsSource` → `autoGeneratePromptsSource`).
- Native `dark-factory spec generate <id>` subcommand — tracked at `specs/ideas/spec-generate-cli-subcommand.md`.

## Assumptions

- The existing layered-config plumbing (`partialConfig` + `Config` + merge precedence in `pkg/config/loader.go`, documented in `docs/config-layering.md`) is unchanged by this spec; only field identities change.
- The `roundtrip_test.go` parity test in `pkg/config/` catches `Config`-vs-`partialConfig` drift automatically.
- There is no out-of-tree consumer of the dark-factory config schema. The operator confirms this is a single-operator repo with no published config API.
- The operator will, as part of the verification step, audit and update `~/.dark-factory/config.yaml` and any `.dark-factory.yaml` in this repo to add `autoGeneratePrompts: true` where today's behavior must be preserved.

## Desired Behavior

1. With `autoGeneratePrompts` unset in every layer: the watcher detects an approved spec, logs an `INFO` skip line containing the substring `auto-generation disabled` and the spec path, and does NOT call `generator.Generate`. No container starts. The spec stays at `status: approved` in `specs/in-progress/`. This is the new default behavior.
2. With `autoGeneratePrompts: true` in any config layer (global, project, or `--set`): the watcher detects an approved spec and calls `generator.Generate(ctx, specPath)`, matching the pre-rename "enabled" path byte-for-byte (same container start, same prompts emitted).
3. With `autoGeneratePrompts: false` explicitly set in a layer that overrides a `true` from a lower-precedence layer: behavior matches the unset default (skip + log line).
4. `scanExistingInProgress` on daemon startup honors the same flag with the same semantics for every approved spec it discovers.
5. Layered precedence is unchanged: default `false` ← global ← project ← `--set autoGeneratePrompts=true|false`. The effective-config log line at daemon startup emits the annotation under the new key `autoGeneratePromptsSource=<default|global|project|arg>`.
6. `--set autoGeneratePrompts=<v>` accepts `true` and `false` (case-insensitive, identical to existing bool handling); any other value returns a parse error naming the key and listing accepted values.
7. The CLI `--set` allowed-keys list and both `--set` help-text blocks list `autoGeneratePrompts` and do NOT list `disableAutoGeneratePrompts`.
8. Passing `--set disableAutoGeneratePrompts=true` (the old key) returns the same "unknown key" parse error the CLI returns for any other unknown `--set` key — there is no special case.

## Constraints

- Field name: `AutoGeneratePrompts bool` in `Config`, YAML tag `autoGeneratePrompts,omitempty`. `*bool` in `partialConfig` with YAML tag `autoGeneratePrompts` (no `,omitempty` — needed for explicit `false` override at project layer to beat global `true`, matching `hideGit`).
- Source annotation field: `AutoGeneratePrompts string` on the sources struct; emitted as `autoGeneratePromptsSource` in the effective-config log line.
- The skip log line stays at `INFO` level and keeps wording substantively equivalent to spec 088's `spec approved — auto-generation disabled, run /dark-factory:generate-prompts-for-spec <spec-path> manually` so operator muscle memory and any log-grep tooling continues to work. The substring `auto-generation disabled` MUST appear verbatim.
- No new package; only `pkg/config/`, `pkg/specwatcher/`, `pkg/factory/`, `main.go`, tests, README, and `docs/configuration.md`.
- Zero occurrences of the literal strings `disableAutoGeneratePrompts`, `DisableAutoGeneratePrompts`, or `disableAutoGeneratePromptsSource` anywhere in the repo after this work (search: `git grep -i disableautogenerateprompts` returns no matches). Exception: this spec file itself and the existing `specs/completed/088-disable-auto-prompt-generation.md` archive — both reference the old name as historical record.
- `docs/config-layering.md` requires no edits if it does not already name the flag; if it does, the rename is mechanical.
- The watcher's other behaviors (loading the spec via `spec.Load`, status checks, `scanExistingInProgress` traversal logic, notifications, idle-log accounting) are unchanged. Only the field name read at the gate and the polarity of the comparison change.
- Existing precedence/layering tests in `pkg/specwatcher/`, `pkg/config/`, and `main_internal_test.go` are renamed and inverted to match the new field; their structure stays.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Operator upgrades dark-factory without updating their `~/.dark-factory/config.yaml` (still contains `disableAutoGeneratePrompts: true`) | The unknown YAML key is ignored by the strict-or-lenient loader (same behavior as any other unknown YAML key today); effective config has `AutoGeneratePrompts: false`; daemon runs with auto-gen OFF | **Detection:** approve a spec → no container spawns → grep config for the dead key. **Recovery:** remove the dead key and, if auto-gen wanted, add `autoGeneratePrompts: true` |
| Operator upgrades dark-factory without updating their `~/.dark-factory/config.yaml` (still contains `disableAutoGeneratePrompts: false`) | Same as above — unknown key ignored, effective config has `AutoGeneratePrompts: false`; daemon runs with auto-gen OFF (behavior change from upgrade) | **Detection:** same — approve a spec → no container spawns. **Recovery:** add `autoGeneratePrompts: true` if the old auto-on behavior is wanted |
| Project `.dark-factory.yaml` still contains the old key | Same — unknown key ignored at the project layer | **Detection:** effective-config log line at daemon startup shows `autoGeneratePromptsSource=default` despite the project file having the dead key. **Recovery:** update the project file |
| Operator passes `--set disableAutoGeneratePrompts=true` after upgrade | Parse error at startup naming `disableAutoGeneratePrompts` as an unknown key; non-zero exit; error lists accepted `--set` keys (which now include `autoGeneratePrompts`, not the old name) | Switch to `--set autoGeneratePrompts=true` |
| Operator passes `--set autoGeneratePrompts=garbage` | Parse error at startup, before daemon initialises; non-zero exit; error names the key and lists accepted values (`true`, `false`) | Fix the value |
| `--set autoGeneratePrompts=true` on `dark-factory daemon` start | CLI layer wins for the daemon's lifetime; generator fires on approve | Restart without `--set` to revert to the layer-defined default |
| Operator expects symmetric grep — searches for the old name and finds zero hits | This is intended; the rename is total. The historical name survives only in `specs/completed/088-disable-auto-prompt-generation.md` | Search for `autoGeneratePrompts`; or consult the 088 completed spec for the history |
| Two specs in `specs/in-progress/` are at `status: approved` when the upgraded daemon starts and the operator forgot to set `autoGeneratePrompts: true` | `scanExistingInProgress` logs the skip line once per spec; nothing else fires | Either set `autoGeneratePrompts: true` and restart, or manually run `/dark-factory:generate-prompts-for-spec` per spec |

## Security / Abuse Cases

Not applicable. The change is a local config-field rename + default flip on a CLI tool the operator runs themselves. No new network surface, no new trust boundary, no new input parsing beyond the existing `--set` bool handling.

## Acceptance Criteria

**Rung 1 — Config field rename:**

- [ ] `pkg/config/config.go` `Config` struct has a field `AutoGeneratePrompts bool` with YAML tag `autoGeneratePrompts,omitempty` and NO field named `DisableAutoGeneratePrompts` — evidence: `grep -nE 'AutoGeneratePrompts\s+bool' pkg/config/config.go` returns ≥1 line with the matching YAML tag; `grep -n 'DisableAutoGeneratePrompts' pkg/config/config.go` returns 0 lines.
- [ ] `pkg/config/loader.go` `partialConfig` struct has `AutoGeneratePrompts *bool` with YAML tag `autoGeneratePrompts` (no `,omitempty`) and NO `DisableAutoGeneratePrompts` field — evidence: `grep -nE 'AutoGeneratePrompts\s+\*bool' pkg/config/loader.go` returns ≥1 line; `grep -n 'DisableAutoGeneratePrompts' pkg/config/loader.go` returns 0 lines.
- [ ] Loader merge branch reads `partial.AutoGeneratePrompts` — evidence: `grep -nE 'partial\.AutoGeneratePrompts' pkg/config/loader.go` returns ≥1 line; the matching `cfg.AutoGeneratePrompts = *partial.AutoGeneratePrompts` assignment exists.
- [ ] `roundtrip_test.go` parity test passes after the rename — evidence: `go test ./pkg/config/... -run Roundtrip -v` exits 0.

**Rung 2 — Watcher gate inversion:**

- [ ] `pkg/specwatcher/watcher.go` `handleFileEvent` calls `w.generator.Generate(...)` ONLY when `cfg.AutoGeneratePrompts == true`; when `false`, it emits an `INFO` log line containing the substring `auto-generation disabled` and the spec path, then returns without invoking the generator — evidence: `grep -nE 'AutoGeneratePrompts' pkg/specwatcher/watcher.go` returns ≥1 line inside `handleFileEvent`; a new/renamed test in `pkg/specwatcher/watcher_test.go` uses a Counterfeiter mock `SpecGenerator`, sets `AutoGeneratePrompts: true`, simulates an approved spec event, asserts `mock.GenerateCallCount() == 1`; a second test with `AutoGeneratePrompts: false` (zero value) asserts `mock.GenerateCallCount() == 0` and the expected `INFO` log line was captured.
- [ ] `scanExistingInProgress` honors the same gate with the same polarity — evidence: a renamed test exercises the scan path with `AutoGeneratePrompts: false` and a pre-existing approved spec, asserts `mock.GenerateCallCount() == 0`; the inverse test with `AutoGeneratePrompts: true` asserts `mock.GenerateCallCount() == 1`.
- [ ] No code path in `pkg/specwatcher/` references the old field name — evidence: `grep -n 'DisableAutoGeneratePrompts' pkg/specwatcher/` returns 0 lines.

**Rung 3 — CLI + layering:**

- [ ] `main.go` `--set` allowed-keys slice contains `"autoGeneratePrompts"` and does NOT contain `"disableAutoGeneratePrompts"` — evidence: `grep -n '"autoGeneratePrompts"' main.go` returns ≥1 line in the allowed-keys slice; `grep -n '"disableAutoGeneratePrompts"' main.go` returns 0 lines.
- [ ] `main.go` `--set` switch has `case "autoGeneratePrompts":` parsing `true`/`false` (case-insensitive) and rejecting other values — evidence: `grep -nE 'case "autoGeneratePrompts"' main.go` returns ≥1 line; `main_internal_test.go` asserts `--set autoGeneratePrompts=true` sets `Config.AutoGeneratePrompts = true`, `=false` sets it to `false`, `=garbage` returns a parse error naming the key, and `--set disableAutoGeneratePrompts=true` returns an unknown-key error.
- [ ] Global `~/.dark-factory/config.yaml` with `autoGeneratePrompts: true` flows through when project does not set it — evidence: a renamed test in `main_internal_test.go` (mirroring the existing `applies global hideGit` test) asserts the effective config has `AutoGeneratePrompts: true`.
- [ ] Project config overrides global in both directions (`true → false` and `false → true`) — evidence: two test cases in `main_internal_test.go` mirroring the existing `hideGit` precedence tests.
- [ ] `--set autoGeneratePrompts=<v>` overrides both layers — evidence: a renamed test asserts CLI wins over project.
- [ ] Effective-config log line in `pkg/factory/factory.go` emits `autoGeneratePrompts` and `autoGeneratePromptsSource` and does NOT emit the old names — evidence: `grep -nE 'autoGeneratePrompts(Source)?' pkg/factory/factory.go` returns ≥2 lines; `grep -n 'disableAutoGeneratePrompts' pkg/factory/factory.go` returns 0 lines.
- [ ] Source-annotation struct field is renamed — evidence: `grep -nE 'AutoGeneratePrompts\s+string' main.go` returns ≥1 line (the sources-struct definition); `grep -nE 's\.AutoGeneratePrompts\s*=\s*"(global|project|arg)"' main.go` returns 3 lines (one per layer).

**Documentation:**

- [ ] `README.md` line ~153 (user-level defaults paragraph) lists `autoGeneratePrompts` instead of `disableAutoGeneratePrompts`, and the surrounding text describing the spec→prompts auto/manual default is inverted to read "default: the daemon does NOT auto-generate prompts; set `autoGeneratePrompts: true` to enable" — evidence: `grep -n 'autoGeneratePrompts' README.md` returns ≥2 lines; `grep -n 'disableAutoGeneratePrompts' README.md` returns 0 lines.
- [ ] `docs/configuration.md` subsection is renamed and rewritten: heading mentions `autoGeneratePrompts`, the YAML example shows `autoGeneratePrompts: true`, the default-value table row reads `false` (disabled), the `--set` examples use the new key, and the prose describes the new default — evidence: `grep -nE 'autoGeneratePrompts' docs/configuration.md` returns ≥4 lines (heading + table row + YAML example + `--set` example); `grep -n 'disableAutoGeneratePrompts' docs/configuration.md` returns 0 lines.
- [ ] Both `main.go` `--set` help-text blocks list `autoGeneratePrompts` and not `disableAutoGeneratePrompts` — evidence: `grep -n 'autoGeneratePrompts' main.go` returns ≥1 line in each of the two help-text blocks (around lines 987 and 1009); `grep -n 'disableAutoGeneratePrompts' main.go` returns 0 lines.

**Repo-wide cleanup:**

- [ ] No reference to the old name remains anywhere in the live tree (specs/completed/088 and this spec are the only allowed exceptions) — evidence: `git grep -i 'disableautogenerateprompts' -- ':!specs/completed/088-disable-auto-prompt-generation.md' ':!specs/rename-auto-generate-prompts-flag.md' ':!specs/in-progress/*rename-auto-generate-prompts-flag*' ':!specs/completed/*rename-auto-generate-prompts-flag*'` returns 0 lines.
- [ ] `make precommit` exits 0 — evidence: exit code 0.

**Scenario coverage:** None. Same justification as spec 088 — the contract is "watcher calls generator iff `AutoGeneratePrompts == true`", observable via Counterfeiter mock assertions in `pkg/specwatcher/watcher_test.go`. Layering is covered by unit tests in `pkg/config/` and `main_internal_test.go`. No real container, no host integration, no E2E behavior change that is not directly observable via unit-level mocks. Per `docs/scenario-writing.md`, no scenario AC needed.

## Verification

```
make precommit
go test ./pkg/config/... -v
go test ./pkg/specwatcher/... -v
go test ./pkg/config/... -run Roundtrip -v
go test . -run "ApplyGlobal|AppliesProject|SetOverrides|AutoGeneratePrompts" -v

# Forward-grep: new name is present where expected
grep -nE 'AutoGeneratePrompts\s+bool' pkg/config/config.go
grep -nE 'AutoGeneratePrompts\s+\*bool' pkg/config/loader.go
grep -nE 'AutoGeneratePrompts' pkg/specwatcher/watcher.go
grep -nE 'autoGeneratePrompts(Source)?' pkg/factory/factory.go
grep -n 'autoGeneratePrompts' main.go README.md docs/configuration.md

# Reverse-grep: old name is gone from the live tree (specs/completed/088 + this spec excepted)
git grep -i 'disableautogenerateprompts' -- ':!specs/completed/088-disable-auto-prompt-generation.md' ':!specs/**/rename-auto-generate-prompts-flag*'
```

The reverse-grep MUST return 0 lines. Any other failure of forward-grep (line count below the asserted threshold) is a defect.

After code work, the operator audits and updates their own configs:

```
# Check operator global config
grep -i 'autogenerateprompts' ~/.dark-factory/config.yaml || echo 'no flag set'

# Check this repo's project config
grep -i 'autogenerateprompts' .dark-factory.yaml 2>/dev/null || echo 'no project flag'
```

If the operator wants the pre-rename behavior (auto-gen ON), they add `autoGeneratePrompts: true` to whichever layer fits their workflow. This audit is part of the work, not a separate follow-up.

## Related

- `specs/completed/088-disable-auto-prompt-generation.md` — original spec that introduced the negative-named flag; this spec inverts it. All field-shape and merge-branch patterns established there carry over with the renamed identifiers.
- `specs/ideas/per-spec-disable-auto-generate.md` — per-spec frontmatter override; out of scope here. When that idea is picked up, it will need to be re-titled / re-named consistent with the new positive flag.
- `pkg/config/config.go:119` — current `DisableAutoGeneratePrompts` field; rename target.
- `pkg/config/loader.go:43, 122, 186, 381-382` — current `partialConfig` field and merge branch; rename target.
- `pkg/specwatcher/watcher.go` — current gate at `handleFileEvent` (around line 154) and `scanExistingInProgress`; gate polarity flips.
- `pkg/factory/factory.go:151-152, 697` — effective-config log line; rename target.
- `main.go:558-559, 581, 598-599, 617-618, 692, 769, 774-775, 987, 1009` — allowed-keys, parse switch, sources struct, help text; rename + reverse-default in source-annotation defaults.
- `README.md:153, 155` — user-level defaults paragraph + spec→prompts auto/manual description; rewrite for new default.
- `docs/configuration.md:446, 461, 466, 471-472, 490-491` — subsection rename + inversion.

## Do-Nothing Option

Keep `disableAutoGeneratePrompts` with auto-gen ON by default. The operator continues to opt out per project, which is the inverse of their actual workflow (most approvals want manual generation). The double-negative naming continues to confuse on every config audit. The cost of doing nothing is low per-incident but recurring; the cost of the rename is one bounded mechanical pass with no migration risk because this is a single-operator repo. Doing nothing is acceptable but strictly worse than the rename.
