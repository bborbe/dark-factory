---
status: draft
spec: [102-executor-backend-neutral-naming]
created: "2026-06-26T09:00:04Z"
branch: dark-factory/executor-backend-neutral-naming
---

<summary>

- Adds a new doc explaining the backend-neutral execution abstraction and where the line between neutral and docker-specific vocabulary sits.
- Lists which packages are backend-neutral (callers of the abstraction) and which are the docker-CLI implementation.
- Walks through the exact file set a hypothetical second backend (`LocalSubprocessExecutor`) would touch, proving it is three files or fewer — the evidence that the abstraction actually holds.
- This is documentation only; no code changes, no behavior change.
- Depends on prompts 1 and 2 so the doc can reference the real, final type and package names.

</summary>

<objective>
Write `docs/execution-backends.md` documenting the neutral-vs-container vocabulary split, listing which packages live on which side, and walking through the ≤ 3-file set a hypothetical `LocalSubprocessExecutor` would touch — proving the abstraction is genuinely extensible.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read the parent spec:
- `/workspace/specs/in-progress/102-executor-backend-neutral-naming.md` — Summary bullet 5; Goal; Desired Behavior 7; Acceptance Criterion 7 (the doc must have `## Adding a Backend` listing ≤ 3 files, and `## Neutral vs Container Vocabulary` naming neutral vs docker packages).

DEPENDS ON prompts 1 and 2: the doc must cite the FINAL names (`ExecutionChecker`, `ExecutionStopper`, `executionID`, `pkg/executionslot`, `NewDockerExecutionChecker`, `NewDockerExecutionStopper`). Before writing, confirm those exist: `grep -nE '^type (ExecutionChecker|ExecutionStopper) interface' pkg/executor/checker.go pkg/executor/stopper.go` returns two matches AND `test -d pkg/executionslot`. If not, prompts 1/2 have NOT shipped — STOP and report `Status: failed` with "prompts 1 and/or 2 not yet deployed; cannot document final names".

Read these source files to ground the doc in reality:
- `/workspace/pkg/executor/executor.go` — the `Executor` interface (the neutral abstraction): `Execute(ctx, promptContent, logFile, executionID)`, `Reattach(ctx, logFile, executionID, maxPromptDuration)`, `StopAndRemoveContainer(ctx, executionID)`; and `NewDockerExecutor(policy, model, maxPromptDuration, currentDateTimeGetter, fmtr)` (the docker-CLI constructor).
- `/workspace/pkg/executor/checker.go` and `stopper.go` — `ExecutionChecker` / `ExecutionStopper` interfaces and their `NewDockerExecutionChecker` / `NewDockerExecutionStopper` constructors.
- `/workspace/pkg/factory/factory.go` — the single wiring point: where `NewDockerExecutor`, `NewDockerExecutionChecker`, `NewDockerExecutionStopper` are called (these are the lines a second backend would add an alternative branch to). Find the actual line numbers with `grep -n 'NewDockerExecutor\|NewDockerExecutionChecker\|NewDockerExecutionStopper'`.
- `/workspace/docs/architecture-flow.md` — match its tone, heading style, and depth. The new doc should feel like a sibling.
- `/workspace/pkg/launchpolicy/policy.go` (skim) — the docker launch shape lives here; note it as docker-flavored in the vocabulary table.

Existing docs for tone reference: skim `/workspace/docs/configuration.md` and `/workspace/docs/architecture-flow.md`.
</context>

<requirements>

## 1. Create `docs/execution-backends.md`

Write `/workspace/docs/execution-backends.md`. It is a Markdown doc (no license header needed — docs are not `.go` files). Required structure:

1.1. A short intro paragraph: the `Executor` interface and its collaborators (`ExecutionChecker`, `ExecutionStopper`) are a backend-neutral abstraction. Today there is one backend (docker-CLI). The abstraction names its identifier `executionID`, not `containerName`, so a second backend can be added without touching callers. Reference spec 102.

1.2. A section titled EXACTLY `## Neutral vs Container Vocabulary` (this heading text is asserted by the AC grep). It must contain TWO clearly labelled lists:
   - **Neutral packages** (use the abstraction, must NOT contain container vocabulary; guarded by `make hotpath-execution-naming-check`): `pkg/factory`, `pkg/runner`, `pkg/promptresumer`, `pkg/cancellationwatcher`, `pkg/queuescanner`, `pkg/healthcheckgate`, `pkg/generator`, `pkg/processor`, `pkg/executionslot`. Note that these speak `executionID` / `ExecutionChecker` / `ExecutionStopper`.
   - **Docker-CLI (container-flavored) packages** (own the docker concepts; container vocabulary is correct here): `pkg/executor` (the `dockerExecutor`, `dockerExecutionChecker`, `dockerExecutionStopper`, `dockerContainerCounter`, `launch.go`), `pkg/launchpolicy`, `pkg/containerlock`. Note `ContainerCounter` and `containerlock` stay container-named on purpose.
   - One sentence explaining the gate: any container token (`containerName`, `ContainerChecker`, `ContainerStopper`, `containerslot`) appearing in a neutral package fails `make hotpath-execution-naming-check`.

1.3. A section titled EXACTLY `## Adding a Backend` (asserted by the AC grep). It walks through adding a hypothetical `LocalSubprocessExecutor`. It MUST contain a Markdown list of the files touched, and that list MUST have ≤ 3 entries (the AC counts lines starting with `- ` inside this section's file list). Use exactly these three (or fewer):
   - `- pkg/executor/local_subprocess.go` — new file: a `localSubprocessExecutor` implementing the existing `Executor` interface (`Execute`/`Reattach`/`StopAndRemoveContainer`) plus, if the backend needs liveness/stop, `ExecutionChecker`/`ExecutionStopper` implementations, with a `NewLocalSubprocessExecutor(...)` constructor. No new interface is introduced — it satisfies the existing neutral interfaces.
   - `- pkg/factory/factory.go` — one wiring change: select `NewLocalSubprocessExecutor` instead of `NewDockerExecutor` (and the matching checker/stopper) behind a config switch. Cite the actual lines you found (`NewDockerExecutor`, `NewDockerExecutionChecker`, `NewDockerExecutionStopper` call sites).
   - `- pkg/config/...` (optional third entry) — a config field selecting the backend (e.g. `backend: docker|local`). Mark this as optional so the count can be 2 or 3.
   - After the list, add a sentence: "No caller package changes — `pkg/runner`, `pkg/promptresumer`, `pkg/processor`, etc. already depend only on the neutral interfaces, so they compile unchanged." This is the proof the abstraction holds.

1.4. Keep total file count in the `## Adding a Backend` list ≤ 3 (AC requirement). Do NOT pad the list. If you find while reading `factory.go` that more than the wiring + new file is genuinely required, document the surprise in `## Improvements` (category: PROMPT) rather than inflating the list past 3 — that would itself be evidence the abstraction is NOT clean.

## 2. Cross-link (optional, low-cost)

2.1. If `docs/architecture-flow.md` has a section listing related docs or a "see also", add a one-line link to `execution-backends.md`. If there is no natural place, skip — do not force it.

## 3. Changelog

Append to `## Unreleased`:
```
- docs: Add docs/execution-backends.md documenting the neutral-vs-container vocabulary split and the ≤3-file second-backend walk (spec 102)
```

</requirements>

<constraints>
- Documentation only — no code changes, no behavior change.
- The `## Adding a Backend` file list MUST have ≤ 3 entries (AC #7).
- The two section headings must be EXACTLY `## Neutral vs Container Vocabulary` and `## Adding a Backend` (the AC greps for them verbatim).
- Cite real type/package names produced by prompts 1 and 2 — verify they exist before writing (see context).
- Depends on prompts 1 and 2 — if final names are absent, STOP and report `Status: failed`.
- Do NOT commit — dark-factory handles git.
</constraints>

<verification>
Run from `/workspace`:

```
test -f docs/execution-backends.md && echo "doc exists"
grep -nE '^## Adding a Backend|^## Neutral vs Container Vocabulary' docs/execution-backends.md
# count file-list entries under "## Adding a Backend" — must be <= 3:
awk '/^## Adding a Backend/{f=1;next} /^## /{f=0} f && /^- /{c++} END{print "file_list_entries="c}' docs/execution-backends.md
make check-links
```

Expected: the doc exists; the two headings grep returns exactly two matches; `file_list_entries` is ≤ 3; `make check-links` passes (no broken links). `make precommit` is NOT required for a docs-only change, but run `make check-links` and `make check-changelog`.
</verification>
