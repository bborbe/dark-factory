---
status: approved
spec: ["104"]
created: "2026-07-12T19:00:00Z"
queued: "2026-07-12T19:08:22Z"
---

<summary>

- Wires the `backend` config value into the dependency graph: when `backend: local`, the factory builds the local subprocess executor/checker/stopper instead of the docker ones; when unset or `docker`, it builds the docker ones exactly as today.
- The single switch routes BOTH prompt execution and spec→prompts generation in-process, because the generator is handed the same injected executor.
- No caller package (`pkg/runner`, `pkg/promptresumer`, `pkg/processor`, ...) changes — they depend only on the neutral interfaces and compile unchanged.
- Adds a guard test proving the docker argv is byte-identical to before (default backend is a no-op) and a guard proving caller packages are untouched.
- Adds a test proving that with `backend: local` the factory produces a local executor (no docker CLI is invoked when a prompt runs).
- Resolves how a restarted daemon recovers a local execution that cannot reattach (re-run the prompt) — see the reattach-recovery decision in requirements.

</summary>

<objective>
Switch the factory's `NewDocker*` executor/checker/stopper construction sites to the `NewLocalSubprocess*` variants when `cfg.Backend == config.BackendLocal`, covering both the prompt-execution path and the generation path, with zero caller-package changes. Add guard tests for docker-argv byte-identity, caller-diff emptiness, and local-backend selection.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read the parent spec end-to-end:
- `/workspace/specs/in-progress/104-local-execution-backend.md` — Desired Behavior 4; Constraints; Failure Modes row "pod restarts mid-local-execution"; Acceptance Criteria 3, 5, 6 (per the spec's Suggested Decomposition; AC 4 is owned by prompt 2, AC 8 by prompt 4).

Read these coding-plugin docs (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — factory has zero business logic; a backend SELECT is a construction concern; keep it minimal.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — table/golden tests, coverage.
- `/workspace/docs/execution-backends.md` — the "Adding a Backend" blueprint: this is the factory-switch half; note the line numbers there are stale, so anchor by call-site (see verified facts).

PREREQUISITE: prompts 1 and 2 MUST be on the tree. If `grep -rn 'BackendLocal' pkg/config/` returns nothing OR `grep -rn 'NewLocalSubprocessExecutor' pkg/executor/local_subprocess.go` returns nothing, STOP and report `Status: failed` with message "prompt 1 (config) and/or prompt 2 (local executor) not yet on tree". (Grep the `pkg/config/` package dir, not a specific file — prompt 1 may place the `BackendLocal` constant in `backend.go` OR `config.go`.)

Read these files END-TO-END before editing:
- `/workspace/pkg/factory/factory.go` — the file that wires everything. The docker construction sites to switch (anchor by function + call, NOT line number — verify each with grep):
  1. `createContainerDeps(ctx, currentDateTimeGetter)` — returns `executor.NewDockerExecutionChecker(currentDateTimeGetter)` (in the `return cl, executor.NewDockerExecutionChecker(...)` line). This is the daemon-path `ExecutionChecker`. It is called from two sites (the daemon runner and the one-shot runner).
  2. The daemon runner's `NewDockerExecutionStopper()` argument (in `CreateRunner`, passed to `runner.NewRunner(...)`).
  3. The generator's executor + checker: inside `CreateSpecGenerator` (the `generator.NewSpecGenerator(executor.NewDockerExecutor(specGenPolicy, cfg.Model, ...), executor.NewDockerExecutionChecker(currentDateTimeGetter), ...)` call).
  4. The processor's executor: inside `CreateProcessor` (the `exec := executor.NewDockerExecutor(processorPolicy, cfg.Model, cfg.MaxPromptDuration, currentDateTimeGetter, formatter.NewFormatter(currentDateTimeGetter))` assignment). Note `exec` is passed to BOTH `promptresumer.NewResumer(...)` and `processor.NewProcessor(...)` — one switch covers both.
  - `createContainerCounter()` returns `executor.NewDockerContainerCounter(...)` — this is the SYSTEM-WIDE running-container COUNT (used for the maxContainers gate), NOT a per-execution primitive. See requirement 4 for how to handle it under local.
- `/workspace/pkg/executor/executor.go` — `NewDockerExecutor(policy launchpolicy.Policy, model string, maxPromptDuration time.Duration, currentDateTimeGetter libtime.CurrentDateTimeGetter, fmtr formatter.Formatter) Executor`. Compare with the local constructor from prompt 2: `NewLocalSubprocessExecutor(model string, maxPromptDuration time.Duration, currentDateTimeGetter libtime.CurrentDateTimeGetter, fmtr formatter.Formatter) Executor` (NO policy arg).
- `/workspace/pkg/executor/checker.go` — `NewDockerExecutionChecker(currentDateTimeGetter) ExecutionChecker`; prompt-2 sibling `NewLocalSubprocessExecutionChecker(currentDateTimeGetter) ExecutionChecker`.
- `/workspace/pkg/executor/stopper.go` — `NewDockerExecutionStopper() ExecutionStopper`; prompt-2 sibling `NewLocalSubprocessExecutionStopper() ExecutionStopper`.
- `/workspace/pkg/config/config.go` — `config.BackendLocal` / `config.BackendDocker` (added in prompt 1); `cfg.Backend` field.
- `/workspace/pkg/promptresumer/resumer.go` (lines ~119–163) — `resumePrompt` calls `r.executor.Reattach(ctx, logFile, executionID, remainingDuration)` at line ~153 and wraps any error as "reattach to container", which would FAIL the daemon on the local backend's `ErrReattachUnsupported`. See requirement 5 for the reattach-recovery decision. Also note the existing `!canResume` branch (~line 130) already does `pf.MarkApproved(); pf.Save(ctx)` to re-queue — this is the exact recovery mechanism to reuse.

Verified facts (do not re-derive):
- The factory selects the backend by reading `cfg.Backend`. Introduce ONE small helper `func createExecutor(cfg config.Config, policy launchpolicy.Policy, currentDateTimeGetter libtime.CurrentDateTimeGetter) executor.Executor` that returns the local or docker executor based on `cfg.Backend`, and analogous `createExecutionChecker(cfg, currentDateTimeGetter)` / `createExecutionStopper(cfg)` helpers. This keeps the switch in ONE place and every call site becomes a helper call. The helpers live in `pkg/factory` (a neutral package) so they must speak the neutral interfaces only — they return `executor.Executor` / `executor.ExecutionChecker` / `executor.ExecutionStopper` (no container tokens).
- The generator takes an injected `executor.Executor` — routing the executor via `createExecutor` in `CreateSpecGenerator` makes generation run in-process automatically (AC 5). No generator code change.
- `cfg.Backend` defaults to `config.BackendDocker` (prompt 1) — so an unset backend selects the docker branch, preserving today's behavior byte-for-byte (AC 3).

</context>

<requirements>

## 1. Add backend-select helpers in `pkg/factory`

In `/workspace/pkg/factory/factory.go` add three small construction helpers (place near the other `create*` helpers; keep them free of container tokens so `hotpath-execution-naming-check` stays green):

```go
// createExecutor returns the executor for the configured backend. Docker is the
// default; backend: local returns the in-process subprocess executor (no policy needed).
func createExecutor(
	cfg config.Config,
	policy launchpolicy.Policy,
	model string,
	maxPromptDuration time.Duration,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	fmtr formatter.Formatter,
) executor.Executor {
	if cfg.Backend == config.BackendLocal {
		return executor.NewLocalSubprocessExecutor(model, maxPromptDuration, currentDateTimeGetter, fmtr)
	}
	return executor.NewDockerExecutor(policy, model, maxPromptDuration, currentDateTimeGetter, fmtr)
}

// createExecutionChecker returns the liveness checker for the configured backend.
func createExecutionChecker(cfg config.Config, currentDateTimeGetter libtime.CurrentDateTimeGetter) executor.ExecutionChecker {
	if cfg.Backend == config.BackendLocal {
		return executor.NewLocalSubprocessExecutionChecker(currentDateTimeGetter)
	}
	return executor.NewDockerExecutionChecker(currentDateTimeGetter)
}

// createExecutionStopper returns the stopper for the configured backend.
func createExecutionStopper(cfg config.Config) executor.ExecutionStopper {
	if cfg.Backend == config.BackendLocal {
		return executor.NewLocalSubprocessExecutionStopper()
	}
	return executor.NewDockerExecutionStopper()
}
```

Adjust each helper's parameter list to match the arguments actually available at the call sites you rewrite (e.g. the generator passes `specGenPolicy`, `cfg.Model`, `cfg.ParsedMaxPromptDuration()`; the processor passes `processorPolicy`, `cfg.Model`, `cfg.MaxPromptDuration`). It is fine for `policy` to be constructed and passed even when unused by the local branch — the docker branch needs it and constructing it has no side effects. Keep the helper signatures minimal but sufficient.

## 2. Rewrite the four docker construction sites to use the helpers

Replace each of these with the corresponding helper call (anchor by grep, verify each edit):
- In `createContainerDeps`: `executor.NewDockerExecutionChecker(currentDateTimeGetter)` → `createExecutionChecker(cfg, currentDateTimeGetter)`. NOTE: `createContainerDeps` currently does NOT receive `cfg` — thread `cfg config.Config` (or just the `config.Backend`) into its signature and update BOTH call sites (the daemon runner and the one-shot runner — grep `createContainerDeps(` to find them). This is an INTERNAL factory helper, not a caller package, so changing its signature is in-scope.
- The `executor.NewDockerExecutionStopper()` argument to `runner.NewRunner(...)` → `createExecutionStopper(cfg)`.
- In `CreateSpecGenerator`: `executor.NewDockerExecutor(specGenPolicy, cfg.Model, cfg.ParsedMaxPromptDuration(), currentDateTimeGetter, formatter.NewFormatter(currentDateTimeGetter))` → `createExecutor(cfg, specGenPolicy, cfg.Model, cfg.ParsedMaxPromptDuration(), currentDateTimeGetter, formatter.NewFormatter(currentDateTimeGetter))`; and the adjacent `executor.NewDockerExecutionChecker(currentDateTimeGetter)` → `createExecutionChecker(cfg, currentDateTimeGetter)`.
- In `CreateProcessor`: `exec := executor.NewDockerExecutor(processorPolicy, cfg.Model, cfg.MaxPromptDuration, currentDateTimeGetter, formatter.NewFormatter(currentDateTimeGetter))` → `exec := createExecutor(cfg, processorPolicy, cfg.Model, cfg.MaxPromptDuration, currentDateTimeGetter, formatter.NewFormatter(currentDateTimeGetter))`. (`exec` still flows to both the resumer and the processor — one edit covers both.)

Do NOT change any OTHER factory line. No caller package changes.

## 3. Do NOT change generation wiring separately

The generator (`generator.NewSpecGenerator`) already takes the injected `executor.Executor` you switched in requirement 2 step 3. Confirm — do NOT add a separate generation switch. (This satisfies AC 5: generation runs in-process under local because it rides the same injected executor.) State in the completion report that generation routing was verified to ride the executor injection with no generator code change.

## 4. `createContainerCounter` under local backend

`createContainerCounter()` returns `executor.NewDockerContainerCounter(...)` and feeds the maxContainers gate (system-wide running-container count). Under `backend: local` there are no containers to count, and calling `docker ps` would require a docker daemon — violating "no docker daemon required when backend: local" (Constraint, AC 9). Handle this: make the container counter selection backend-aware. Add `executor.NewNoopContainerCounter()` in `pkg/executor` (a `ContainerCounter` whose `CountRunning(ctx)` returns `(0, nil)` — no docker call) and select it in a `createContainerCounter(cfg)` helper when `cfg.Backend == config.BackendLocal`. Thread `cfg` (or the backend) into `createContainerCounter` and update its call sites. There are THREE call sites (grep `createContainerCounter(` — expect the daemon runner, the one-shot runner, and `createStatusChecker`). Update the two RUNTIME sites (the daemon runner and the one-shot runner). For the `createStatusChecker` site: it does not have `cfg` in scope (only `projectMax int`); it backs the `dark-factory status` CLI display, NOT the daemon execution runtime, so AC 9 does not require it. Leave that site calling the docker counter and add a code comment noting `dark-factory status` under `backend: local` may attempt `docker ps` (out of scope for AC 9, which is the daemon RUNTIME). Do NOT thread `cfg` into `createStatusChecker`. Add a one-line unit test for `NewNoopContainerCounter` (`CountRunning` returns `(0, nil)`). This keeps the local path free of any docker invocation.

## 5. Reattach recovery for the local backend (Failure Mode: pod restarts mid-local-execution)

DECISION (implement this): The spec's Desired Behavior 3 says a restarted daemon must recover a local execution by RE-RUNNING the prompt, while AC 5 requires `git diff --stat pkg/promptresumer` to be empty. Reconcile as follows — the local `ExecutionChecker.IsRunning` returns `(false, nil)` (prompt 2), and the resumer's reattach is only reached after `ReconstructState`. The minimal, spec-faithful recovery that keeps `pkg/promptresumer` UNCHANGED is: the local backend's `Reattach` returns `ErrReattachUnsupported`, but the resumer already has a re-queue path (`pf.MarkApproved()` on the `!canResume` branch). To route local executions down the re-queue path WITHOUT editing the resumer, this cannot be achieved purely in the factory.

Therefore: make the SMALLEST possible resumer change. In `resumePrompt`, capture the reattach error into a variable and, when it is the local-backend sentinel, re-queue the prompt instead of failing the daemon:
```go
if reattachErr := r.executor.Reattach(ctx, logFile, executionID, remainingDuration); reattachErr != nil {
    if errors.Is(reattachErr, executor.ErrReattachUnsupported) {
        log.From(ctx).Info(
            "local backend cannot reattach; re-queueing prompt for re-run",
            "prompt_id", filepath.Base(promptPath),
        )
        pf.MarkApproved()
        if err := pf.Save(ctx); err != nil {
            return errors.Wrap(ctx, err, "save prompt after reattach-unsupported")
        }
        return nil
    }
    return errors.Wrap(ctx, reattachErr, "reattach to container")
}
```
This replaces the existing single-line `if err := r.executor.Reattach(...); err != nil { return errors.Wrap(ctx, err, "reattach to container") }` at resumer.go line ~153. It mirrors the existing `!canResume` re-queue branch (`pf.MarkApproved()`). This is a genuine, spec-mandated behavior change, not scope creep — it is the recovery path in Failure Mode row "pod restarts mid-local-execution".

This creates a tension with AC 5's literal `git diff --stat pkg/promptresumer` empty assertion. RESOLUTION: implement the resumer change (correctness wins over a literal diff-emptiness metric), and record in `## Improvements` (category PROMPT) that AC 5's evidence should be amended to "no SIGNATURE/interface change to caller packages" rather than "zero diff", because the reattach-unsupported recovery is a required behavior that lives in the resumer. The neutral-interface contract is still honored (no new interface; `ErrReattachUnsupported` is a sentinel on the existing `Reattach` method). Do NOT change any OTHER caller package.

`pkg/promptresumer` is a NEUTRAL package guarded by `hotpath-execution-naming-check` — the added code must reference `executor.ErrReattachUnsupported` and `pf.MarkApproved()` only; introduce NO container tokens.

## 6. Guard tests

Add to `/workspace/pkg/factory/factory_test.go` (or a new `pkg/factory/backend_switch_test.go`, `package factory_test`):

- **Docker-argv byte-identity (AC 3)**: use the existing `pkg/executor/export_test.go` seam `NewDockerExecutorWithRunnerForTest` to assert the built docker argv is byte-identical when the backend is default/`docker`. Concretely: with `cfg.Backend` unset/`docker`, drive `createExecutor(...)` (or the docker executor it returns) through `NewDockerExecutorWithRunnerForTest` with a captured `commandRunner`, run a prompt, and assert the captured `docker run` argv equals the pre-change golden argv exactly. Do NOT branch on "if a golden exists… else…"; `NewDockerExecutorWithRunnerForTest` is the concrete seam — use it.
- **Backend routing to local (factory selection)**: assert that with `cfg.Backend = config.BackendLocal`, `createExecutor(...)` returns the local executor and running a prompt does NOT invoke docker. The cleanest evidence: `createExecutionChecker(config.Config{Backend: config.BackendLocal}, ...)` returns a checker whose `IsRunning` yields `(false, nil)` and `createExecutionStopper(config.Config{Backend: config.BackendLocal})` returns the local stopper (`StopContainer` returns nil) — both distinguishable from docker without a daemon. Combine with the prompt-2 executor tests that already prove no docker call. (The literal no-docker-call execution evidence is AC 4, owned by prompt 2's executor tests; this test only proves the factory ROUTES to the local backend.)
- **promptresumer reattach-recovery (requirement 5)**: add a `pkg/promptresumer` test with a fake executor whose `Reattach` returns `executor.ErrReattachUnsupported`; assert the prompt is re-queued (`MarkApproved` observed / status flips) and `resumePrompt` returns nil (daemon not failed). Add a CONTROL case: a fake executor whose `Reattach` returns a non-sentinel error still returns the wrapped `"reattach to container"` error (daemon fails as before). This proves the sentinel branch is the ONLY new behavior in the resumer.
- **Caller-diff guard (AC 6)**: this is a repo-level check, not a Go test. Add it to `<verification>` (below), not as a unit test.

## 7. CHANGELOG

Append ONE bullet to `## Unreleased` in `/workspace/CHANGELOG.md`:
```
- feat: route execution and generation through the configured backend in pkg/factory (createExecutor/createExecutionChecker/createExecutionStopper); backend: local selects the in-process subprocess executor, docker stays the default; local reattach recovers by re-queueing the prompt (spec 104 prompt 3)
```

</requirements>

<constraints>

- Default (`backend` unset → `docker`) behavior is byte-for-byte identical to today — same labels, image, argv (AC 3). The docker branch of every helper calls the SAME `NewDocker*` constructor with the SAME arguments as before.
- Backend selection is the ONLY factory change beyond the internal helper signatures. No behavior change to any caller package EXCEPT the single spec-mandated `ErrReattachUnsupported` recovery in `pkg/promptresumer` (requirement 5).
- Introduce NO new interface — the helpers return the existing neutral interfaces.
- No docker daemon may be required at runtime when `backend: local` (AC 9) — hence the noop container counter (requirement 4) and the local checker/stopper that never call docker.
- The `hotpath-execution-naming-check` gate MUST still pass. `pkg/factory` and `pkg/promptresumer` are NEUTRAL packages — the new helper code and the resumer change must NOT introduce `containerName`/`ContainerChecker`/`ContainerStopper`/`containerslot` tokens.
- `make precommit` passes; counterfeiter mocks regenerate cleanly.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.

</constraints>

<verification>

```bash
cd /workspace

# whole repo compiles (proves no caller signature broke) — AC 5 part 1
go build -mod=mod ./...
# expected: exit 0

# factory + resumer + executor tests
go test -mod=mod ./pkg/factory/... ./pkg/promptresumer/... ./pkg/executor/...
# expected: PASS

# caller packages have NO signature/interface change (AC 5) — only the resumer's
# ErrReattachUnsupported recovery is expected; runner/processor must be untouched
git diff --stat pkg/runner pkg/processor
# expected: empty (no changes)
git diff --stat pkg/promptresumer
# expected: ONLY resumer.go, ErrReattachUnsupported recovery (documented deviation from AC-5 literal wording)

# naming gate still green (factory + resumer are neutral)
make hotpath-execution-naming-check; echo "exit=$?"
# expected: exit=0

# local branch reachable without docker: run the local-selection factory tests
go test -mod=mod -run 'Backend|Local' ./pkg/factory/...
# expected: PASS

# changelog entry
grep -n 'spec 104 prompt 3' CHANGELOG.md
# expected: >= 1 line

make precommit
# expected: exit 0
```

</verification>
