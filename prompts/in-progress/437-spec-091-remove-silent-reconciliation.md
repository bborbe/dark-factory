---
status: approved
spec: [091-doctor-command]
created: "2026-06-02T00:00:00Z"
queued: "2026-06-01T22:42:15Z"
---

<summary>
- Deletes the `reindexAll` function and its call from `pkg/runner/lifecycle.go` startup sequence — the daemon no longer silently renames spec or prompt files on startup
- Removes the `Mover` field from `pkg/runner.StartupDeps`, the `runner.runner` struct, and the `oneShotRunner` struct since the only consumer (the now-removed `reindexAll`) is gone
- Drops the `mover prompt.FileMover` parameter from `runner.NewRunner` and `runner.NewOneShotRunner` public constructors; updates all callers in `pkg/factory/factory.go` and ~10 test sites in `pkg/runner/runner_test.go` and `pkg/runner/oneshot_test.go`
- The `Mover` field on `processor.WorkflowDeps` STAYS — it is used by `processor.WorkflowExecutor` for legitimate file moves during prompt execution, which is unrelated to the silent reconciliation
- The `reindex` package itself STAYS — prompt 2's doctor fixer uses `reindex.NewReindexer` to compute renumber fixes on operator demand
- The `pkg/spec.normalize.RenumberSpecsAfterRemoval` function STAYS — it is used by `pkg/cmd/spec_unapprove.go` which is operator-driven
- The daemon's startup sequence is now `migrateQueueDir → createDirectories → resumeOrResetExecuting → normalizeFilenames → migrateSpecSlugs` (one step fewer)
- Existing tests in `pkg/runner/`, `pkg/factory/`, `pkg/reindex/`, `pkg/spec/`, `pkg/prompt/`, `pkg/cmd/` continue to pass without modification beyond the constructor signature change

</summary>

<objective>
Strip the silent startup reconciliation path that today renumbers spec and prompt files on every daemon boot. The functionality is being replaced by the operator-driven `dark-factory doctor --fix` flow (prompts 1 + 2). Per the spec, demote-to-log is REJECTED — the call sites are deleted outright, not made conditional. This prompt is a pure deletion + call-site update; no new logic, no new knobs, no new tests beyond mechanical test-call updates.

</objective>

<context>
Read `/workspace/CLAUDE.md` first for project conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-architecture-patterns.md` for the Interface → Constructor → Struct pattern (the runner constructors stay first-class; only their parameter list changes).

Files to read end-to-end before editing:
- `/workspace/prompts/1-spec-091-add-doctor-detection-package.md` — establishes the read-only detection layer that replaces the silent path
- `/workspace/prompts/2-spec-091-add-doctor-fixer-and-cli.md` — establishes the operator-driven `--fix` flow that takes over from the silent path
- `/workspace/specs/in-progress/091-doctor-command.md` — Desired Behavior #12 ("The daemon's existing startup reconciliation path is removed entirely. The silent renumber code path is deleted, not demoted to log.")
- `/workspace/pkg/runner/lifecycle.go` lines 46–138 — the six-step `startupSequence` and the `reindexAll` helper that must be deleted
- `/workspace/pkg/runner/lifecycle.go` line 25–44 — `StartupDeps` struct (the `Mover` field is removed)
- `/workspace/pkg/runner/runner.go` lines 42–128 — `NewRunner` signature (drop the `mover` parameter) and the `runner` struct (drop the `mover` field); lines 242–261 — `startupDeps` (drop the `Mover` assignment)
- `/workspace/pkg/runner/oneshot.go` lines 30–158 — same as above for `NewOneShotRunner` and the `oneShotRunner` struct
- `/workspace/pkg/factory/factory.go` line 482–496 — `CreateRunner`'s call to `runner.NewRunner(...)` drops the `releaser`-as-`mover` argument
- `/workspace/pkg/factory/factory.go` line 563–632 — `CreateOneShotRunner`'s call to `runner.NewOneShotRunner(...)` drops the same
- `/workspace/pkg/runner/runner_test.go` — ~10 call sites of `runner.NewRunner(...)` (lines 70, 366, 434, 652, 735, 828, 917, 980, 1293, 1566) drop the corresponding `mover` arg
- `/workspace/pkg/runner/oneshot_test.go` — 3 call sites of `runner.NewOneShotRunner(...)` (lines 61, 265, 311) drop the same
- `/workspace/pkg/factory/factory_test.go` — test mocks for `runner.Runner` and `runner.OneShotRunner` (verify no test asserts the `mover` arg passed through)
- `/workspace/pkg/processor/workflow_helpers.go` and `processor/workflow_executor_*.go` — the `WorkflowDeps.FileMover` field STAYS (this is for legitimate prompt-execution file moves); verify no caller of `WorkflowDeps.FileMover` is affected by this prompt
- `/workspace/pkg/spec/normalize.go` line 55–96 — `RenumberSpecsAfterRemoval` STAYS (used by `pkg/cmd/spec_unapprove.go` which is operator-driven and outside the daemon's startup path)
- `/workspace/pkg/reindex/reindex.go` — `NewReindexer` and `Reindex` STAY (used by prompt 2's fixer)
- `/workspace/pkg/reindex/specref.go` — `UpdateSpecRefs` STAYS (used by prompt 2's fixer)
- `/workspace/pkg/cmd/spec_unapprove.go` line 106–110 — calls `spec.RenumberSpecsAfterRemoval`; verify it is NOT in the startup path
- `/workspace/docs/architecture-flow.md` — current daemon lifecycle; update the description of startup steps to reflect the new 5-step sequence

</context>

<requirements>

1. **Delete the silent reconciliation from `pkg/runner/lifecycle.go`** — this is the core change. Three edits in this file:
   - **Delete** the `reindexAll` function (lines 106–138). It is the function that scans spec dirs, calls `reindex.NewReindexer` to compute renames, and calls `reindex.UpdateSpecRefs` to rewrite prompt frontmatter. Nothing else in the codebase calls it after this prompt (grep `reindexAll` should return 0 hits after the deletion). Verify by `grep -rn 'reindexAll' /workspace/pkg /workspace/main.go /workspace/cmd 2>/dev/null` — must return 0 lines.
   - **Delete** the call to `reindexAll` in `startupSequence` (lines 89–91). The four lines of `specDirs := …`, `promptDirs := …`, `if err := reindexAll(…); err != nil`, and the matching `}` block. The function's startup sequence drops from 6 steps to 5.
   - **Update** the doc comment on `startupSequence` (lines 46–58) to reflect the new 5-step list: `migrateQueueDir → createDirectories → resumeOrResetExecuting → normalizeFilenames → migrateSpecSlugs`. The reference to "resolve cross-directory number conflicts" is removed.
   - **Update** the comment block on `runner.Run` (line 178–182) and `oneShotRunner.Run` (line 124–128) that says "Run the six shared startup steps" to "Run the five shared startup steps". Same text fix in `pkg/runner/runner.go` and `pkg/runner/oneshot.go`.
   - **Remove** the `Mover` field from `StartupDeps` (line 42). Update all 5 call sites in `pkg/runner/runner.go` and `pkg/runner/oneshot.go` that set `Mover: r.mover` (lines 258 and 156) and any other references.
   - **Remove** the `reindex` import from `pkg/runner/lifecycle.go` if no other code in the file uses it after the deletion (verify by reading the file post-deletion).

2. **Drop the `mover` parameter from `pkg/runner/runner.go`**:
   - `NewRunner` (line 42–98) — remove the `mover prompt.FileMover,` parameter. The signature drops from 21 parameters to 20.
   - The `runner` struct (line 121) — delete the `mover prompt.FileMover` field.
   - `startupDeps()` (line 244) — delete the `Mover: r.mover,` line.

3. **Drop the `mover` parameter from `pkg/runner/oneshot.go`**:
   - `NewOneShotRunner` (line 30–72) — remove the `mover prompt.FileMover,` parameter.
   - The `oneShotRunner` struct (line 92) — delete the `mover prompt.FileMover` field.
   - `startupDeps()` (line 142) — delete the `Mover: r.mover,` line.

4. **Update `pkg/factory/factory.go`** — the two callers in the factory's `CreateRunner` and `CreateOneShotRunner`:
   - In `CreateRunner` (around line 482–496), the call to `runner.NewRunner(...)` no longer receives the trailing `releaser` argument. The current call passes `releaser,` as one of the last args (line ~489 — verify by reading). Remove that argument. The `releaser` local variable STAYS (it is still used by `CreateProcessor` and other consumers).
   - In `CreateOneShotRunner` (around line 563), the call to `runner.NewOneShotRunner(...)` no longer receives the trailing `releaser` argument. The `releaser` arg is at **line 581** (verified — line 567 is `cfg.Prompts.LogDir`, NOT the releaser). Remove that line. The `releaser` local variable STAYS.
   - Do NOT remove the `mover` argument from `CreateProcessor` or `CreateWorkflowExecutor` — those are not part of the runner's startup path. The `prompt.FileMover` interface and its callers in `processor` STAY (used for legitimate prompt-execution file moves).

5. **Update test call sites** — these are mechanical edits that drop one argument from each `NewRunner` / `NewOneShotRunner` call:
   - `/workspace/pkg/runner/runner_test.go` — 9 `runner.NewRunner(...)` call sites at the lines listed in context AND one extra `NewOneShotRunner(...)` call site at **line 1443** (find via `grep -n 'NewOneShotRunner(' pkg/runner/runner_test.go`). Each drops the `mover` argument. Use `git diff` to see the exact form (most pass a `&mocks.ReindexFileMover{}` or a real `prompt.FileMover` mock — drop that line).
   - `/workspace/pkg/runner/oneshot_test.go` — 3 call sites at lines 61, 265, 311. Same edit.
   - `/workspace/pkg/factory/factory_test.go` — only update tests that explicitly construct a `runner.Runner` or `runner.OneShotRunner` via the constructor (most tests use `factory.CreateRunner` which is the higher-level wrapper and don't need changes). If any test directly calls `runner.NewRunner` / `runner.NewOneShotRunner` and was not already updated, edit it.
   - The trailing argument is identifiable as the LAST positional arg in each call (since the runner constructors were last refactored to take ~20 params; the `mover` arg is the most-recent addition per the history of the constructor).

6. **Update `docs/architecture-flow.md`** — find the section that documents the daemon's startup sequence (the file is the canonical architecture reference per `/home/node/.claude/plugins/marketplaces/coding/docs/architecture-flow.md` in the in-container doc path). The "Startup" or "Lifecycle" section currently lists 6 steps; update it to 5 and add a one-line note explaining that the silent reindex was removed in spec 091 and that operators run `dark-factory doctor` for the same detection on demand. Cite the spec.

7. **Verify the deletion is complete and clean**:
   - `cd /workspace && grep -rn 'reindexAll\|RenumberSpecsAfterRemoval' pkg/runner/ pkg/factory/ pkg/specwatcher/ pkg/cmd/ main.go` returns 0 lines.
   - `cd /workspace && grep -rn 'Mover:' pkg/runner/lifecycle.go` returns 0 lines (the field is gone from `StartupDeps`).
   - `cd /workspace && grep -rn 'mover prompt.FileMover\|mover:                 mover' pkg/runner/runner.go pkg/runner/oneshot.go` returns 0 lines (the parameter and field are gone).
   - The `reindex` package itself is still present and its tests still pass (it is used by prompt 2's fixer).

8. **Run `cd /workspace && make precommit`** — must pass. The most likely failure is gosec flagging the deleted `Mover` field for unused-import warnings in `pkg/factory/factory.go`; if `releaser` is now only used by `CreateProcessor`/`CreateWorkflowExecutor` (not by `NewRunner`/`NewOneShotRunner`), the imports stay valid.

</requirements>

<constraints>
- DO NOT introduce any new config knob, opt-out flag, or escape hatch. The spec § Non-goals: "An escape hatch on the Goal is itself a regression."
- DO NOT add a log-only "doctor suggestion" emitted from the daemon. The spec § Desired Behavior #12 explicitly REJECTS demote-to-log: "Demote-to-log was considered and rejected: it keeps dead code on the silent-mutation surface and creates a permanent grep-allowlist entry."
- DO NOT modify the `pkg/reindex` package. The doctor fixer in prompt 2 needs it.
- DO NOT modify `pkg/spec/normalize.go`'s `RenumberSpecsAfterRemoval`. It is operator-driven via `pkg/cmd/spec_unapprove.go`, not daemon-driven.
- DO NOT modify the `WorkflowDeps.FileMover` field in `pkg/processor/types.go` or any `processor/workflow_executor_*.go`. That is the legitimate prompt-execution move path (used by `WorkflowExecutor` to move files between lifecycle dirs during prompt completion), completely unrelated to the silent reconciliation.
- DO NOT modify `pkg/processor/`, `pkg/specwatcher/`, or `pkg/specsweeper/`. The spec § Constraints: "Daemon behavior change is limited to removing/demoting the silent reconciliation path. No other daemon ticks, watchers, or processors are modified."
- DO NOT touch any of the prompt 1 or prompt 2 files (`pkg/doctor/`, `pkg/cmd/doctor.go`, `pkg/lock/filelock.go`, `pkg/cmd/doctor_test.go`). This prompt is a pure deletion of legacy code.
- DO NOT add a CHANGELOG entry in this prompt. The CHANGELOG entry for both the doctor command AND the silent-reconciliation removal is owned by prompt 4 (`docs-and-changelog`). Adding it here would either duplicate prompt 4's entry or split a single changelog concept across two prompts.
- All edits are mechanical. No new exports, no new interfaces, no new tests beyond updating the existing call sites to match the new constructor signatures. Coverage of unchanged code does not regress (the deleted code had its own tests, which are also deleted as part of step 5's call-site cleanup; any test file under `pkg/runner/` that exclusively tested the `reindexAll` path is removed in the same edit).
- Do NOT commit. dark-factory handles git.
- File mode `0600` for any test-fixture frontmatter writes; `0750` for directories the test creates. Existing project conventions.

</constraints>

<verification>
- `cd /workspace && make precommit` exits 0.
- `cd /workspace && go test -count=1 ./pkg/runner/... ./pkg/factory/... ./pkg/reindex/... ./pkg/processor/... ./pkg/spec/... ./pkg/prompt/... ./pkg/cmd/...` exits 0. All existing tests pass.
- `cd /workspace && grep -rn 'reindexAll\|RenumberSpecsAfterRemoval' pkg/ pkg/factory/ pkg/specwatcher/ pkg/cmd/ main.go` returns 0 lines. (The `RenumberSpecsAfterRemoval` in `pkg/spec/normalize.go` is a function definition, not a call site; this grep targets call sites only. The function definition itself stays.)
- `cd /workspace && grep -rn 'Mover:                 r.mover\|Mover: r.mover' pkg/` returns 0 lines.
- `cd /workspace && grep -rn 'mover prompt.FileMover' pkg/runner/runner.go pkg/runner/oneshot.go` returns 0 lines (the parameter is gone from both constructors).
- `cd /workspace && grep -n 'releaser' pkg/factory/factory.go` still returns ≥1 line (the `releaser` local is still used by `CreateProcessor` / `CreateWorkflowExecutor`).
- `cd /workspace && grep -n 'mover' pkg/factory/factory.go` returns 0 lines in the `CreateRunner` and `CreateOneShotRunner` functions (verified by reading those function bodies in the diff).
- `cd /workspace && git diff --name-only HEAD` shows ONLY files under `pkg/runner/`, `pkg/factory/`, `docs/architecture-flow.md`, and the test files in `pkg/runner/*_test.go` and `pkg/factory/*_test.go`. No changes to `pkg/doctor/`, `pkg/cmd/doctor.go`, `pkg/lock/`, `pkg/processor/`, `pkg/specwatcher/`, or `main.go` (those are out of scope — `main.go` does not call the runner constructors directly, the factory does).
- `cd /workspace && go test -count=1 -mod=mod ./pkg/runner/ -run 'Startup' -v` exits 0. (The lifecycle tests are the most direct check that the new 5-step sequence runs without calling `reindexAll`.)
- `cd /workspace && go test -count=1 -mod=mod ./pkg/runner/ -run 'NewRunner\|NewOneShotRunner' -v` exits 0. (Confirms the constructors compile with the dropped parameter and the existing test call sites have been updated to match.)

</verification>
