---
status: idea
created: "2026-04-25T14:33:00Z"
---

<summary>
- Extract two related responsibilities from `pkg/processor/processor.go`:
  - `PreflightConditions`: wraps `checkPreflightConditions` + `checkDirtyFileThreshold` + `checkGitIndexLock` (lines ~943–991). Single method `ShouldSkip(ctx) (skip bool, err error)`. Returns sentinel `processor.ErrPreflightSkip` for the baseline-broken case.
  - `ContainerSlotManager`: wraps `prepareContainerSlot` + `waitForContainerSlot` + `hasFreeSlot` + `startContainerLockRelease` (lines ~825–938). Two methods: `Acquire(ctx) (release func(), err error)` and `ReleaseAfterStart(ctx, ContainerName, release func())`.
- Together remove ~150 lines from processor.go
- Both feed `processPrompt` directly; clean SRP boundaries
</summary>

<objective>
Pull pre-execution gating logic (preflight + slot management) out of the processor god-object into two SRP services so each can be unit-tested without docker / fsnotify and processor's `processPrompt` shrinks to pure orchestration.
</objective>

<context>
**Prerequisites:** A1 + A2 + A3 must have landed first.

Read `CLAUDE.md` for project conventions.
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/`.

Current code (`pkg/processor/processor.go`):
- `checkPreflightConditions(ctx) (bool, error)` at line ~972 — orchestrates baseline preflight, git lock, dirty files
- `checkGitIndexLock()` at line ~964 — wraps `gitLockChecker`
- `checkDirtyFileThreshold(ctx) (bool, error)` at line ~943 — wraps `dirtyFileChecker`
- `prepareContainerSlot(ctx) (func(), error)` at line ~880 — acquires container lock, polls for free slot
- `waitForContainerSlot(ctx) error` at line ~825
- `hasFreeSlot(ctx) bool` at line ~856
- `startContainerLockRelease(ctx, name, release)` at line ~929 — async release-on-running
- All called from `processPrompt` (line ~995, ~1044, ~1051)

Both responsibilities own their own dependencies — preflight owns `preflightChecker` / `gitLockChecker` / `dirtyFileChecker` / `dirtyFileThreshold`; slot manager owns `containerLock` / `containerCounter` / `containerChecker` / `maxContainers` / `containerPollInterval`.
</context>

<requirements>

## 1. New package `pkg/preflightconditions/`

`pkg/preflightconditions/conditions.go`:

```go
package preflightconditions

//counterfeiter:generate -o ../../mocks/preflight-conditions.go --fake-name Conditions . Conditions

// Conditions runs all pre-execution skip checks in order: baseline preflight, git index lock,
// dirty-file threshold. Returns ErrPreflightSkip (re-exported from processor) for the baseline
// case so callers can break the scan loop.
type Conditions interface {
    ShouldSkip(ctx context.Context) (skip bool, err error)
}

func NewConditions(
    preflightChecker preflight.Checker,        // nil = preflight disabled
    gitLockChecker processor.GitLockChecker,    // nil = git lock check disabled
    dirtyFileChecker processor.DirtyFileChecker, // nil = dirty file check disabled
    dirtyFileThreshold processor.DirtyFileThreshold,
) Conditions { ... }
```

Move bodies of `checkPreflightConditions` + `checkGitIndexLock` + `checkDirtyFileThreshold` into `(*conditions).ShouldSkip`. Preserve the `ErrPreflightSkip` sentinel semantics.

## 2. New package `pkg/containerslot/`

`pkg/containerslot/manager.go`:

```go
package containerslot

//counterfeiter:generate -o ../../mocks/container-slot-manager.go --fake-name Manager . Manager

// Manager coordinates the per-host container concurrency limit.
type Manager interface {
    // Acquire blocks until a slot is available, then returns with the lock held
    // and an idempotent release function.
    Acquire(ctx context.Context) (release func(), err error)

    // ReleaseAfterStart releases the lock once the named container is running
    // (or after a 30s timeout). Spawns a goroutine; safe to call after Acquire.
    ReleaseAfterStart(ctx context.Context, name processor.ContainerName, release func())
}

func NewManager(
    lock containerlock.ContainerLock,            // nil = no locking
    counter executor.ContainerCounter,
    checker executor.ContainerChecker,           // nil = skip release-after-start
    maxContainers processor.MaxContainers,
    pollInterval time.Duration,
) Manager { ... }
```

Move bodies of `prepareContainerSlot` + `waitForContainerSlot` + `hasFreeSlot` + `startContainerLockRelease` into the new package.

## 3. Wire into processor

- Add `preflightConditions preflightconditions.Conditions` and `containerSlotManager containerslot.Manager` to `processor` struct
- Add as constructor parameters (services group)
- Replace `processPrompt` body:
  - `p.checkPreflightConditions(ctx)` → `p.preflightConditions.ShouldSkip(ctx)`
  - `p.prepareContainerSlot(ctx)` → `p.containerSlotManager.Acquire(ctx)`
  - `p.startContainerLockRelease(...)` → `p.containerSlotManager.ReleaseAfterStart(...)`
- Delete the 6 helper methods from processor
- Remove fields: `preflightChecker`, `gitLockChecker`, `dirtyFileChecker`, `dirtyFileThreshold`, `containerLock`, `containerCounter`, `containerChecker`, `maxContainers`, `containerPollInterval`

## 4. Wire from factory

Construct both new services in `pkg/factory/factory.go`, pass into `NewProcessor`. Drop the now-redundant primitives from the call.

## 5. Tests

- `pkg/preflightconditions/conditions_test.go`: each branch — preflight nil (skip check), preflight passes, preflight fails (ErrPreflightSkip), preflight error (ErrPreflightSkip), git lock present, dirty files over threshold, dirty files within threshold, all-disabled (returns false, nil)
- `pkg/containerslot/manager_test.go`: lock nil (just polls), lock present and slot free (acquires, returns), slot full (releases, sleeps, retries), ctx cancelled mid-wait (returns ctx error), ReleaseAfterStart with checker nil (no-op), ReleaseAfterStart with checker (goroutine fires release on running)
- Update processor tests to use counterfeiter mocks

## 6. CHANGELOG

```
- refactor: extracted PreflightConditions and ContainerSlotManager from processor — pure refactor, no behaviour change
```

## 7. Verify

```bash
cd /workspace
make generate
make precommit
```

</requirements>

<constraints>
- Pure refactor — no behaviour change
- `ErrPreflightSkip` semantics preserved — caller's `stderrors.Is(err, processor.ErrPreflightSkip)` must still match
- External test packages
- Coverage ≥80% on both new packages
- `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors`
- Do not commit
</constraints>

<verification>
```bash
cd /workspace

# Methods gone from processor
! grep -n "func (p \*processor) checkPreflightConditions\|func (p \*processor) checkGitIndexLock\|func (p \*processor) checkDirtyFileThreshold" pkg/processor/processor.go
! grep -n "func (p \*processor) prepareContainerSlot\|func (p \*processor) waitForContainerSlot\|func (p \*processor) hasFreeSlot\|func (p \*processor) startContainerLockRelease" pkg/processor/processor.go

# New packages exist
ls pkg/preflightconditions/ pkg/containerslot/

# Processor uses interfaces
grep -n "preflightconditions.Conditions\|containerslot.Manager" pkg/processor/processor.go

make precommit
```
</verification>
