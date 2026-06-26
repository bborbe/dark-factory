---
status: draft
spec: [102-executor-backend-neutral-naming]
created: "2026-06-26T09:00:00Z"
branch: dark-factory/executor-backend-neutral-naming
---

<summary>

- Renames the execution abstraction so callers no longer speak "container": the checker becomes `ExecutionChecker`, the stopper becomes `ExecutionStopper`, and their parameter is `executionID` instead of `containerName`.
- Updates every package that depends on those types (factory, runner, promptresumer, cancellationwatcher, queuescanner if any, generator, processor, containerslot) to use the neutral names.
- The docker-CLI implementation files keep their container-flavored internal variable names — only the exported abstraction and its callers change.
- Counterfeiter mock filenames change to `execution-checker.go` / `execution-stopper.go`; mocks regenerate cleanly with no leftover stale fakes.
- No runtime behavior changes: the docker container is still spawned with identical labels and arguments.
- This is the load-bearing rename that the precommit gate (prompt 4) and the docs (prompt 5) depend on.

</summary>

<objective>
Rename the leaky container vocabulary in the execution abstraction to backend-neutral terms. The exported `Executor` interface, the `ContainerChecker`/`ContainerStopper` interfaces, and every neutral-layer caller use `executionID` / `ExecutionChecker` / `ExecutionStopper`. Container-specific words remain only inside the docker-CLI implementation files. No runtime behavior changes.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read the parent spec end-to-end:
- `/workspace/specs/in-progress/102-executor-backend-neutral-naming.md` — focus on Desired Behavior 1, 2, 3; Acceptance Criteria 1, 2, 3, 10, 11, 12; the Failure Modes table (row "mock regeneration drifts").

Read these coding docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — interface→constructor→struct, counterfeiter annotations, error wrapping.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — counterfeiter mocks, external test packages.

Read these source files fully before editing (they are the rename targets):
- `/workspace/pkg/executor/checker.go` — defines `ContainerChecker` interface (lines 32-40), its `IsRunning(ctx, name)` and `WaitUntilRunning(ctx, name, timeout)` methods, the counterfeiter directive at line 32, and the unrelated `ContainerCounter` (lines 101-149) which is NOT renamed.
- `/workspace/pkg/executor/stopper.go` — defines `ContainerStopper` interface (lines 14-19), its `StopContainer(ctx, name)` method, and the counterfeiter directive at line 14.
- `/workspace/pkg/executor/executor.go` — defines the `Executor` interface (lines 30-46); methods `Execute(ctx, promptContent, logFile, containerName)`, `Reattach(ctx, logFile, containerName, maxPromptDuration)`, `StopAndRemoveContainer(ctx, containerName)`. The internal `dockerExecutor` methods and helpers below the interface keep their container-flavored variable names.
- `/workspace/pkg/runner/runner.go` — `executor.ContainerChecker` (line 58, 117), `executor.ContainerStopper` (line 63, 122), struct fields `containerChecker`/`containerStopper`, and the `ContainerChecker:` deps assignment at line 272.
- `/workspace/pkg/runner/oneshot.go` — `executor.ContainerChecker` param/field (lines 43, 86) and the `ContainerChecker:` deps assignment at line 148.
- `/workspace/pkg/runner/lifecycle.go` — `ContainerChecker` field on the deps struct (line 37, used at line 75), `executor.ContainerChecker` params (lines 146, 174, 201, 250), and several `containerName :=` locals (lines 221, 262).
- `/workspace/pkg/runner/health_check.go` — `executor.ContainerChecker` (lines 33, 65, 105, 239), `executor.ContainerStopper` (lines 39, 70, 180), and `containerName :=` locals (lines 121, 177, 263).
- `/workspace/pkg/promptresumer/resumer.go` — `containerName` locals derived from `pf.Frontmatter.Container` (lines 120, 196), passed to `r.executor.Reattach`/`StopAndRemoveContainer`.
- `/workspace/pkg/cancellationwatcher/watcher.go` — `Watch(ctx, promptPath, containerName)` interface method (line 28), the `watch` helper param (line 63), passed to `w.executor.StopAndRemoveContainer` (line 105).
- `/workspace/pkg/generator/generator.go` — `executor.ContainerChecker` field (line 36) and constructor param (line 55).
- `/workspace/pkg/factory/factory.go` — `executor.NewDockerContainerStopper()` (line 473), `executor.NewDockerContainerChecker(...)` (line 637, 737), `executor.ContainerChecker` return type (line 732) and param (line 920).
- `/workspace/pkg/processor/processor.go` — uses `containerslot.Manager` (do NOT rename the package here; that is prompt 2). It does NOT reference `ContainerChecker`/`ContainerStopper` directly — confirm with grep before assuming.
- `/workspace/pkg/containerslot/manager.go` — uses `executor.ContainerChecker` (lines 37, 53) and `executor.ContainerCounter` (line 36-ish). The package rename is prompt 2; here ONLY swap the `executor.ContainerChecker` type reference to `executor.ExecutionChecker`. The `ReleaseAfterStart(ctx, containerName, release)` param at line 96/29 stays `containerName` for now (prompt 2 owns the package internals); leave it.
</context>

<requirements>

## 1. Rename the `ContainerChecker` interface → `ExecutionChecker`

In `/workspace/pkg/executor/checker.go`:

1.1. Rename the interface type `ContainerChecker` to `ExecutionChecker`. Update its doc comment to describe it neutrally: "ExecutionChecker checks whether a unit of execution (identified by executionID) is currently running."

1.2. Rename the interface method parameter `name string` to `executionID string` in both `IsRunning(ctx context.Context, executionID string) (bool, error)` and `WaitUntilRunning(ctx context.Context, executionID string, timeout time.Duration) error`.

1.3. Update the counterfeiter directive (line 32) to:
```go
//counterfeiter:generate -o ../../mocks/execution-checker.go --fake-name ExecutionChecker . ExecutionChecker
```

1.4. Rename the constructor `NewDockerContainerChecker` → `NewDockerExecutionChecker` (its return type becomes `ExecutionChecker`). The constructor is a docker-CLI factory, but its return type is the neutral interface, so the `Execution` name is correct. The concrete struct `dockerContainerChecker` and its method receiver variable `name` MAY stay container-flavored (docker-CLI implementation per spec Desired Behavior 3) — but the method SIGNATURES must match the renamed interface. To keep it simple and avoid an interface/impl param mismatch lint, rename the concrete method params to `executionID` as well; the `#nosec` comment text and internal `docker inspect ... executionID` usage stay correct.

1.5. Do NOT touch `ContainerCounter`, `NewDockerContainerCounter`, `dockerContainerCounter`, or the `container-counter.go` mock — `ContainerCounter` counts docker containers system-wide and is genuinely docker-specific. The spec renames only Checker and Stopper.

## 2. Rename the `ContainerStopper` interface → `ExecutionStopper`

In `/workspace/pkg/executor/stopper.go`:

2.1. Rename the interface type `ContainerStopper` → `ExecutionStopper`. Doc comment: "ExecutionStopper stops a running unit of execution by its executionID."

2.2. Rename the interface method to `StopContainer(ctx context.Context, executionID string) error`. Keep the method NAME `StopContainer` unchanged (renaming the method name is out of scope and would ripple further); only the parameter name changes to `executionID`.

2.3. Update the counterfeiter directive (line 14) to:
```go
//counterfeiter:generate -o ../../mocks/execution-stopper.go --fake-name ExecutionStopper . ExecutionStopper
```

2.4. Rename constructor `NewDockerContainerStopper` → `NewDockerExecutionStopper` (return type `ExecutionStopper`). The concrete struct `dockerContainerStopper` and its `docker stop` internals stay container-flavored; rename only the method param to `executionID` to match the interface.

## 3. Rename the `Executor` interface parameters

In `/workspace/pkg/executor/executor.go`:

3.1. In the `Executor` interface (lines 30-46), rename the `containerName string` parameter to `executionID string` in all three methods: `Execute`, `Reattach`, `StopAndRemoveContainer`. Type stays `string`.

3.2. The concrete `dockerExecutor` methods and all unexported helpers (`buildDockerCommand`, `timeoutKiller`, `watchForCompletionReport`, `removeContainerIfExists`, `extractPromptBaseName`, `buildRunFuncs*`, etc.) and their `containerName` variables STAY container-flavored — they are docker-CLI implementation per spec Desired Behavior 3. Do NOT rename them. The only requirement is that the `dockerExecutor` method signatures still satisfy the renamed `Executor` interface (Go matches by type, not param name, so the concrete methods may keep `containerName` while the interface declares `executionID` — this compiles fine). Leave the concrete methods exactly as they are except where step 3.1 changed the interface.

3.3. Do NOT change any string literal: `dark-factory.project`, image refs, `docker run` argv, env keys (`YOLO_PROMPT_FILE`, `ANTHROPIC_MODEL`, `YOLO_OUTPUT`), or the `dark-factory.prompt` label. `git diff pkg/executor/launch.go` must show zero changes (you should not edit launch.go at all).

## 4. Update all neutral-layer callers

Replace every `executor.ContainerChecker` → `executor.ExecutionChecker` and `executor.ContainerStopper` → `executor.ExecutionStopper`. Replace `executor.NewDockerContainerChecker` → `executor.NewDockerExecutionChecker` and `executor.NewDockerContainerStopper` → `executor.NewDockerExecutionStopper`. Rename the `containerName` parameters/locals listed below to `executionID`. Update the files:

4.1. `/workspace/pkg/runner/runner.go`:
   - constructor params `containerChecker executor.ContainerChecker` → `executionChecker executor.ExecutionChecker`; `containerStopper executor.ContainerStopper` → `executionStopper executor.ExecutionStopper`.
   - struct fields `containerChecker` → `executionChecker`, `containerStopper` → `executionStopper`, and their assignments.
   - the lifecycle deps assignment `ContainerChecker: r.containerChecker` (line ~272) — the deps field name on the lifecycle struct is renamed in 4.3, so this becomes `ExecutionChecker: r.executionChecker`.

4.2. `/workspace/pkg/runner/oneshot.go`:
   - constructor param `containerChecker executor.ContainerChecker` → `executionChecker executor.ExecutionChecker`; struct field `containerChecker` → `executionChecker`; assignment in the struct literal; the lifecycle deps assignment `ContainerChecker: r.containerChecker` → `ExecutionChecker: r.executionChecker` (line ~148).

4.3. `/workspace/pkg/runner/lifecycle.go`:
   - the deps struct field `ContainerChecker executor.ContainerChecker` (line ~37) → `ExecutionChecker executor.ExecutionChecker`; update its use at line ~75 (`deps.ContainerChecker` → `deps.ExecutionChecker`).
   - all helper params `checker executor.ContainerChecker` → `checker executor.ExecutionChecker` (the local var name `checker` is fine to keep).
   - rename the `containerName :=` locals (lines ~221, ~262) to `executionID :=` and update their uses, including the log key — keep the log VALUE the same but you MAY keep the slog key string `"container"` unchanged to avoid an externally-observable change (the spec forbids changing observable strings; slog keys for these debug/info lines are internal — prefer renaming the Go variable to `executionID` while leaving the slog key literal `"container"` as-is to minimize observable churn).

4.4. `/workspace/pkg/runner/health_check.go`:
   - all `checker executor.ContainerChecker` → `checker executor.ExecutionChecker`; all `stopper executor.ContainerStopper` → `stopper executor.ExecutionStopper`.
   - rename `containerName :=` locals (lines ~121, ~177, ~263) to `executionID`; keep slog key literals `"container"` unchanged (observable-string minimization).

4.5. `/workspace/pkg/promptresumer/resumer.go`:
   - rename the `containerName` locals (lines ~120, ~196, and the `killTimedOutContainer` param at ~225) to `executionID`. These are read from `pf.Frontmatter.Container` — leave the struct field reference `pf.Frontmatter.Container` exactly as-is (the frontmatter field rename is prompt 3 and changes only YAML, not the Go field). Keep slog key literals `"container"` unchanged.

4.6. `/workspace/pkg/cancellationwatcher/watcher.go`:
   - rename the interface method param `Watch(ctx context.Context, promptPath string, containerName string)` (line ~28) to `executionID string`; update the comment at line ~26 to say "executionID is passed as a string to avoid an import cycle with pkg/processor."
   - rename the `Watch` impl param (line ~53) and the `watch` helper param (line ~63) to `executionID`; update the call `w.executor.StopAndRemoveContainer(ctx, executionID)` (line ~105). Keep slog key literal `"container"` unchanged.

4.7. `/workspace/pkg/generator/generator.go`:
   - struct field `containerChecker executor.ContainerChecker` (line ~36) → `executionChecker executor.ExecutionChecker`; constructor param `containerChecker executor.ContainerChecker` (line ~55) → `executionChecker executor.ExecutionChecker`; update the struct-literal assignment and the constructor doc comment ("executionChecker is used to detect whether a generation execution is already running on restart").

4.8. `/workspace/pkg/factory/factory.go`:
   - `executor.NewDockerContainerStopper()` (line ~473) → `executor.NewDockerExecutionStopper()`.
   - `executor.NewDockerContainerChecker(currentDateTimeGetter)` (lines ~637, ~737) → `executor.NewDockerExecutionChecker(currentDateTimeGetter)`.
   - return type `(containerlock.ContainerLock, executor.ContainerChecker, error)` (line ~732) → `(containerlock.ContainerLock, executor.ExecutionChecker, error)`. Do NOT touch `containerlock.ContainerLock` — that package is out of scope per spec.
   - param `containerChecker executor.ContainerChecker` (line ~920) → `executionChecker executor.ExecutionChecker`; update its use in the same function and any positional pass-through.

4.9. `/workspace/pkg/containerslot/manager.go`:
   - swap `executor.ContainerChecker` (lines ~37, ~53) → `executor.ExecutionChecker`. Do NOT touch `executor.ContainerCounter`. Do NOT rename the package or the `ReleaseAfterStart(ctx, containerName, release)` param — prompt 2 owns the package internals.

4.10. Run `grep -rn 'executor.ContainerChecker\|executor.ContainerStopper\|NewDockerContainerChecker\|NewDockerContainerStopper' --include='*.go' .` (excluding vendor and excluding the mocks dir) — there must be ZERO remaining matches in non-test, non-mock files after your edits. Then check `_test.go` files in the touched packages and update them too (mocks references like `&mocks.ContainerChecker{}` become `&mocks.ExecutionChecker{}` after the regenerate in step 5).

## 5. Regenerate counterfeiter mocks

5.1. After renaming the interfaces and updating the directives, run `go generate ./...`. This produces `mocks/execution-checker.go` and `mocks/execution-stopper.go` and should delete-or-leave-stale `mocks/container-checker.go` / `mocks/container-stopper.go`.

5.2. Manually `git rm` the now-orphaned `mocks/container-checker.go` and `mocks/container-stopper.go` (counterfeiter does not delete old output paths). The new fake type names are `mocks.ExecutionChecker` and `mocks.ExecutionStopper`.

5.3. Update every `_test.go` in the touched packages (runner, promptresumer, cancellationwatcher, generator, factory, processor, containerslot, executor) that referenced `mocks.ContainerChecker` / `mocks.ContainerStopper` to use `mocks.ExecutionChecker` / `mocks.ExecutionStopper`. Also update any test-local field/var named `containerChecker`/`containerStopper` if it improves clarity (not strictly required by AC, but keeps the tree clean).

5.4. After all edits, `go generate ./...` must exit 0 and `git status --porcelain pkg/` must show zero MODIFIED mock files under `pkg/` (the mocks live in top-level `mocks/`, so this AC is about the `pkg/` tree being stable after regenerate — verify `go generate ./...` is idempotent: run it twice, second run produces no diff).

## 6. Changelog

Add a `## Unreleased` entry (or append if it exists):
```
- refactor: Rename ContainerChecker/ContainerStopper to ExecutionChecker/ExecutionStopper and executor params to executionID across neutral-layer packages (spec 102)
```

</requirements>

<constraints>
- No new third-party dependencies.
- Counterfeiter mocks must regenerate cleanly with `go generate ./...`.
- BSD license headers preserved on every touched file.
- The `dark-factory.project` docker label and all other externally observable strings are unchanged. Do NOT change slog key literals from `"container"` to `"execution_id"` — those are observable log keys; keep them.
- Backward compatible: no on-disk schema change in this prompt (frontmatter is prompt 3).
- No behavior change at runtime — `git diff pkg/executor/launch.go` shows zero changes.
- Do NOT rename the `containerlock` package or `executor.ContainerCounter` — both out of scope.
- Do NOT rename `pkg/containerslot` — that is prompt 2.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
</constraints>

<verification>
Run from `/workspace`:

```
go generate ./...
git status --porcelain pkg/ mocks/
grep -rn 'executor.ContainerChecker\|executor.ContainerStopper\|NewDockerContainerChecker\|NewDockerContainerStopper' --include='*.go' . | grep -v vendor
grep -rE '\bcontainerName\b' pkg/factory pkg/runner pkg/promptresumer pkg/cancellationwatcher pkg/queuescanner pkg/healthcheckgate --include='*.go' | grep -v _test.go
grep -rE '\bContainerChecker\b|\bContainerStopper\b' pkg/factory pkg/runner pkg/promptresumer pkg/cancellationwatcher pkg/queuescanner --include='*.go' | grep -v _test.go
grep -nE '^type (ExecutionChecker|ExecutionStopper) interface' pkg/executor/checker.go pkg/executor/stopper.go
make precommit
```

Expected: the three grep-for-old-names commands return zero lines; the `grep -nE '^type'` returns two matches; `go generate ./...` is idempotent (run twice, no diff); `make precommit` exits 0.
</verification>
