---
status: approved
spec: [050-factory-dedup]
created: "2026-04-16T19:50:00Z"
queued: "2026-04-16T21:01:41Z"
---

<summary>
- `createDockerExecutor` (private, single-caller) is removed; its body is inlined directly into `CreateProcessor`
- `createRunnerInstance` (private, single-caller) is removed; its body is inlined directly into `CreateRunner`
- If inlining causes a `funlen` lint violation, the affected function receives a `//nolint:funlen` comment with a brief justification naming the function and explaining why splitting would harm readability
- No behavioral changes — all existing factory tests pass unchanged
- `make precommit` passes with no new lint violations
</summary>

<objective>
Inline the two private helpers `createDockerExecutor` and `createRunnerInstance` back into their single call sites in `pkg/factory/factory.go`. These helpers were extracted only to satisfy the `funlen` linter on the parent function, not because they represent meaningful seams. Having them as separate functions spreads one construction sequence across three functions and makes the file harder to read top-to-bottom.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/` — factory functions should be composition roots; private single-caller helpers that exist only for linter compliance are an anti-pattern.
Read `go-precommit.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `funlen` limit is 80 lines; `//nolint:funlen` is acceptable when a function is a legitimate composition root.

Read `pkg/factory/factory.go` in full before making changes. The two helpers to inline are:

- `createDockerExecutor` (~line 529–548): called from exactly one site, inside `CreateProcessor` (~line 611). It constructs an `executor.Executor` by calling `executor.NewDockerExecutor`.
- `createRunnerInstance` (~line 332–362): called from exactly one site, at the end of `CreateRunner` (~line 326). It constructs the final `runner.Runner` by calling `runner.NewRunner`.

Read `pkg/factory/factory_test.go` to confirm neither helper is tested directly (they are internal plumbing).

Run this before making changes to confirm single-caller status:
```bash
grep -n "createDockerExecutor\|createRunnerInstance" pkg/factory/factory.go
```
If either function has more than two matches (declaration + one call site), stop and report — the spec assumption is wrong.
</context>

<requirements>
1. In `pkg/factory/factory.go`, find `createDockerExecutor` (the function declaration and its call site in `CreateProcessor`).

   Remove the `createDockerExecutor` function body entirely.

   In `CreateProcessor`, replace the call:
   ```go
   createDockerExecutor(
       containerImage, projectName, model, netrcFile,
       gitconfigFile, env, extraMounts, claudeDir, maxPromptDuration,
       currentDateTimeGetter,
       workflow == config.WorkflowWorktree || hideGit,
   ),
   ```
   with the inlined equivalent (copy the body from the removed helper):
   ```go
   executor.NewDockerExecutor(
       containerImage, projectName, model, netrcFile, gitconfigFile, env, extraMounts, claudeDir,
       maxPromptDuration, currentDateTimeGetter, formatter.NewFormatter(),
       workflow == config.WorkflowWorktree || hideGit,
   ),
   ```
   Verify the exact argument list by reading the removed helper body before deleting it.

2. In `pkg/factory/factory.go`, find `createRunnerInstance` (the function declaration and its call site at the end of `CreateRunner`).

   Remove the `createRunnerInstance` function body entirely.

   In `CreateRunner`, replace the call:
   ```go
   return createRunnerInstance(cfg, inboxDir, inProgressDir, completedDir,
       promptManager, releaser, watcher, proc, srv, poller, specWatcher,
       projectName, containerChecker, n, migrator, currentDateTimeGetter,
       createStartupLogger(ctx, cfg, globalCfg))
   ```
   with the inlined `runner.NewRunner(...)` call that was inside the helper. Copy all arguments verbatim from the removed function body, substituting the parameters with the local variables already in scope in `CreateRunner`.

3. After inlining both helpers, run `make test` to confirm tests still pass.

4. Run `make lint` (or `make precommit` if lint is not a separate target). If `CreateProcessor` or `CreateRunner` now exceeds the `funlen` limit (80 lines), add a `//nolint:funlen` comment on the `func` line with a justification:
   ```go
   //nolint:funlen // composition root: wires N subsystems; splitting into sub-helpers hides initialization order
   func CreateProcessor(...) processor.Processor {
   ```
   Use the same comment for `CreateRunner` if needed. Do not restructure the function internals just to satisfy the linter — the `//nolint` is the correct escape hatch here.

5. Verify no other references to the removed helpers exist:
   ```bash
   grep -n "createDockerExecutor\|createRunnerInstance" pkg/factory/factory.go
   # Expected: no output
   ```

6. Update `CHANGELOG.md` — append to the `## Unreleased` section (which prompt 1 of this spec created):
   ```
   - refactor: inline single-caller factory helpers createDockerExecutor and createRunnerInstance
   ```
</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- No behavioral changes — the factory's observable output must be identical
- If, during the grep in the requirements step, either helper is found to have more than one call site, do NOT inline it — raise the discrepancy and stop
- The factory's public API does not change
- Existing factory tests must pass without modification; do not modify test files unless a renamed helper appears in test setup
- Preserve the file's top-to-bottom wiring order after inlining
- `//nolint:funlen` is acceptable with a justification comment; do NOT restructure the function body unnecessarily just to stay under the linter limit
</constraints>

<verification>
Run `make precommit` — must pass.

Confirm helpers are gone:
```bash
grep -n "createDockerExecutor\|createRunnerInstance" pkg/factory/factory.go
# Expected: no output
```

Confirm full acceptance criteria from the spec:
```bash
grep -c "NewDockerContainerCounter\|globalconfig.Load" pkg/factory/factory.go
# Expected: ≤ 2 (one NewDockerContainerCounter inside createContainerCounter, one globalconfig.Load each inside createStatusChecker and CreateRunner/CreateOneShotRunner for MaxContainers resolution)
```
</verification>
