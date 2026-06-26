---
status: approved
spec: [102-executor-backend-neutral-naming]
created: "2026-06-26T09:00:01Z"
queued: "2026-06-26T10:11:44Z"
branch: dark-factory/executor-backend-neutral-naming
---

<summary>

- Renames the `pkg/containerslot` package directory to `pkg/executionslot` because the slot it allocates is keyed by the neutral execution ID, not a docker concept.
- Updates the package clause, all import paths, and every call site (processor, factory) to the new name.
- Renames the package-internal `containerName` parameter and the docker-flavored slog/comment wording to neutral terms where they describe the slot key.
- The counterfeiter mock for the slot manager moves to a backend-neutral filename and regenerates cleanly.
- No runtime behavior changes: slot acquisition and release logic are byte-identical, only names move.

</summary>

<objective>
Rename the `pkg/containerslot` package to `pkg/executionslot` (the slot is keyed by the neutral `executionID`, not a docker concept) and update every importer. No runtime behavior change.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read the parent spec:
- `/workspace/specs/in-progress/102-executor-backend-neutral-naming.md` — Desired Behavior 4; Acceptance Criterion 1 (no `containerName` in `pkg/...` neutral packages); the Failure Modes row "containerslot → executionslot package rename leaves dangling import".

Read these source files fully:
- `/workspace/pkg/containerslot/manager.go` — package `containerslot`; interface `Manager` with `Acquire(ctx)` and `ReleaseAfterStart(ctx, containerName, release)`; constructor `NewManager(lock, counter, checker, maxContainers, pollInterval)`; the counterfeiter directive (line 19) targeting `mocks/container-slot-manager.go`. NOTE: this file depends on prompt 1 having renamed `executor.ContainerChecker` → `executor.ExecutionChecker`; the `checker executor.ExecutionChecker` reference must already exist. If it still says `executor.ContainerChecker`, prompt 1 has NOT shipped — STOP and report `Status: failed` with "prompt 1 (executor rename) not yet deployed".
- `/workspace/pkg/containerslot/manager_test.go` — `package containerslot_test`, imports `pkg/containerslot`, many `containerslot.NewManager(...)` calls.
- `/workspace/pkg/containerslot/containerslot_suite_test.go` — `package containerslot_test`, `TestContainerSlot` suite registration with `go:generate` directive.
- `/workspace/pkg/containerslot/manager.go` line 14 imports `pkg/containerlock` — do NOT rename that import (containerlock is out of scope).

Importers / call sites (all must update the import path and selector):
- `/workspace/pkg/processor/processor.go` — import (line 22), `containerslot.Manager` param (line 83) and field (line 156).
- `/workspace/pkg/processor/processor_test.go`, `processor_cancel_test.go`, `processor_retry_test.go` — `containerslot.NewManager(...)` calls.
- `/workspace/pkg/factory/factory.go` — import + `containerslot.NewManager(...)` (line ~1017).
- `/workspace/mocks/container-slot-manager.go` — generated fake; references `containerslot.Manager` (line 156). Will be regenerated to the new path/filename.
</context>

<requirements>

## 1. Move the package directory

1.1. `git mv pkg/containerslot pkg/executionslot`. Then `git mv` the three files inside if their names should follow (keep file basenames but they are inside the renamed dir): the suite test file `containerslot_suite_test.go` SHOULD be renamed to `executionslot_suite_test.go` for consistency; rename `manager.go` and `manager_test.go` stay as-is (generic names). Use `git mv pkg/executionslot/containerslot_suite_test.go pkg/executionslot/executionslot_suite_test.go`.

## 2. Update the package clause and identifiers

2.1. In `pkg/executionslot/manager.go`: change `package containerslot` → `package executionslot`. Update the counterfeiter directive (line 19) to:
```go
//counterfeiter:generate -o ../../mocks/execution-slot-manager.go --fake-name Manager . Manager
```
Update the `Manager` interface doc comment to be neutral: "Manager coordinates the per-host execution-slot concurrency limit." Rename the `ReleaseAfterStart(ctx context.Context, containerName string, release func())` parameter to `executionID` (interface AND impl at line ~96), and update its use inside the goroutine (`cc.WaitUntilRunning(ctx, executionID, 30*time.Second)`). Update the `ReleaseAfterStart` doc comment to say "releases the lock once the named execution is running". You MAY keep the constructor param name `maxContainers` and the slog key `"limit"` / log message strings as-is (those describe the docker concurrency limit and are not in the AC grep scope), but prefer renaming `maxContainers` → `maxExecutions` for clarity if it does not ripple beyond this file. The `lock containerlock.ContainerLock` and `counter executor.ContainerCounter` references stay — both out of scope.

2.2. In `pkg/executionslot/manager_test.go`: change `package containerslot_test` → `package executionslot_test`; update the import path `github.com/bborbe/dark-factory/pkg/containerslot` → `github.com/bborbe/dark-factory/pkg/executionslot`; replace all `containerslot.NewManager` → `executionslot.NewManager`.

2.3. In `pkg/executionslot/executionslot_suite_test.go`: change `package containerslot_test` → `package executionslot_test`; rename the test func `TestContainerSlot` → `TestExecutionSlot` and the suite name string `"ContainerSlot Suite"` → `"ExecutionSlot Suite"`.

## 3. Update importers

3.1. `pkg/processor/processor.go`: import path → `github.com/bborbe/dark-factory/pkg/executionslot`; selectors `containerslot.Manager` → `executionslot.Manager` (param line ~83, field line ~156). The struct field name `containerSlotManager` MAY stay (it is a processor-internal field, not in the AC grep scope for `pkg/processor`), but prefer renaming to `executionSlotManager` for clarity — if you rename it, update all uses within `processor.go` and its tests.

3.2. `pkg/processor/processor_test.go`, `processor_cancel_test.go`, `processor_retry_test.go`: import path + `containerslot.NewManager` → `executionslot.NewManager`.

3.3. `pkg/factory/factory.go`: import path + `containerslot.NewManager` → `executionslot.NewManager` (line ~1017).

## 4. Regenerate the mock

4.1. Run `go generate ./...`. This produces `mocks/execution-slot-manager.go` (package `mocks`, `var _ executionslot.Manager = new(Manager)` self-check at the bottom).

4.2. `git rm mocks/container-slot-manager.go` (counterfeiter does not delete the old output path).

4.3. Verify `go generate ./...` is idempotent (run twice, no diff).

## 5. Changelog

Append to `## Unreleased`:
```
- refactor: Rename pkg/containerslot to pkg/executionslot (slot is keyed by neutral executionID) (spec 102)
```

</requirements>

<constraints>
- No new third-party dependencies.
- Counterfeiter mocks must regenerate cleanly with `go generate ./...`.
- BSD license headers preserved on every touched file.
- Do NOT rename the `containerlock` package or `executor.ContainerCounter` — both out of scope.
- The `dark-factory.project` docker label and all observable strings are unchanged.
- No behavior change at runtime — slot acquire/release logic is byte-identical.
- Depends on prompt 1: `pkg/executionslot/manager.go` must reference `executor.ExecutionChecker`. If it still says `executor.ContainerChecker`, STOP and report `Status: failed` with "prompt 1 not yet deployed".
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
</constraints>

<verification>
Run from `/workspace`:

```
test ! -d pkg/containerslot && echo "old dir gone"
grep -rn '"github.com/bborbe/dark-factory/pkg/containerslot"' --include='*.go' . | grep -v vendor
grep -rn 'containerslot\.' --include='*.go' . | grep -v vendor
go generate ./...
git status --porcelain pkg/ mocks/
make precommit
```

Expected: `pkg/containerslot` directory no longer exists; the two `containerslot` greps return zero lines; `mocks/container-slot-manager.go` is gone and `mocks/execution-slot-manager.go` exists; `go generate ./...` idempotent; `make precommit` exits 0.
</verification>
