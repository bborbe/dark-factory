---
status: prompted
tags:
    - dark-factory
    - spec
    - config
approved: "2026-05-23T22:16:46Z"
generating: "2026-05-23T22:18:37Z"
prompted: "2026-05-23T22:22:38Z"
branch: dark-factory/disable-auto-prompt-generation
---

## Summary

- Today: when a spec moves to `status: approved` in `specs/in-progress/`, `specWatcher.handleFileEvent` (`pkg/specwatcher/watcher.go:154`) unconditionally calls `generator.Generate(ctx, specPath)`, which runs `/dark-factory:generate-prompts-for-spec` in a fresh claude-yolo container. The same fires for every spec found on startup via `scanExistingInProgress` (line ~165+).
- This spec adds a single layered config flag `disableAutoGeneratePrompts: bool` (default `false` = current behavior) that gates both call sites. When `true`, the daemon logs a one-line skip message and does not start the generator container; the operator runs `commands/generate-prompts-for-spec.md` manually when ready.
- The flag follows the existing `hideGit` / `maxContainers` layering pattern: default ← global (`~/.dark-factory/config.yaml`) ← project (`.dark-factory.yaml`) ← CLI (`--set disableAutoGeneratePrompts=true|false`).
- Naming rationale: phrasing the flag as a *disable* keeps the Go zero-value (`false`) aligned with current behavior. No existing `dark-factory.yaml` needs to be touched — only configs that opt out add the line.

## Problem

The dark-factory maintainer (the sole operator today — single-user repo, no team users yet) routinely wants to approve a spec to lock its contents but defer the generator container start: re-read the approved spec, run `commands/generate-prompts-for-spec.md` from the host with custom args, or skip prompt generation entirely for spec-only experiments. The generator step costs a fresh claude-yolo container and a Sonnet pass; observed recurring pattern (2026-05 spec sessions): approve → generator fires → operator kills the container or discards its output because the spec needed one more refine pass.

Today the approve → generate edge has no off-switch. The workaround — keep the spec at `specs/` instead of moving it to `specs/in-progress/` — bypasses every other lifecycle hook the watcher does (status transitions, sweep, idle-log accounting). The operator needs a clean toggle that preserves the rest of the watcher loop.

## Goal

A single boolean config flag, layered globally + per-project + via `--set`, gates whether the spec watcher auto-fires the generator. When the flag is `true`, approving a spec is a no-op as far as generation is concerned; the operator invokes `/dark-factory:generate-prompts-for-spec <id>` from the host when ready. Every other watcher behavior (status transitions, scan-existing-on-startup detection, notifications) is unchanged.

## Non-goals

- **Per-spec frontmatter override** (`disableAutoGenerate: true` on individual spec files). Split out to `specs/ideas/per-spec-disable-auto-generate.md` — different decision layer, more code paths, premature until the global flag is in use.
- **Native CLI subcommand `dark-factory spec generate <id>`** replacing the auto-trigger. Split out to `specs/ideas/spec-generate-cli-subcommand.md` — breaking change, different ergonomics, host-side `commands/generate-prompts-for-spec.md` already covers manual invocation.
- **UI / telemetry surfacing "specs awaiting manual generation"**. Split out to `specs/ideas/awaiting-generation-telemetry.md` — different surface area, depends on operator demand once flag is shipped.
- **Gating `dark-factory spec approve` from triggering generation directly.** No separate code path exists today — approve writes the file, the watcher picks it up. If a future change adds a direct path, gating it is a follow-up.
- **Migration of in-flight specs when the flag is toggled.** Toggle is operator-controlled and idempotent: zero-value preserves current behavior; flipping `true` while a generator is mid-run does not abort the running container, only suppresses future starts. No in-flight data to migrate.
- **Renaming `disableAutoGeneratePrompts` to a positive form** (e.g. `enableAutoGeneratePrompts: true`). Considered and rejected: would require every existing `dark-factory.yaml` to be touched to preserve current behavior.

## Assumptions

- The existing layered-config plumbing (`partialConfig` + `Config` + merge precedence in `pkg/config/loader.go`, documented in `docs/config-layering.md`) is the canonical path for any flag that should be settable globally + per-project + via `--set`. This spec uses it unchanged.
- `pkg/specwatcher/watcher.go` is the single chokepoint for both auto-trigger paths (`handleFileEvent` and `scanExistingInProgress`); gating both inside the watcher reaches every entry into generation without touching the generator package itself.
- The `roundtrip_test.go` parity test catches `Config`-without-`partialConfig` drift at CI time; no manual parity test is needed.
- The host-side command `commands/generate-prompts-for-spec.md` already exists and works when invoked manually (used for re-generation today). This spec does not modify it.

## Desired Behavior

1. With `disableAutoGeneratePrompts` unset or `false` (default): behavior is byte-identical to today. The watcher detects an approved spec, calls `generator.Generate`, the container runs, prompts appear in `prompts/`.
2. With `disableAutoGeneratePrompts: true` in any config layer (global, project, or `--set`): the watcher detects an approved spec, logs `spec approved — auto-generation disabled, run /dark-factory:generate-prompts-for-spec <spec-path> manually` at `INFO` level, and does NOT call `generator.Generate`. No container starts. The spec file remains in `specs/in-progress/` with `status: approved`.
3. The same gating applies to `scanExistingInProgress` — daemon startup with `disableAutoGeneratePrompts: true` scans the dir, logs the skip line for each approved spec it finds, but does not enqueue any generator runs.
4. Layered precedence matches `hideGit`: default `false` ← global ← project ← `--set disableAutoGeneratePrompts=true|false`. The effective-config log line (already emitted at daemon startup) gains a `disableAutoGeneratePromptsSource=<default|global|project|cli>` annotation, matching the existing `hideGitSource` / `modelSource` pattern.
5. `--set disableAutoGeneratePrompts=<value>` accepts `true` and `false` (case-insensitive); any other value is a parse error matching the existing bool handling in `main.go`.
6. The CLI help text for `--set` lists `disableAutoGeneratePrompts` among the supported keys (alongside `hideGit`, `autoRelease`, etc.).

## Constraints

- Field name: `DisableAutoGeneratePrompts bool` in `Config`, YAML tag `disableAutoGeneratePrompts,omitempty`. `*bool` in `partialConfig` with YAML tag `disableAutoGeneratePrompts` (no `,omitempty` — needed for explicit `false` override at project layer to beat global `true`, matching `hideGit`).
- No new package; only `pkg/config/`, `pkg/specwatcher/`, `main.go`, and docs.
- The skip log line is emitted at `INFO` level so a daemon operator sees the decision in normal logs, not just debug.
- The watcher's other behaviors (loading the spec via `spec.Load`, status checks, `scanExistingInProgress` traversal logic) are unchanged. Only the `w.generator.Generate(...)` call is gated.
- No `Defaults()` change — Go zero-value `false` is the current behavior.
- Existing tests in `pkg/specwatcher/`, `pkg/config/`, and `main_internal_test.go` must continue to pass without modification.
- README's "User-level defaults" paragraph (line 153) gains `disableAutoGeneratePrompts` in the supported-keys list.
- `docs/configuration.md` gains a subsection documenting the flag, the trigger, the expected log line, and a brief note on how to manually trigger generation when the flag is `true`.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| `disableAutoGeneratePrompts: true` in project config; spec approved | Watcher logs skip line; no container starts; spec stays at `status: approved` in `specs/in-progress/` | Operator runs `/dark-factory:generate-prompts-for-spec <spec-path>` manually when ready |
| Operator misunderstands layering precedence (e.g. expects global to override project) | Effective-config log line at startup names the source per field; manual inspection resolves | Consult `docs/config-layering.md`; the precedence is default ← global ← project ← CLI, identical to `hideGit` |
| `--set disableAutoGeneratePrompts=true` on `dark-factory daemon` start | CLI layer wins for the daemon's lifetime; behavior matches "disabled" path above | Restart daemon without `--set` to revert |
| `--set disableAutoGeneratePrompts=garbage` | Parse error at startup, before daemon initialises; non-zero exit; error message names the key and lists accepted values (`true`, `false`) | Fix the value |
| `disableAutoGeneratePrompts: true` AND a generator was already mid-run when the daemon restarted | New daemon does not abort the orphan container (out of scope); the next approve event is gated correctly | Operator inspects `docker ps` for orphan generator container; same recovery as any orphan today |
| Operator sets `disableAutoGeneratePrompts: true` but `scanExistingInProgress` already enqueued approved specs in memory before the config was read | Cannot happen — config is loaded before the watcher starts; the scan path reads the same effective config | n/a |
| Operator expects per-spec opt-out (frontmatter `disableAutoGenerate: true`) | Not supported in this spec — see Non-goals; flag is global per dark-factory config | Use the global flag; per-spec is a future spec |

## Acceptance Criteria

**Rung 1 — Config field plumbing:**

- [ ] `pkg/config/config.go` `Config` struct has a new field `DisableAutoGeneratePrompts bool \`yaml:"disableAutoGeneratePrompts,omitempty"\`` — evidence: `grep -nE 'DisableAutoGeneratePrompts\s+bool' pkg/config/config.go` returns ≥1 line with the matching YAML tag.
- [ ] `pkg/config/loader.go` `partialConfig` struct has a matching `DisableAutoGeneratePrompts *bool \`yaml:"disableAutoGeneratePrompts"\`` — evidence: `grep -nE 'DisableAutoGeneratePrompts\s+\*bool' pkg/config/loader.go` returns ≥1 line.
- [ ] The loader merges the partial value into `Config` with precedence default ← global ← project — evidence: `grep -nE 'partial\.DisableAutoGeneratePrompts' pkg/config/loader.go` returns ≥1 merge branch matching the `HideGit` pattern.
- [ ] `roundtrip_test.go` parity test passes (no manual change needed; it auto-detects the new field) — evidence: `go test ./pkg/config/... -run Roundtrip -v` exits 0.
- [ ] `main.go` `--set` allowed-keys list includes `disableAutoGeneratePrompts` — evidence: `grep -n '"disableAutoGeneratePrompts"' main.go` returns ≥1 line in the allowed-keys slice.
- [ ] `main.go` `--set` switch handles the new key, parsing `true`/`false` (case-insensitive) and rejecting other values — evidence: `grep -nE 'case "disableAutoGeneratePrompts"' main.go` returns ≥1 line; a unit test in `main_internal_test.go` asserts `--set disableAutoGeneratePrompts=true` sets `Config.DisableAutoGeneratePrompts = true`, `=false` sets it to `false`, and `=garbage` returns a parse error naming the key.
- [ ] `make precommit` exits 0 — evidence: exit code 0.

**Rung 2 — Watcher gating:**

- [ ] `pkg/specwatcher/watcher.go` `handleFileEvent` short-circuits before `w.generator.Generate(...)` when the config field is `true` — evidence: `grep -nE 'DisableAutoGeneratePrompts' pkg/specwatcher/watcher.go` returns ≥1 line in the `handleFileEvent` function body; the short-circuit logs an `INFO` line containing the substring `auto-generation disabled` and the spec path.
- [ ] `scanExistingInProgress` honors the same flag — evidence: the same gate (or a shared helper called from both sites) appears in or upstream of `scanExistingInProgress`; an integration test starts the watcher with `DisableAutoGeneratePrompts: true` and a pre-existing approved spec, asserts the generator mock is called zero times.
- [ ] When the flag is `false` (default), the existing behavior is byte-identical — evidence: existing watcher tests pass unchanged (`go test ./pkg/specwatcher/...` exits 0 without test edits in the `false` branches).
- [ ] When the flag is `true`, the watcher does NOT call `generator.Generate` — evidence: a new test in `pkg/specwatcher/watcher_test.go` constructs the watcher with a `Counterfeiter` mock `SpecGenerator`, the config flag set to `true`, simulates an approved spec event, and asserts `mock.GenerateCallCount() == 0` and the expected `INFO` log line was emitted.
- [ ] `make precommit` exits 0 — evidence: exit code 0.

**Rung 3 — CLI + global layering:**

- [ ] Global `~/.dark-factory/config.yaml` with `disableAutoGeneratePrompts: true` flows through to `Config.DisableAutoGeneratePrompts = true` when the project config does not set it — evidence: a new test in `main_internal_test.go` (matching the existing `applies global hideGit when project did not set it` test) sets the global, leaves project unset, asserts the effective config has `DisableAutoGeneratePrompts: true`.
- [ ] Project config overrides global (both `true → false` and `false → true` directions) — evidence: two test cases in `main_internal_test.go` matching the existing `hideGit` precedence tests.
- [ ] `--set disableAutoGeneratePrompts=<v>` overrides both — evidence: test case asserts CLI wins over project.
- [ ] Effective-config log line at daemon startup includes a `disableAutoGeneratePromptsSource=<default|global|project|cli>` annotation — evidence: `grep -nE 'disableAutoGeneratePromptsSource' main.go` (or the effective-config printer) returns ≥1 line; a log-capture test asserts the annotation appears with the correct source value for each layer.

**Documentation:**

- [ ] `README.md` "User-level defaults" paragraph (around line 153) lists `disableAutoGeneratePrompts` alongside `hideGit`, `autoRelease`, `dirtyFileThreshold`, `maxContainers` — evidence: `grep -n 'disableAutoGeneratePrompts' README.md` returns ≥1 line in that paragraph.
- [ ] `docs/configuration.md` gains a subsection documenting the flag, the trigger condition, the expected log line, and an example of manual invocation via `/dark-factory:generate-prompts-for-spec` — evidence: `grep -niE 'disableAutoGeneratePrompts|auto-generation disabled' docs/configuration.md` returns ≥2 lines (heading + example).
- [ ] `main.go` `--set` help text (around lines 969, 991) lists `disableAutoGeneratePrompts` — evidence: `grep -n 'disableAutoGeneratePrompts' main.go` returns ≥1 line in each of the two help-text blocks.

**Scenario coverage:** None. The contract is "watcher skips generator call iff `disableAutoGeneratePrompts=true`" — observable via a Counterfeiter mock assertion. Unit tests in `pkg/specwatcher/`, `pkg/config/`, and `main_internal_test.go` cover all layered-config and watcher-gating paths without a real container. Per `docs/scenario-writing.md`, no scenario AC needed.

## Verification

```
make precommit
go test ./pkg/config/... -v
go test ./pkg/specwatcher/... -v
go test ./pkg/config/... -run Roundtrip -v
go test . -run "ApplyGlobal|AppliesProject|SetOverrides" -v
grep -nE 'DisableAutoGeneratePrompts\s+bool' pkg/config/config.go
grep -nE 'DisableAutoGeneratePrompts\s+\*bool' pkg/config/loader.go
grep -nE 'DisableAutoGeneratePrompts' pkg/specwatcher/watcher.go
grep -n 'disableAutoGeneratePrompts' main.go README.md docs/configuration.md
```

## Related

- `pkg/config/config.go:91` — `HideGit bool` field; this spec mirrors the field shape.
- `pkg/config/loader.go:123` — `HideGit *bool` in `partialConfig`; this spec mirrors the partial shape and merge branch.
- `pkg/specwatcher/watcher.go:154` — the call site this spec gates (`handleFileEvent`).
- `pkg/specwatcher/watcher.go:~165` — the second call site this spec gates (`scanExistingInProgress`).
- `main.go:672, 751, 781, 969, 991` — `--set` allowed-keys list, parse switch, and help text; this spec mirrors the `hideGit` / `maxContainers` entries.
- `README.md:153` — "User-level defaults" paragraph; this spec extends it.
- `commands/generate-prompts-for-spec.md` — host-side manual generation command; unchanged by this spec, surfaced in docs as the recovery path.

## Do-Nothing Option

Without this flag, every spec approve auto-fires a generator container. The operator has no cheap way to pause that step — the only workaround (keep the spec at `specs/` instead of moving to `specs/in-progress/`) bypasses every other watcher hook. Operators who want to review-then-generate either accept the cost of running the generator and discarding its output, or live with a half-broken workflow. The fix is one bool, layered through the existing precedence machinery — bounded, mechanical, and removes the only friction in the approve-then-review path.
