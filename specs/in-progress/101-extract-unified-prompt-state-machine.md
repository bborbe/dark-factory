---
status: approved
approved: "2026-06-26T07:28:27Z"
generating: "2026-06-26T07:40:24Z"
prompted: "2026-06-26T07:59:49Z"
branch: dark-factory/extract-unified-prompt-state-machine
---

## Summary

- Prompt-state interpretation is duplicated across five packages — each reads the tuple `(filesystem location, frontmatter status, container field, docker state)` with its own ad-hoc rules.
- Adding or changing a state today requires editing all five sites; the architecture review (2026-06-25) traced the recent half-state recovery fix and resume-mode chdir fix back to this fanout.
- An undocumented seventh state `pending_verification` already exists in code but is missing from the architecture flow doc — doc/code drift compounds the design problem.
- This spec consolidates the rules into a single owning package, migrates the five consumers, captures the diagram, and locks the boundary with a precommit grep gate.
- No on-disk format changes; no external persistence; existing prompts on disk stay readable.

## Problem

Five packages reinterpret the same four inputs to answer the question "what state is this prompt in?" When the answer shifts — a new state, a new transition, a new recovery edge — the change must land in all five places consistently, and there is no single owner that enforces the transition table. Recent bug fixes (PR #29, PR #30) each worked around the fanout rather than removing it, and the `pending_verification` state lives in `processor.go` without appearing in the architecture diagram. The next state change will repeat the pattern unless one place owns the rules.

## Goal

A single package, `pkg/promptstate`, is the only place in the codebase that interprets the four-tuple `(filesystem location, frontmatter status, container field, docker state)` and decides the authoritative current state of a prompt. The five existing consumers ask `pkg/promptstate` instead of reading the tuple inline. The state machine and its transitions — including the previously-undocumented `pending_verification` state — are captured as a diagram in `docs/architecture-flow.md`. A precommit grep gate blocks future regressions by failing when status comparisons leak outside the allow-list.

## Non-goals

- Do NOT persist state outside the prompt frontmatter — the file remains the source of truth; if a future consumer demands an external store, that is a separate spec.
- Do NOT introduce distributed/multi-operator state coordination — separate spec.
- Do NOT build a UI or HTTP endpoint for state inspection — separate spec.
- Do NOT rename any existing state value on disk (`approved`, `executing`, `committing`, `completed`, `cancelled`) — frontmatter compatibility is invariant.
- Do NOT add a per-package opt-out flag — every consumer migrates; if a future consumer demands the old inline behaviour, that's a regression, not a configuration.
- Do NOT change the public surface of `pkg/prompt.PromptFile` — its `Status` field stays as the on-disk storage type.

## Acceptance Criteria

- [ ] `pkg/promptstate` exists with at least the symbols `State`, `IsValidTransition`, and `InterpretTuple` — evidence: `grep -nE '^(type State|func IsValidTransition|func InterpretTuple)' pkg/promptstate/*.go` returns ≥ 3 lines.
- [ ] All seven states are declared as constants in `pkg/promptstate` — evidence: `grep -cE '^\s*State(Approved|Executing|Committing|Completed|Cancelled|PendingVerification|Aborted)\b' pkg/promptstate/*.go` returns ≥ 7.
- [ ] None of the five legacy consumers retain inline tuple interpretation — evidence: `grep -nE 'prompt\.PromptStatus' pkg/runner/lifecycle.go pkg/promptresumer/resumer.go pkg/committingrecoverer/recoverer.go pkg/queuescanner/scanner.go pkg/cancellationwatcher/watcher.go` returns 0 lines.
- [ ] The repository-wide hotpath grep returns no offenders outside the allow-list — evidence: `grep -rE 'prompt\.PromptStatus' pkg/ | grep -v '^pkg/promptstate/' | grep -v '_test\.go:' | grep -v '^pkg/prompt/prompt\.go:'` returns 0 lines.
- [ ] `make hotpath-statemachine-check` exits 0 on `master` after migration — evidence: exit code 0.
- [ ] `make hotpath-statemachine-check` exits non-zero when a deliberate `prompt.PromptStatus` comparison is reintroduced in any consumer file (verified by a transient edit during prompt verification, reverted before commit) — evidence: exit code ≠ 0 with stderr naming the offending file:line.
- [ ] `hotpath-statemachine-check` is wired into precommit — evidence: `grep -n 'hotpath-statemachine-check' Makefile` returns ≥ 1 line inside the `precommit` target's transitive dependency chain.
- [ ] `make precommit` exits 0 on the migrated tree — evidence: exit code 0.
- [ ] `docs/architecture-flow.md` contains a state-machine subsection that names all seven states — evidence: `grep -cE 'Approved|Executing|Committing|Completed|Cancelled|PendingVerification|Aborted' docs/architecture-flow.md` returns ≥ 7 AND `grep -cE 'pending_verification|PendingVerification' docs/architecture-flow.md` returns ≥ 1.
- [ ] The state-machine subsection includes a diagram (mermaid or ASCII) showing transitions — evidence: `grep -nE '```mermaid|stateDiagram|state\s+\w+\s*->' docs/architecture-flow.md` returns ≥ 1 line.
- [ ] `pkg/promptstate` has regression tests covering the four recovery edges (resume-executing-stays-executing, executing-to-cancelled, committing-to-completed, half-state committing-frontmatter-in-completed-dir → completed) — evidence: `go test ./pkg/promptstate/... -run 'Recover|Resume|HalfState|Cancel' -v` lists ≥ 4 test functions, all PASS.
- [ ] Existing prompts under `prompts/in-progress/` and `prompts/completed/` remain readable post-migration — evidence: `dark-factory prompt list` exits 0 against an unmodified prompts directory and prints each existing prompt with its prior state label.
- [ ] Counterfeiter mocks regenerate cleanly — evidence: `make generate` exits 0 and `git status --porcelain pkg/` reports no untracked or modified mock files after regeneration.

## Verification

```
make precommit
make hotpath-statemachine-check
go test ./pkg/promptstate/... -v
grep -rE 'prompt\.PromptStatus' pkg/ | grep -v '^pkg/promptstate/' | grep -v '_test\.go:' | grep -v '^pkg/prompt/prompt\.go:'
```

The grep command above must produce no output. Run on a clean checkout of the post-migration branch.

## Desired Behavior

1. A new package `pkg/promptstate` defines an in-memory `State` enum with the seven values listed above, plus a transition table accessible through `IsValidTransition(from, to State) bool`. Transitions not in the table return `false`.
2. The same package exposes `InterpretTuple(location, status, containerField, dockerState) State` — a pure function that takes the four observable inputs and returns the authoritative current state. This is the only function in the codebase allowed to make that decision.
3. `pkg/runner/lifecycle.go`, `pkg/promptresumer/resumer.go`, `pkg/committingrecoverer/recoverer.go`, `pkg/queuescanner/scanner.go`, and `pkg/cancellationwatcher/watcher.go` each replace their inline tuple-reading with a single call into `pkg/promptstate`. Each consumer's external behaviour — what action it takes for each state — stays identical to today.
4. The previously-undocumented `pending_verification` state — currently used in `processor.go` — becomes a first-class `State` value in `pkg/promptstate`, included in the transition table, and documented in `docs/architecture-flow.md`.
5. `docs/architecture-flow.md` gains a "Prompt State Machine" subsection with a diagram (mermaid preferred, ASCII acceptable) showing all seven states and the allowed transitions, including the recovery edges.
6. The Makefile gains a `hotpath-statemachine-check` target that greps for `prompt.PromptStatus` usage outside `pkg/promptstate/`, the `pkg/prompt/prompt.go` storage type, and `_test.go` files. The target exits non-zero on any unexpected match and prints the offending file:line. It is wired into the `precommit` dependency chain.
7. Regression tests in `pkg/promptstate` lock the four recovery edges named in the acceptance criteria, plus the full transition table (every allowed edge passes, a representative disallowed edge fails).

## Constraints

- Frontmatter values on disk MUST NOT change. Files written under any prior dark-factory version remain valid input to `InterpretTuple`.
- `pkg/prompt.PromptFile.Status` keeps its current type and its on-disk YAML tag. The 1:1 mapping `PromptFile.Status ↔ promptstate.State` lives inside `pkg/promptstate`.
- No new third-party dependencies.
- BSD license header preserved on every new file.
- Existing Counterfeiter `//go:generate` directives keep producing identical mock signatures (modulo the new package's added interfaces).
- `make precommit` and the existing `make hotpath-logcheck` continue to pass — the new check augments, never replaces.

## Failure Modes

| Trigger | Detection | Expected behavior | Recovery |
|---|---|---|---|
| Migration leaves a consumer using inline `prompt.PromptStatus` comparison | `make hotpath-statemachine-check` exits non-zero in precommit | Build blocks before merge | Author moves the comparison behind `pkg/promptstate` and re-runs precommit |
| On-disk prompt frontmatter holds a status string `InterpretTuple` does not recognise | `InterpretTuple` returns a sentinel `StateUnknown` (the eighth, error-only sentinel — added to `pkg/promptstate` alongside the seven canonical states) and the caller logs `ERROR unknown_prompt_status status=<raw>` | Daemon does not silently coerce; the prompt is reported in `dark-factory prompt list` as `unknown` | Operator inspects the file, edits the status to a known value, restart resolves |
| Concurrent calls to `InterpretTuple` from the 5 consumers | `InterpretTuple` is pure (no shared mutable state); each consumer reads inputs independently | No interaction; both succeed | `go test -race -v ./pkg/promptstate/... -run Concurrent` reports no data race; output contains `PASS` | Reversible — race appears as red CI |
| Half-state crash: frontmatter `committing` but file already in `prompts/completed/` | `InterpretTuple` returns `StateCompleted` (location wins; matches the PR #30 fix) | Recoverer treats the prompt as completed, no double-commit | Identical to current behaviour; regression test locks this edge |
| New state added to `State` without a corresponding transition table entry | Unit test `TestTransitionTableCoversAllStates` fails — every state must appear as either a source or sink in the table | `go test` exits non-zero | Author adds the transition row and re-runs |
| Counterfeiter mocks become stale after adding `promptstate` interfaces | `make generate` followed by `git status` shows modified mock files | Author runs `make generate` and commits regenerated mocks | Same — single command resolves |
| Docker daemon unavailable when `InterpretTuple` is called | `dockerState` input arrives as `DockerStateUnavailable`; `InterpretTuple` returns the state computed from the other three inputs and never blocks on docker | Caller continues with file-frontmatter truth | Identical to current `cancellationwatcher` fallback — locked by regression test |

## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Create `pkg/promptstate` with `State`, `IsValidTransition`, `InterpretTuple`, and the transition-table + recovery-edge tests | 1, 2, 7 | 1, 2, 11, 13 | — |
| 2 | Migrate the five consumers (`lifecycle`, `promptresumer`, `committingrecoverer`, `queuescanner`, `cancellationwatcher`) to call `pkg/promptstate` | 3 | 3, 4, 8, 12 | prompt 1 |
| 3 | Promote `pending_verification` to a first-class state and migrate `processor.go` | 4 | 2, 4 | prompt 1 |
| 4 | Add `make hotpath-statemachine-check`, wire into `precommit`, document the gate's allow-list | 6 | 5, 6, 7 | prompts 2, 3 |
| 5 | Add the "Prompt State Machine" subsection (diagram + state list) to `docs/architecture-flow.md` | 5 | 9, 10 | prompts 1, 3 |

Rationale: prompt 1 defines the contract — every later prompt builds on its exported surface. Prompts 2 and 3 are migrations that can land in either order once 1 exists; both must complete before 4's grep gate can pass. Prompt 5 documents the finished state machine — its content depends on prompt 3 nailing down the seventh state.

## Do-Nothing Option

The codebase keeps working — the recent half-state and chdir fixes already patched the most visible bugs. The cost of doing nothing is paid the next time a state or transition changes: every future state-machine edit must touch five files, two of which (`cancellationwatcher`, `queuescanner`) had no test coverage of their inline rules until those bug fixes added some. The architecture review flagged this fanout as a top-three drift source. Deferring this spec converts that drift into a recurring tax on every related bug fix, plus continued doc/code mismatch on `pending_verification`.
