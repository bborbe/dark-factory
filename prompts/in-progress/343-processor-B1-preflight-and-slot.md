---
status: committing
summary: Extracted PreflightConditions (pkg/preflightconditions) and ContainerSlotManager (pkg/containerslot) from processor god-object; re-exported ErrPreflightSkip sentinel; wired both services in factory; all call sites updated; 98.4% coverage on new packages; make precommit passed with exit code 0
container: dark-factory-343-processor-B1-preflight-and-slot
dark-factory-version: v0.135.3-1-gf3b7a3f
created: "2026-04-25T14:33:00Z"
queued: "2026-04-25T16:57:11Z"
started: "2026-04-25T16:57:13Z"
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

import (
    "context"
    stderrors "errors"

    "github.com/bborbe/dark-factory/pkg/preflight"
)

//counterfeiter:generate -o ../../mocks/preflight-conditions.go --fake-name Conditions . Conditions

// Conditions runs all pre-execution skip checks in order: baseline preflight, git index lock,
// dirty-file threshold. Returns ErrPreflightSkip for the baseline-broken case.
type Conditions interface {
    ShouldSkip(ctx context.Context) (skip bool, err error)
}

// ErrPreflightSkip is the canonical owner of this sentinel.
// pkg/processor re-exports it via `var ErrPreflightSkip = preflightconditions.ErrPreflightSkip`
// so existing `stderrors.Is(err, processor.ErrPreflightSkip)` call sites continue to match.
var ErrPreflightSkip = stderrors.New("preflight: baseline broken; skip prompt this cycle")

// Local minimal interfaces — defined here (not imported from pkg/processor) to avoid an import cycle.
// Go interface satisfaction is structural, so processor.GitLockChecker / DirtyFileChecker satisfy
// these automatically.
type GitLockChecker interface {
    HasGitLock() bool
}

type DirtyFileChecker interface {
    CountDirtyFiles(ctx context.Context) (int, error)
}

func NewConditions(
    preflightChecker preflight.Checker, // nil = preflight disabled
    gitLockChecker GitLockChecker,       // nil = git lock check disabled
    dirtyFileChecker DirtyFileChecker,   // nil = dirty file check disabled
    dirtyFileThreshold int,              // primitive type — no processor dependency
) Conditions { ... }
```

Move bodies of `checkPreflightConditions` + `checkGitIndexLock` + `checkDirtyFileThreshold` into `(*conditions).ShouldSkip`. Move the `errPreflightSkip`/`ErrPreflightSkip` sentinel into this package. In `pkg/processor/processor.go` keep:

```go
// ErrPreflightSkip re-exports preflightconditions.ErrPreflightSkip so existing
// stderrors.Is callers continue to match without rewriting.
var ErrPreflightSkip = preflightconditions.ErrPreflightSkip
```

`stderrors.Is(err, processor.ErrPreflightSkip)` still matches because both names point to the same `error` value.

## 2. New package `pkg/containerslot/`

`pkg/containerslot/manager.go`:

```go
package containerslot

import (
    "context"
    "time"

    "github.com/bborbe/dark-factory/pkg/containerlock"
    "github.com/bborbe/dark-factory/pkg/executor"
)

//counterfeiter:generate -o ../../mocks/container-slot-manager.go --fake-name Manager . Manager

// Manager coordinates the per-host container concurrency limit.
type Manager interface {
    // Acquire blocks until a slot is available, then returns with the lock held
    // and an idempotent release function.
    Acquire(ctx context.Context) (release func(), err error)

    // ReleaseAfterStart releases the lock once the named container is running
    // (or after a 30s timeout). Spawns a goroutine; safe to call after Acquire.
    // containerName is a primitive string to avoid an import cycle.
    ReleaseAfterStart(ctx context.Context, containerName string, release func())
}

func NewManager(
    lock containerlock.ContainerLock,            // nil = no locking
    counter executor.ContainerCounter,
    checker executor.ContainerChecker,           // nil = skip release-after-start
    maxContainers int,                            // primitive — no processor dependency
    pollInterval time.Duration,
) Manager { ... }
```

Move bodies of `prepareContainerSlot` + `waitForContainerSlot` + `hasFreeSlot` + `startContainerLockRelease` into the new package. Use primitives in the public API (no `processor.MaxContainers` / `processor.ContainerName`) — the processor unwraps `MaxContainers` to `int` and `ContainerName` to `string` at the call boundary.

## 3. Wire into processor

- Add `preflightConditions preflightconditions.Conditions` and `containerSlotManager containerslot.Manager` to `processor` struct
- Add as constructor parameters (services group)
- Replace `processPrompt` body:
  - `p.checkPreflightConditions(ctx)` → `p.preflightConditions.ShouldSkip(ctx)`
  - `p.prepareContainerSlot(ctx)` → `p.containerSlotManager.Acquire(ctx)`
  - `p.startContainerLockRelease(...)` → `p.containerSlotManager.ReleaseAfterStart(...)`
- Delete the 6 helper methods from processor
- Remove fields: `preflightChecker`, `gitLockChecker`, `dirtyFileChecker`, `dirtyFileThreshold`, `containerLock`, `containerCounter`, `containerChecker`, `maxContainers`, `containerPollInterval`

## 4. Wire from factory and update ALL `NewProcessor` call sites

```bash
grep -rn "processor\.NewProcessor(" --include="*.go"
```

Update every call site (factory + ALL test files — recurring lesson from 337/338/339/340: a `newTestProcessor` helper does NOT cover all direct constructor calls in tests).

Construct both new services in `pkg/factory/factory.go`, pass into `NewProcessor`. Drop the now-redundant primitives from the call.

## 5. Tests

- `pkg/preflightconditions/conditions_test.go`: each branch — preflight nil (skip check), preflight passes, preflight fails (ErrPreflightSkip), preflight error (ErrPreflightSkip), git lock present, dirty files over threshold, dirty files within threshold, all-disabled (returns false, nil)
- `pkg/containerslot/manager_test.go`: lock nil (just polls), lock present and slot free (acquires, returns), slot full (releases, sleeps, retries), ctx cancelled mid-wait (returns ctx error), ReleaseAfterStart with checker nil (no-op), ReleaseAfterStart with checker (goroutine fires release on running). For the goroutine test, use a fake `ContainerChecker` whose `WaitUntilRunning` blocks on a channel the test controls — assert release fires only after the test signals; never use `time.Sleep` in the assertion (flaky)
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

# Old fields removed from processor struct
! grep -n "preflightChecker\|gitLockChecker\|dirtyFileChecker\|containerLock\|containerCounter\|containerChecker" pkg/processor/processor.go

# New packages exist
ls pkg/preflightconditions/ pkg/containerslot/

# Processor uses interfaces
grep -n "preflightconditions.Conditions\|containerslot.Manager" pkg/processor/processor.go

# No reverse import — new packages MUST NOT import processor
! grep -rn "github.com/bborbe/dark-factory/pkg/processor" pkg/preflightconditions/ pkg/containerslot/

# Factory wires the new services
grep -n "preflightconditions\.\|containerslot\." pkg/factory/factory.go

# All NewProcessor call sites updated
grep -rn "processor\.NewProcessor(" --include='*.go'

make precommit
```
</verification>
