---
status: verifying
tags:
    - dark-factory
    - spec
approved: "2026-05-25T20:01:28Z"
verifying: "2026-05-25T20:35:30Z"
branch: dark-factory/cli-spec-mark-prompted
---

## Summary

- The manual prompt-generation command writes prompt files but leaves the source spec stuck at `status: approved` forever, breaking the lifecycle state machine for operators who use it.
- After spec 089 lands and auto-generation is OFF by default, the manual command becomes the primary path — making this gap operationally load-bearing.
- Add a CLI subcommand `dark-factory spec mark-prompted <id>` that performs the same status transition the auto path already performs internally, using the same code path.
- Wire that subcommand into `commands/generate-prompts-for-spec.md` as a final step (gated on prompt-creator success), so the manual flow ends in the same observable state as the auto flow.
- Update `docs/running.md` so the manual-vs-auto comparison no longer falsely implies the manual path lacks status transitions.

## Problem

The auto-generation path (`pkg/generator/generator.go`'s `executeAndFinalize`) transitions an approved spec through `approved → generating → prompted` as part of generating prompts. The manual command `/dark-factory:generate-prompts-for-spec` invokes the prompt-creator agent directly, which writes prompt files but never touches the spec's status frontmatter. The spec stays at `status: approved` indefinitely after prompts are generated, approved, executed, and merged. This breaks `dark-factory status`, the `IsActive()` filter, lifecycle reporting, and any downstream automation that keys off spec status. Operators currently have no CLI path to repair this, and the project rule "never manually edit frontmatter status — use CLI" leaves them with no compliant option. After spec 089 flips `autoGeneratePrompts` to OFF by default, the manual command becomes the primary prompt-generation flow and the gap becomes the default experience.

## Goal

After this spec ships, every spec for which prompts have been successfully generated — whether via the auto path or the manual command — has been transitioned out of `approved` into `prompted` (or beyond), via the existing `SetStatus` API, with no operator hand-editing of frontmatter.

## Non-goals

- Do NOT add a `--force` flag to bypass state-machine validation — illegal transitions must be rejected.
- Do NOT add a new transition edge `approved → prompted` to the state machine; the existing edges (`approved → generating → prompted`) plus the manual command's allowance to start from either `approved` or `generating` cover the needed cases without a state-machine change.
- Do NOT make the manual command transactional (rolling back written prompt files if the status transition fails is out of scope).
- Do NOT replace the prompt-creator agent invocation with a host-side process.
- Do NOT modify the auto path (`pkg/generator/`) — the auto path already works.
- Do NOT add an opt-out flag for the new mark-prompted step in the manual command — the whole point is to close the gap; an opt-out reintroduces the bug.

## Desired Behavior

1. A new CLI subcommand `dark-factory spec mark-prompted <id-or-name>` exists and is reachable from `dark-factory spec` help output.
2. The subcommand resolves the spec by id-or-name using the same resolution behavior used by `dark-factory spec approve` and `dark-factory spec complete` (flexible matching introduced in spec 064).
3. When the resolved spec is in `status: approved` or `status: generating`, the subcommand transitions it to `status: prompted` via the existing `SetStatus` API (so the written frontmatter — status field, timestamp field, ordering — is byte-identical to what the auto path produces when it finalizes a generated spec).
4. When the resolved spec is already in `status: prompted`, the subcommand exits 0 and prints a single-line message stating the spec is already prompted (no frontmatter write, idempotent).
5. When the resolved spec is in any other status (e.g. `idea`, `draft`, `verifying`, `completed`, `rejected`, `hold`), the subcommand exits non-zero with an error message naming the current status and the rule that was violated.
6. When the spec id-or-name cannot be resolved to exactly one spec, the subcommand exits non-zero with an error message.
7. The manual command `commands/generate-prompts-for-spec.md` invokes `dark-factory spec mark-prompted` on the input spec as its final step, only after the prompt-creator agent has reported success and produced at least one prompt file. The success gate is the existing per-prompt audit loop in `commands/generate-prompts-for-spec.md` (the step that runs `prompt-auditor` against each created file); mark-prompted runs after that loop completes without error. If the prompt-creator wrote zero files or reported failure, the mark-prompted step is skipped (matching the auto path's `handleNoNewFiles` behavior of not marking prompted).
8. `docs/running.md` § "Two ways to generate prompts from an approved spec" describes the manual path as performing the same `approved → prompted` lifecycle transition as the auto path.

## Constraints

- The state machine in `pkg/spec/spec.go` (`specTransitions` map) MUST NOT change. The valid edges into `prompted` remain `generating → prompted`. The manual command's acceptance of an `approved`-status spec as input is handled by the new subcommand performing the same two-step transition (`approved → generating → prompted`) the auto path performs, NOT by adding a new edge.
- The new subcommand MUST use the existing `spec.File.SetStatus` API and `spec.File.Save` — no parallel frontmatter-writing code.
- The frontmatter written by the new subcommand for a fresh `approved → prompted` transition MUST be byte-identical to what the auto path's `finalizePrompted` writes for the same starting state (same status field, same timestamp fields including any intermediate `generating` timestamp).
- The new subcommand MUST appear in `printHelp()` in `main.go` alongside the existing `spec approve` / `spec complete` lines.
- The CLI test pattern MUST mirror the existing spec-subcommand tests (same file or same naming scheme as `spec approve` / `spec complete` tests).
- Existing CLI behaviors (`spec approve`, `spec complete`, `spec reject`, `spec unapprove`, `spec show`, `spec list`, `spec status`) MUST continue to work unchanged.
- Auto-path behavior (`pkg/generator/generator.go`) MUST NOT change — same prompts generated, same status transitions, same logs.
- Referenced source-of-truth code: `pkg/spec/spec.go` (status constants + `specTransitions` map + `SetStatus`), `pkg/generator/generator.go` (`markSpecGenerating`, `finalizePrompted`, `executeAndFinalize`), `main.go` `printHelp`, `commands/generate-prompts-for-spec.md`, `docs/running.md`.

## Failure Modes

| Trigger | Expected behavior | Detection | Recovery |
|---------|-------------------|-----------|----------|
| Spec id-or-name does not resolve to any spec | Exit non-zero, error message names the input and "no match" | stderr line + non-zero exit | Operator re-runs with corrected id |
| Spec id-or-name resolves to multiple specs | Exit non-zero, error message names the input and "ambiguous" | stderr line + non-zero exit | Operator re-runs with more specific id |
| Spec is in `status: prompted` already | Exit 0, single-line "already prompted, no-op" message, no file write | stdout line + zero exit + unchanged file mtime | None needed — idempotent |
| Spec is in `status: verifying` / `completed` / `rejected` / `hold` / `idea` / `draft` | Exit non-zero, error message names current status and that the transition is not allowed | stderr line + non-zero exit | Operator inspects spec; no automatic rollback |
| Spec file write fails (disk full, permissions) | Exit non-zero, error message includes the underlying I/O error; spec file left in whatever state the failed write produced (existing `Save` semantics) | stderr line + non-zero exit | Operator inspects + fixes filesystem; re-runs |
| Manual command invoked when prompt-creator wrote zero files | mark-prompted step is skipped; manual command surfaces creator's report unchanged | command output shows no mark-prompted invocation | Operator inspects creator output, re-runs or rejects spec |
| Manual command run again after a successful previous run for the same spec | mark-prompted exits 0 with "already prompted" message; command output unchanged otherwise | command stdout shows the idempotent message | None — designed to be re-run-safe |
| Two operators invoke `mark-prompted` for the same spec concurrently | First wins and transitions to prompted; second hits the idempotent "already prompted" branch and exits 0 | both invocations exit 0, file mtime corresponds to first write | None — single-operator repo, race is benign |

## Security / Abuse Cases

Not applicable. The CLI runs locally as the operator, operates only on files in the project's `specs/` tree, and accepts no network input. The id-or-name argument is matched against existing spec filenames using the established resolver — no path traversal surface beyond what `spec approve` / `spec complete` already expose.

## Acceptance Criteria

- [ ] `dark-factory spec mark-prompted --help` (or `dark-factory spec` help) lists the new subcommand — evidence: stdout contains the literal string `spec mark-prompted`.
- [ ] Running `dark-factory spec mark-prompted <id>` against a fixture spec with `status: approved` exits 0 — evidence: exit code 0.
- [ ] After the above invocation, the fixture spec's frontmatter `status` field equals `prompted` — evidence: file content diff / `grep -E '^status: prompted$' <file>` returns one match.
- [ ] After the above invocation, the fixture spec's frontmatter contains a `prompted:` timestamp field — evidence: `grep -E '^prompted:' <file>` returns one match.
- [ ] The frontmatter produced by `mark-prompted` on an `approved` fixture is byte-identical to the frontmatter `pkg/generator`'s `finalizePrompted` writes for the same starting state — evidence: `diff` of the two output files (with timestamps held constant via a fake clock) returns no differences.
- [ ] Running `dark-factory spec mark-prompted <id>` against a fixture spec with `status: generating` exits 0 and transitions to `prompted` — evidence: exit code 0 and `grep -E '^status: prompted$' <file>` returns one match.
- [ ] Running `dark-factory spec mark-prompted <id>` against a fixture spec with `status: prompted` exits 0 without modifying the file — evidence: exit code 0 AND file mtime unchanged AND stdout contains the literal string `already prompted`.
- [ ] Running `dark-factory spec mark-prompted <id>` against a fixture spec with `status: completed` exits non-zero — evidence: non-zero exit code AND stderr contains the current status (`completed`).
- [ ] Running `dark-factory spec mark-prompted <id>` against a fixture spec with `status: draft` exits non-zero — evidence: non-zero exit code AND stderr contains the current status (`draft`).
- [ ] Running `dark-factory spec mark-prompted <unknown-id>` exits non-zero — evidence: non-zero exit code AND stderr contains the input id and a no-match indication.
- [ ] `commands/generate-prompts-for-spec.md` contains an instruction step that invokes `dark-factory spec mark-prompted` on the input spec after the prompt-creator agent returns and the per-prompt audits complete — evidence: `grep -n 'spec mark-prompted' commands/generate-prompts-for-spec.md` returns at least one line.
- [ ] `commands/generate-prompts-for-spec.md` explicitly states that the mark-prompted step is skipped when prompt-creator produced zero files or reported failure — evidence: `grep -n -i 'zero\|no prompts\|skip\|failure' commands/generate-prompts-for-spec.md` returns a line in the relevant section.
- [ ] `docs/running.md` § "Two ways to generate prompts from an approved spec" describes the manual path as performing the lifecycle transition to `prompted` — evidence: `grep -n -i 'mark-prompted\|transitions.*prompted' docs/running.md` returns at least one line in that section.
- [ ] `make precommit` exits 0 in the repo root — evidence: exit code 0.
- [ ] `pkg/spec/spec.go`'s `specTransitions` map is unchanged by this work — evidence: `git diff` of `pkg/spec/spec.go`'s `specTransitions` block returns no lines (the state machine itself is not modified).
- [ ] `pkg/generator/generator.go` is unchanged by this work — evidence: `git diff pkg/generator/generator.go` returns no lines.

No scenario AC is added. The behavior is CLI-only and fully observable via unit + CLI-level integration tests in `main_internal_test.go` (matching the pattern used for `spec approve` and `spec complete`); no real Docker, real `gh`, or real cluster is required.

## Verification

```
make precommit
```

Plus a manual sanity check on a fixture spec:

```
dark-factory spec mark-prompted <approved-fixture-id>
grep '^status:' specs/in-progress/<resolved-file>
# expect: status: prompted

dark-factory spec mark-prompted <approved-fixture-id>
# expect: exit 0, stdout contains "already prompted"

dark-factory spec mark-prompted <completed-fixture-id>
# expect: non-zero exit, stderr names current status
```

## Do-Nothing Option

If we don't ship this: after spec 089 lands, every spec the operator generates prompts for via the manual command stays at `status: approved` permanently. `dark-factory status`, `IsActive()`, the lifecycle reporting in `dark-factory list`, and any downstream automation keyed on spec status all report wrong state for the majority of specs. Operators either ignore the status drift (compounding over time and eroding trust in `dark-factory status`) or hand-edit frontmatter (forbidden by project rules). Not acceptable — the manual command becomes the default path post-089, so the gap immediately becomes the common case rather than an edge case.
