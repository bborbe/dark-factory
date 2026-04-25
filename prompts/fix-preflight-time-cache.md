---
status: draft
created: "2026-04-25T13:10:00Z"
---

<summary>
- Preflight cache is keyed by time alone, not by git commit SHA
- The `preflightInterval` becomes a real time-based TTL: a successful preflight is reused for `interval` after it ran, regardless of how many auto-commits the daemon makes between prompts
- Failed preflights are NOT cached — every Check call re-runs the command after a failure so an operator fix is picked up immediately
- The `shaFetcher` machinery is removed from `pkg/preflight/preflight.go` because it is no longer needed
- `docs/configuration.md` is updated to describe the new time-based behavior (replaces the SHA-keyed wording)
- Existing Ginkgo tests are updated to reflect the new contract: same-SHA / different-SHA scenarios collapse to one "within interval / past interval" pair
</summary>

<objective>
Fix `pkg/preflight/preflight.go` so that `preflightInterval` actually saves work across sequential prompts. Today the cache is keyed by `(sha, checkedAt)`; every prompt's auto-commit changes the SHA, invalidating the cache and forcing preflight to re-run before every prompt. Cache by time only — preflight is a daemon-startup security check, not a per-commit gate.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega patterns.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`.

Read these files before editing:
- `pkg/preflight/preflight.go` — the `Checker` implementation, `cacheEntry`, `Check`, `runInContainer`, `getHeadSHA`. The fix lives mostly in `Check()` (lines 82–128).
- `pkg/preflight/preflight_test.go` — existing tests, including `It("reuses cached result within interval for same SHA", ...)` (line 102) and the SHA-advance test (line 124). These need updating.
- `pkg/preflight/export_test.go` — `NewCheckerWithRunner` test helper signature; if `shaFetcher` is removed from production wiring it can also be removed here.
- `docs/configuration.md` — the `Preflight Baseline Check` section (lines 248–266) describes today's SHA-keyed behavior. Must be rewritten to describe time-based caching.

Authoritative intent (from project owner):
> "Preflight runs at daemon startup to detect security vulnerabilities or broken baseline. Once it passes, don't run it again until the daemon restarts or `preflightInterval` elapses."

This means the cache is **time-only**: successful preflight is reused for `interval` regardless of git activity in between.
</context>

<requirements>

## 1. Update `pkg/preflight/preflight.go` — make cache time-only

### 1a. Simplify the cache check in `Check()`

Current code (lines 87–98):
```go
sha, err := c.shaFetcher(ctx)
if err != nil {
    slog.Warn("preflight: could not get HEAD SHA, skipping cache", "error", err)
    sha = ""
}

// Cache hit: same SHA and within interval
if c.cache != nil && sha != "" && c.cache.sha == sha &&
    c.interval > 0 && time.Since(c.cache.checkedAt) < c.interval {
    slog.Debug("preflight: cache hit", "sha", sha[:minLen(sha, 12)], "ok", c.cache.ok)
    return c.cache.ok, nil
}
```

Replace with a time-only cache hit that only honors successful results:

```go
// Cache hit: a successful preflight is reused for `interval` after it ran,
// regardless of git activity. Failed results are not cached — operator fixes
// must be picked up on the next Check call.
if c.cache != nil && c.cache.ok && c.interval > 0 &&
    time.Since(c.cache.checkedAt) < c.interval {
    slog.Debug("preflight: cache hit (time-based)",
        "age", time.Since(c.cache.checkedAt).Round(time.Second),
        "interval", c.interval,
    )
    return true, nil
}
```

### 1b. Remove `sha`, `shaFetcher`, and `getHeadSHA` from the production code path

After 1a the SHA is no longer needed for caching. Drop:

- The `sha` field on `cacheEntry` (the struct can be simplified or kept with only `checkedAt` and `ok`; remove `output` only if no other callers read it)
- The `shaFetcher` field on `checker`
- The `shaFetcherFn` type alias (if it has no remaining consumer)
- The `getHeadSHA` method

The `slog.Info("preflight: running baseline check", ...)` line should drop the `sha` field, e.g.:

```go
slog.Info("preflight: running baseline check", "command", c.command)
```

Same for the post-run success/failure logs — drop the `sha` field.

`truncateSHA` and `minLen` become unused if no other call site references them. Delete both helpers if so. Run `make precommit` to confirm.

### 1c. Only store successful preflight results in the cache

In the post-run block, store the cache entry only when `ok` is true:

```go
output, runErr := c.runner(ctx)
ok := runErr == nil

if ok {
    c.cache = &cacheEntry{
        checkedAt: time.Now(),
        ok:        true,
    }
    slog.Info("preflight: baseline check passed")
    return true, nil
}

// Failure: do not cache — operator may fix the issue between calls,
// and we want the next Check to re-run the command.
slog.Error("preflight: baseline check FAILED — prompts will not start until baseline is fixed",
    "command", c.command,
    "output", output,
    "error", runErr,
)
_ = c.notifier.Notify(ctx, notifier.Event{
    ProjectName: c.projectName,
    EventType:   "preflight_failed",
})
return false, nil
```

### 1d. Update `NewChecker` signature

If the production `NewChecker` no longer needs `projectRoot` for SHA fetching but still uses it for `runInContainer.Dir`, keep it. If `runInContainer` is the only remaining user, leave the signature unchanged.

If a `shaFetcher` parameter is exposed via `NewCheckerWithRunner` in `export_test.go`, remove that parameter too. Updating callers in tests is part of step 2.

## 2. Update `pkg/preflight/preflight_test.go`

### 2a. Replace SHA-keyed cache tests with time-keyed equivalents

Tests to update (do NOT delete existing test names — rewrite their bodies):

- `It("reuses cached result within interval for same SHA", ...)` (line 102) → rename to `It("reuses cached result within interval", ...)`. Body: call Check twice with a 1-hour interval and a small synthetic clock advance via `time.Sleep` is not acceptable — use a configurable clock. If no clock injection exists, set `interval = 1 * time.Hour` and call Check twice; assert runner called once.

- The `It("re-runs when SHA advances", ...)` test (line 124) is no longer meaningful. Replace with `It("re-runs after interval elapses", ...)`. Body: pick a tiny interval (`10 * time.Millisecond`), call Check, sleep 50ms, call Check again, assert runner called twice.

### 2b. Add new test: failures are NOT cached

```go
It("does not cache a failed preflight — next call re-runs the command", func() {
    callCount := 0
    failingRunner := func(ctx context.Context) (string, error) {
        callCount++
        return "boom", errors.Wrap(ctx, fmt.Errorf("exit 1"), "preflight failed")
    }
    ch := preflight.NewCheckerWithRunner(
        "make precommit",
        1*time.Hour, // huge interval — would cache forever if failures cached
        fakeNotifier,
        "proj",
        failingRunner,
    )

    ok1, _ := ch.Check(ctx)
    ok2, _ := ch.Check(ctx)
    Expect(ok1).To(BeFalse())
    Expect(ok2).To(BeFalse())
    Expect(callCount).To(Equal(2)) // both calls re-ran the command
})
```

### 2c. Remove SHA fetch tests if they exist

Tests like `It("re-runs when SHA fetcher returns empty (cache miss)", ...)` (line 164) are no longer relevant. Delete them.

### 2d. Drop `truncateSHA` tests (if `truncateSHA` is removed)

The `Describe("truncateSHA", ...)` block at line 19–31 should be removed if `truncateSHA` was removed in step 1b.

## 3. Update `pkg/preflight/export_test.go`

If `NewCheckerWithRunner` accepted a `shaFetcher` argument, remove it from the helper signature. Update all test call sites accordingly.

If the helper still needs to exist at all (it does — to inject the runner), keep it but simplify.

## 4. Update `docs/configuration.md`

Replace the current `Preflight Baseline Check` section's caching paragraph (lines 262–266) with the new behavior:

**Old text:**
> `preflightInterval` ... How long a cached green baseline result stays valid for the same git commit SHA. When the SHA advances or the interval elapses, preflight re-runs.
>
> **Caching:** Preflight runs at most once per git commit SHA within `preflightInterval`. Multiple queued prompts on the same baseline SHA reuse the cached result — no wasted container time.

**New text:**
> `preflightInterval` ... How long a successful preflight result is cached. After the daemon runs preflight once and it passes, prompts within the interval reuse that result — git commits between prompts do NOT invalidate the cache. Re-runs happen when the interval elapses, when the daemon restarts, or after a failed preflight (failures are never cached, so an operator fix is picked up on the next prompt).
>
> **Caching:** Preflight runs at most once per `preflightInterval` after a successful check. Sequential prompts within the interval reuse the cached result without re-running the command.

Verify the exact line numbers by reading `docs/configuration.md` first — they may have shifted.

## 5. CHANGELOG entry

Append to `## Unreleased` in `CHANGELOG.md`:

```
- fix: preflight cache is now time-based instead of SHA-based — sequential prompts within preflightInterval reuse the cached green result, saving ~1 minute per prompt; failed preflights are not cached so operator fixes are picked up immediately
```

## 6. Run verification

```bash
cd /workspace && make precommit
```

Must exit 0.

</requirements>

<constraints>
- Do NOT commit — dark-factory handles git
- Do not touch `go.mod` / `go.sum` / `vendor/`
- Frontmatter / YAML config wire format: `preflightInterval` field name unchanged. The semantic shift is internal.
- Failed preflights MUST NOT be cached. Every failure path returns `false` and leaves the cache untouched (either nil or whatever previous successful entry existed — but the cache check requires `ok == true`, so a stale success would also expire on time alone).
- Use `errors.Wrap` / `errors.Wrapf` from `github.com/bborbe/errors` for any new error construction
- External test packages (`package preflight_test`)
- Coverage ≥80% for `pkg/preflight/`
- Do NOT change daemon, processor, or runner code — preflight cache lives entirely inside `pkg/preflight/`
</constraints>

<verification>
Run `make precommit` in `/workspace` — must exit 0.

Spot checks:

```bash
cd /workspace

# SHA references gone from preflight production code
! grep -n "shaFetcher\|getHeadSHA\|c.cache.sha" pkg/preflight/preflight.go

# Cache check is time-only and ok-gated
grep -n "c.cache.ok && c.interval" pkg/preflight/preflight.go

# Doc updated
grep -n "time-based\|successful preflight result is cached" docs/configuration.md
```
</verification>
