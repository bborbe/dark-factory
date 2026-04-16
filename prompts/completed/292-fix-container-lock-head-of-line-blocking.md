---
status: completed
summary: 'Reshaped prepareContainerSlot to release the container flock between slot-wait polls — acquire→check→(free: return held lock)/(full: release lock, sleep, retry) — eliminating head-of-line blocking between dark-factory daemons with different maxContainers limits on the same host, with 7 new regression tests covering all cases including ordering invariant'
container: dark-factory-292-fix-container-lock-head-of-line-blocking
dark-factory-version: v0.110.2
created: "2026-04-16T10:58:30Z"
queued: "2026-04-16T10:58:30Z"
started: "2026-04-16T10:58:31Z"
completed: "2026-04-16T11:10:36Z"
---
<summary>
- `pkg/processor/processor.go` currently acquires the system-wide `~/.dark-factory/container.lock` BEFORE entering `waitForContainerSlot`, so a daemon that is waiting because its maxContainers is saturated holds the flock for the entire wait — potentially hours
- This starves every other dark-factory daemon on the host from even checking its own count; a daemon with `maxContainers: 5` cannot start a container while a sibling daemon with `maxContainers: 3` waits for a slot, even though slots 4 and 5 are available from its perspective
- The comment at `pkg/processor/processor.go:889` already states the intent — "Acquire container lock only for the check-and-start window, not during prep work above" — but the implementation folds the slot-wait into that window
- This fix moves the poll loop OUTSIDE the lock: acquire → count-check → (slot free: return held lock to caller) OR (slot full: release lock, sleep, retry)
- The check-and-start atomicity is preserved: the lock is held across count-check AND docker-run AND wait-until-running, so two daemons cannot both decide "slot available" at the same effective count
- The caller contract is unchanged — `prepareContainerSlot` still returns a held lock and an idempotent release function; `startContainerLockRelease` still fires after the container is confirmed running
</summary>

<objective>
Eliminate head-of-line blocking between dark-factory daemons on the same host. When one daemon is waiting for a container slot to free up, it must not hold the cross-project flock. Other daemons — including ones with higher maxContainers limits that would see a free slot — must be able to acquire the flock, run their own count check, and proceed independently.

The correctness invariant (no two daemons racing past the count check to start a container simultaneously) must be preserved.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions: errors wrapping with `github.com/bborbe/errors` (no `fmt.Errorf`, no bare `return err`), Ginkgo/Gomega tests, Counterfeiter mocks, `libtime.CurrentDateTimeGetter` for injected time where relevant.

Read these source files in full before editing:

- `pkg/processor/processor.go` — two relevant functions and one caller:
  - `waitForContainerSlot` (line ~719): poll loop that blocks until `CountRunning < maxContainers`. This function today does NOT touch the lock.
  - `prepareContainerSlot` (line ~748): acquires the flock via `p.containerLock.Acquire(ctx)`, then calls `waitForContainerSlot` while holding the flock, then returns the held-lock release function to the caller.
  - `startContainerLockRelease` (line ~774): goroutine that releases the flock after `containerChecker.WaitUntilRunning(ctx, name, 30*time.Second)` succeeds or times out. This runs AFTER the caller has started the container.
  - The single caller of `prepareContainerSlot` is around line 889 (`// Acquire container lock only for the check-and-start window, not during prep work above`). It does: `releaseLock, err := prepareContainerSlot(ctx); … docker run … ; startContainerLockRelease(ctx, containerName, releaseLock)`.

- `pkg/containerlock/containerlock.go` — the `ContainerLock` interface is `Acquire(ctx) error` / `Release(ctx) error`. Backed by `syscall.Flock(LOCK_EX|LOCK_NB)` in a 100ms poll loop in `Acquire`. Release calls `LOCK_UN` and closes the fd. File path is `$HOME/.dark-factory/container.lock`, shared across all daemons on the host.

- `pkg/processor/processor_internal_test.go` (line ~836–921) — existing `stubContainerCounter` and `Describe("waitForContainerSlot", ...)` suite. Reuse the stub pattern.

- `mocks/container-lock.go` — Counterfeiter fake for `ContainerLock`. Use it in the new `prepareContainerSlot` tests to assert Acquire/Release call counts and sequencing.

Bug reproduction (observed on 2026-04-16 in this author's environment):
- Host runs daemons for: `billomat` (global `maxContainers: 3`), `mdm` (global `maxContainers: 3`), `commerce` (global `maxContainers: 3`), `dark-factory` (project `maxContainers: 5`).
- `billomat` enters `waitForContainerSlot` with count=3, max=3 → loops. Holds flock for the entire wait.
- `dark-factory` attempts to start a container — calls `containerLock.Acquire(ctx)` — blocks in the 100 ms poll loop on flock because `billomat` is holding it. Cannot even check its own count. count=3 < 5 for dark-factory, slot IS available, but dark-factory is blocked.
- When one container exits, `billomat` sees count=2, exits the wait, starts its container, eventually releases (after `WaitUntilRunning` or 30s timeout). Now dark-factory's Acquire finally succeeds.
- Net effect: the daemon with the LOWEST effective limit serializes all other daemons behind itself.
</context>

<requirements>

## 1. Reshape `prepareContainerSlot`

Replace the current sequential `Acquire → waitForContainerSlot → return` shape with a retry loop that holds the lock only across the count check:

```go
// prepareContainerSlot acquires the global container lock only for the
// check-and-start window. If the slot is full, it RELEASES the lock and
// sleeps before retrying, so other daemons (possibly with higher limits)
// are not blocked while this daemon waits.
//
// On success, returns with the lock held and an idempotent release function.
// The caller is responsible for starting the container and calling
// startContainerLockRelease, which releases the lock after the container is
// confirmed running.
//
// When p.containerLock is nil, no locking is performed and the only wait is
// the unlocked waitForContainerSlot poll (unchanged behaviour for nil-lock case).
func (p *processor) prepareContainerSlot(ctx context.Context) (func(), error) {
    // Existing nil-lock fast path: no lock, just the existing wait loop.
    if p.containerLock == nil {
        if err := p.waitForContainerSlot(ctx); err != nil {
            return func() {}, errors.Wrap(ctx, err, "wait for container slot")
        }
        return func() {}, nil
    }

    for {
        if err := p.containerLock.Acquire(ctx); err != nil {
            return func() {}, errors.Wrap(ctx, err, "acquire container lock")
        }

        // Idempotent release — the caller MAY call this early, the retry
        // branch below WILL call this before sleeping, and startContainerLockRelease
        // will call it after the container is confirmed running.
        var once sync.Once
        releaseLock := func() {
            once.Do(func() { _ = p.containerLock.Release(ctx) })
        }

        // Lock held — count must be stable for this daemon's decision.
        free, err := p.hasFreeSlot(ctx)
        if err != nil {
            releaseLock()
            return func() {}, errors.Wrap(ctx, err, "check free slot")
        }
        if free {
            // Lock stays held; caller does docker run + startContainerLockRelease.
            return releaseLock, nil
        }

        // No slot — release before sleeping so other daemons can proceed.
        releaseLock()
        slog.Info(
            "waiting for container slot",
            "limit", p.maxContainers,
        )
        select {
        case <-ctx.Done():
            return func() {}, errors.Wrapf(ctx, ctx.Err(), "wait for container slot cancelled")
        case <-time.After(p.containerPollInterval):
        }
    }
}
```

Notes on the shape:

- `hasFreeSlot(ctx)` is a new tiny helper that returns `(bool, error)`:

  ```go
  // hasFreeSlot returns true when maxContainers is unlimited (<=0) or when
  // the current system-wide running count is below maxContainers.
  // On counter error, the behaviour matches waitForContainerSlot's existing
  // tolerance: log a warning and return (true, nil) so the daemon makes forward
  // progress — docker itself will reject a start if resources are truly absent.
  func (p *processor) hasFreeSlot(ctx context.Context) (bool, error) {
      if p.maxContainers <= 0 {
          return true, nil
      }
      count, err := p.containerCounter.CountRunning(ctx)
      if err != nil {
          slog.Warn("failed to count running containers, proceeding anyway", "error", err)
          return true, nil
      }
      return count < p.maxContainers, nil
  }
  ```

- `waitForContainerSlot` is no longer called from the locking path. Keep it exported for the nil-lock path above and for any external caller; its behaviour is unchanged.

- The `slog.Info("waiting for container slot", ...)` line now only fires when we decided to wait — i.e. slot was full. Include `"limit"` (no longer include `"running"` since the count is gone by the time we log from outside `hasFreeSlot`; if you prefer, capture `count` before returning false from `hasFreeSlot` and thread it through, but keep the signature simple and drop `running` from the log line).

## 2. Do NOT change the caller

`prepareContainerSlot` still returns `(func(), error)`. The caller at line ~889 does not change. In particular:

- `startContainerLockRelease(ctx, containerName, releaseLock)` still fires after the container is "running" or 30 s.
- The `docker run` call still happens between `prepareContainerSlot` returning and `startContainerLockRelease` being invoked.
- The comment at line 889 ("Acquire container lock only for the check-and-start window, not during prep work above") now accurately describes the implementation — do not remove it.

## 3. Preserve the correctness invariant

Two daemons MUST NOT both be inside the `if free { return }` branch at the same effective count. The flock guarantees this because:

- `Acquire` → `hasFreeSlot` → `return releaseLock` all happen with the flock held.
- The caller then does `docker run` (still under the flock).
- The release goroutine waits for `WaitUntilRunning` (still under the flock) before releasing.
- So by the time a competitor daemon's `Acquire` succeeds, the newly started container is already counted by `CountRunning`.

Do NOT weaken this — specifically, DO NOT move the `docker run` inside `prepareContainerSlot` (out of scope; would change the caller contract). DO NOT release the lock before the caller invokes `startContainerLockRelease`.

## 4. Tests — `pkg/processor/processor_internal_test.go`

Add a new `Describe("prepareContainerSlot", ...)` block adjacent to the existing `Describe("waitForContainerSlot", ...)` suite. Use the existing `stubContainerCounter` pattern for counts. For the lock, use the existing `mocks.FakeContainerLock` generated by Counterfeiter (import `"github.com/bborbe/dark-factory/mocks"`).

Required cases:

1. **nil-lock fast path, slot free immediately**: `p.containerLock = nil`, counter returns 1 (below max=3). Returns `(nonNil, nil)`. `counter.calls()` is 1.

2. **nil-lock fast path, slot-wait then free**: `p.containerLock = nil`, counter returns 3 on first call, 2 on second. Returns `(nonNil, nil)`. `counter.calls()` is 2. (This is the unchanged behaviour; it proves the nil-lock branch still works.)

3. **lock held, slot free immediately**: `fakeLock := &mocks.FakeContainerLock{}`, counter returns 2 (below max=3). After `prepareContainerSlot` returns, assert:
   - `fakeLock.AcquireCallCount() == 1`
   - `fakeLock.ReleaseCallCount() == 0` (lock still held, caller will release)
   - The returned `release` function is non-nil; calling it once invokes `Release` exactly once; calling it a second time does NOT invoke `Release` a second time (idempotence via `sync.Once`).

4. **lock held, slot full then free — THE KEY TEST**: `fakeLock` as above, counter returns 3 on call 1, 3 on call 2, 2 on call 3. `p.containerPollInterval = 10 * time.Millisecond` to keep the test fast. Assert:
   - `prepareContainerSlot` returns `(nonNil, nil)` eventually.
   - `fakeLock.AcquireCallCount() == 3` (acquired three times — once per count check).
   - `fakeLock.ReleaseCallCount() == 2` (released twice — after each of the two "full" count checks, BEFORE the sleep).
   - `counter.calls() == 3`.
   - The returned release is non-nil and still un-invoked — assert `fakeLock.ReleaseCallCount() == 2` immediately after the return, then call release(), then assert it rose to 3.
   - **Ordering**: the Release calls must happen BEFORE the subsequent Acquire calls, not after. To assert this, instrument the `FakeContainerLock` by using the Counterfeiter `AcquireStub` / `ReleaseStub` to record monotonic event IDs into a shared slice: each Acquire appends `"A"`, each Release appends `"R"`. After the test, assert the recorded sequence is `["A", "R", "A", "R", "A"]` — i.e. every "slot-full" Acquire is followed by a Release before the next Acquire. This is the regression signal that proves the flock is actually released between polls.

5. **ctx cancellation during slot-wait**: `fakeLock` returns successfully for `Acquire`. Counter always returns 3 (full). `p.containerPollInterval = 50 * time.Millisecond`. `cancel()` after ~80 ms. Assert:
   - `prepareContainerSlot` returns `(_, non-nil error)` wrapping `context.Canceled` (or use `errors.Is(err, context.Canceled)`).
   - `fakeLock.ReleaseCallCount()` equals `fakeLock.AcquireCallCount()` at the moment of cancellation — i.e. EVERY acquired lock was released. No leaked lock.

6. **acquire error propagation**: `fakeLock.AcquireReturns(errors.New("flock denied"))`. Counter is not invoked. Assert returned error is non-nil and wraps the flock error; `fakeLock.ReleaseCallCount() == 0`.

7. **counter error is tolerated (logs warn, proceeds)**: `fakeLock` as normal, counter returns `(0, errors.New("docker ls failed"))` on first call. Assert `prepareContainerSlot` returns `(nonNil, nil)` (not an error), `fakeLock.AcquireCallCount() == 1`, `fakeLock.ReleaseCallCount() == 0` (lock still held — we proceeded as if slot was free, matching `waitForContainerSlot`'s existing tolerance).

Use `atomic.Int32` or similar for the counter call tracking; the existing `stubContainerCounter` already does this.

## 5. Preserve existing `waitForContainerSlot` tests

The existing `Describe("waitForContainerSlot", ...)` suite at line ~851 continues to test `waitForContainerSlot` directly. Do not delete or modify those tests — `waitForContainerSlot` is still called (from the nil-lock path in `prepareContainerSlot`) so its contract still matters.

## 6. Do NOT change

- `pkg/containerlock/containerlock.go` — flock logic unchanged.
- `pkg/processor/processor.go`'s caller of `prepareContainerSlot` (around line 889) — no caller-side changes.
- `startContainerLockRelease` — unchanged.
- `pkg/config/loader.go` — out of scope (see prompt 291 for that fix).
- `pkg/factory/factory.go` — wiring unchanged.
- `go.mod` / `go.sum` / `vendor/`.
- Any logic in `handleDirectWorkflow`, `handlePRWorkflow`, `handlePostExecution`, or the executor.

## 7. Verification

Run `make precommit` in `/workspace` — must exit 0. The new tests in requirement 4 are the authoritative regression gate.

Manual reasoning check (document in the prompt log, no automation needed): after the change, a daemon stuck in `waitForContainerSlot` with a full count releases the flock between every poll. Other daemons with different maxContainers limits can acquire the flock, run their own `hasFreeSlot` check against their own limit, and make independent decisions. The correctness invariant (no two daemons both deciding "slot available" simultaneously) still holds because the decision-and-start window is still atomic under the lock.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git.
- Wrap all non-nil errors with `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors`. Never `fmt.Errorf`, never `errors.New` in production code (test helpers may still use `stderrors.New` as the existing tests do).
- Keep error messages lowercase, no file paths, no `%v`.
- No new exported names. `hasFreeSlot` must be unexported.
- Preserve the `sync.Once` idempotence on the release function — the caller may call release more than once (early-exit paths, goroutine race) and Release must only fire once per Acquire.
- The flock MUST be released before the `time.After` sleep in the retry branch. This is the whole point of the fix. A test asserting `ReleaseCallCount > 0` after at least one full-slot iteration is mandatory (requirement 4, case 4).
- Do not introduce timing-sensitive tests beyond what is absolutely needed. Use short `containerPollInterval` (10–50 ms) so tests run fast; never `Sleep` in test bodies except to trigger ctx cancellation.
- Do not touch `go.mod` / `go.sum` / `vendor/`.
- Existing tests must still pass.
</constraints>

<verification>
1. `cd /workspace && make precommit` must exit 0.
2. The new `Describe("prepareContainerSlot", ...)` suite must include case 4 ("lock held, slot full then free") and it must assert `fakeLock.ReleaseCallCount() >= 2` after the retries — this is the authoritative "lock released between polls" regression test.
3. The existing `waitForContainerSlot` suite must still pass untouched.
4. `grep -n "Acquire\|Release" pkg/processor/processor.go` — the Acquire happens once per iteration inside `prepareContainerSlot`, the explicit Release happens once per iteration in the slot-full branch, and no other direct lock-manipulation appears.
</verification>
