---
status: approved
spec: [099-correlation-ids-structured-logging]
created: "2026-06-26T05:42:49Z"
queued: "2026-06-26T06:13:15Z"
branch: dark-factory/correlation-ids-structured-logging
---

<summary>

- Flips the hot-path log check from warn mode (prints offenders, always passes) to strict mode (fails when any bare package-level log call is found in a hot-path package).
- Wires the strict check into `make precommit` so any future change that bypasses the context-bound logger fails the build.
- Adds the final CHANGELOG entry recording the completed correlation-id work.
- This is the gate prompt — it cannot land until all six hot-path packages are migrated (prompts 2-4), because strict mode would otherwise fail CI.
- After this prompt the convention is enforced permanently, not just documented.
- The end-to-end live-lifecycle observation (spec AC 7) is verified separately via `/dark-factory:verify-spec 099` on a real-environment scenario run; it is not a container-autonomous step.

</summary>

<objective>
Flip `hotpath-logcheck` to strict mode (non-zero exit on offenders), wire it into `make precommit`, and add the final CHANGELOG entry. This is the enforcement gate — it depends on prompts 2-4 having migrated all six hot-path packages. Spec AC 7 (live single-grep lifecycle verification) is deliberately deferred to `/dark-factory:verify-spec 099`, since the container agent has no Docker socket / live Claude runtime.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read these coding-plugin docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-precommit.md` — the precommit target structure

Read the parent spec end-to-end:
- `/workspace/specs/in-progress/099-correlation-ids-structured-logging.md` — Desired Behavior items 6, 8; Acceptance Criteria 4 (final), 7, 9, 10; Failure Modes rows 3, 4.

Read these source files before editing:
- `/workspace/scripts/hotpath-logcheck.sh` (created in prompt 1) — confirm it accepts a `warn`/`strict` mode argument and that `strict` exits 1 on offenders. The package allow-list and the `pkg/executor/launch.go` exclusion live here. If prompt 1 did NOT implement a `strict` branch, add it now (print offenders to stderr, exit 1 if any found).
- `/workspace/Makefile` — the `precommit` target (line 16: `precommit: ensure format generate test check addlicense check-changelog check-links`) and the `hotpath-logcheck` target (added in prompt 1, currently `bash scripts/hotpath-logcheck.sh warn`).
- `/workspace/docs/rules/logging-conventions.md` — confirm the convention doc is complete.
PRECONDITION CHECK (fail-loud): before flipping to strict, run `make hotpath-logcheck` (warn mode). If it lists ANY offender across the six hot-path packages (`pkg/processor`, `pkg/executor` [excluding `launch.go`], `pkg/promptresumer`, `pkg/committingrecoverer`, `pkg/cancellationwatcher`, `pkg/queuescanner`), then prompts 2-4 are incomplete — STOP and report `Status: failed` with message "hot-path packages not fully migrated; offenders: <list> (prompts 2-4 incomplete)". Do NOT flip to strict over an un-migrated tree.

</context>

<requirements>

## 1. Flip `hotpath-logcheck` to strict mode

1.1. In `/workspace/Makefile`, change the `hotpath-logcheck` target to invoke strict mode:
```makefile
.PHONY: hotpath-logcheck
hotpath-logcheck:
	@bash scripts/hotpath-logcheck.sh strict
```

1.2. In `/workspace/scripts/hotpath-logcheck.sh`, ensure the `strict` mode: prints each offending `file:line` to stderr AND exits 1 if ANY offender is found; exits 0 if none. The `warn` mode behavior (print + exit 0) MUST remain available for ad-hoc use. (If prompt 1 already implemented both modes, just confirm; no change needed beyond the Makefile flip.)

The strict check enforces spec Failure Mode row 3 (a new bare `slog.Info(...)` in a hot-path package fails `make hotpath-logcheck` with file:line and fails `make precommit`).

## 2. Wire `hotpath-logcheck` into `make precommit`

In `/workspace/Makefile`, add `hotpath-logcheck` to the `precommit` target's prerequisite list. Place it AFTER `test` (so compile/test failures surface first) and before `addlicense`:

```makefile
precommit: ensure format generate test hotpath-logcheck check addlicense check-changelog check-links
	@echo "ready to commit"
```

(Match the exact existing prerequisite ordering; insert `hotpath-logcheck` in a sensible slot — after `test`, before `check` is acceptable. The key requirement: a future bare hot-path `slog.X` call now fails `make precommit`.)

Verify: `make precommit` still exits 0 on the migrated tree (all six packages clean), AND a deliberately-introduced bare `slog.Info("x")` in, say, `pkg/processor/processor.go` makes `make hotpath-logcheck` exit non-zero (test this temporarily, then REVERT the deliberate change — do NOT leave it in).

## 3. Final CHANGELOG entry

Append to `## Unreleased` in `/workspace/CHANGELOG.md` ONE bullet that references the spec and the correlation-id / structured-log keywords (spec AC 10 requires `grep -nE 'correlation id|structured log' CHANGELOG.md` to return >= 1 line under `## Unreleased` — at least one bullet must contain that phrasing; ensure the wording includes "correlation id" or "structured log"):

```
- feat: enforce per-prompt correlation-id structured logging — `make hotpath-logcheck` now runs in strict mode and gates `make precommit`, rejecting bare package-level slog.Info/Warn/Error in the six hot-path packages; a single `grep prompt_id=<id> .dark-factory.log` reconstructs a prompt's full lifecycle (spec 099 prompt 5)
```

Confirm the phrase "correlation id" (or "correlation-id"/"correlation ID") appears so AC 10's grep matches. The grep is case-insensitive on `correlation id`? It is NOT (`grep -nE 'correlation id|structured log'`), so include the literal lowercase phrase `correlation id` OR `structured log` somewhere in the Unreleased section. The bullet above contains "structured logging" — that matches `structured log`. Good.

</requirements>

<constraints>

- This prompt is the GATE: it MUST NOT land before prompts 2-4 migrated all six hot-path packages. The precondition check in `<context>` enforces this (fail-loud `Status: failed` if offenders remain).
- After this prompt `make precommit` MUST exit 0 on the migrated tree (spec AC 9).
- Do NOT leave any deliberate test-offender `slog.X` call in the tree — revert it after confirming the strict check fails on it.
- The `hotpath-logcheck` package allow-list and the `pkg/executor/launch.go` exclusion stay checked into `scripts/hotpath-logcheck.sh` (spec Constraint).
- Adding a new hot-path package to the allow-list is a deliberate edit (spec Constraint) — do NOT auto-discover packages.
- The verifier uses POSIX `grep`/`awk` only, no GNU-specific flags (spec Constraint — macOS zsh + Linux bash).
- Do NOT introduce a per-package opt-out flag for the check (spec Non-goal "Does NOT add a per-package opt-out flag").
- No new dependencies (spec Constraint).
- Spec AC 7 (live single-grep lifecycle) is OUT OF SCOPE for this prompt — it requires a real Docker + Claude scenario run and lands via `/dark-factory:verify-spec 099`. Do NOT attempt it from inside the container.
- BSD-style license header on `scripts/hotpath-logcheck.sh` only if the repo's other scripts carry one (match `scripts/check-versions.sh`).
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.

</constraints>

<verification>

```bash
cd /workspace

# AC 4 (final) — strict check exits 0 on the migrated tree
make hotpath-logcheck; echo "exit=$?"
# expected: exit=0 (zero offenders)

# strict mode actually fails on a bare hot-path slog call (temporary probe, then revert)
# (run manually: add `slog.Info("probe")` to a hot-path func, run `make hotpath-logcheck`,
#  confirm exit!=0 and file:line printed, then revert.)

# hotpath-logcheck is a precommit prerequisite
grep -nE '^precommit:.*hotpath-logcheck' Makefile
# expected: 1 line

# AC 9 — full precommit green
make precommit
# expected: exit 0

# AC 10 — CHANGELOG has the correlation/structured-log bullet under Unreleased
grep -nE 'correlation id|structured log' CHANGELOG.md
# expected: >= 1 line

# Spec AC 7 (live single-grep lifecycle) is deferred to /dark-factory:verify-spec 099 —
# requires real Docker + live Claude scenario run, out of scope for this container-autonomous prompt.
```

</verification>
