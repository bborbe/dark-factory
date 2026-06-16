---
status: approved
spec: [096-healthcheck-on-daemon-startup]
created: "2026-06-16T20:10:00Z"
queued: "2026-06-16T20:22:17Z"
branch: dark-factory/healthcheck-on-daemon-startup
---

<summary>
- Builds the daemon healthcheck startup gate as a self-contained, unit-testable unit (no daemon wiring yet — that is prompt 3).
- The gate runs the existing healthcheck probe sequence once and reports pass/fail; on pass it records a success in an on-disk cache so a restart within the cache window skips the probes entirely.
- The cache stores only successes — a failed gate is never cached, so an operator fix (image rebuild, token rotation) is always re-checked on the next start.
- The cache file is keyed per project + image + interval, so two different repos sharing a cache directory never collide.
- A corrupted, unreadable, or future-dated cache file is treated as a miss: the gate re-runs and logs a single warning, never crashes.
- On gate failure the daemon-facing path emits a category-naming terminal error and fires the same notification the preflight terminal-failure path uses.
- Emits the operator-facing log lines the spec's acceptance criteria grep for: gate starting, gate ok (with elapsed), cache hit, cache unreadable.
</summary>

<objective>
Create a new `pkg/healthcheckgate` package providing a `Gate` that runs the existing healthcheck probe sequence at daemon startup, caches success-only on the host filesystem keyed by SHA256 of `<containerImage>:<projectName>:<intervalSeconds>`, treats corrupt/future-dated cache as a miss, and surfaces a terminal category-naming error + notification on failure. No daemon wiring in this prompt — the gate is constructed and unit-tested in isolation.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions.

Read these files fully before editing:
- `/workspace/pkg/preflight/preflight.go` — mirror its shape: public `Checker` interface, private `checker` struct, `NewChecker(...)`, success-only caching, failure fires `notifier.Notify`, NEVER propagates the broken-baseline as a Go error from the cache layer. The gate in this prompt is the same shape but (a) caches to a host file instead of in-memory, and (b) returns a category-naming error on failure (the daemon converts it to a terminal exit).
- `/workspace/pkg/cmd/healthcheck.go` — `HealthcheckCommand` interface: `Run(ctx context.Context, args []string) error`. Its `Run` iterates probes fail-fast and `errors.Wrapf(ctx, err, "healthcheck probe %q failed", p.Name())` on the first failure. The gate will hold a `HealthcheckCommand` and call `.Run(ctx, []string{})`.
- `/workspace/pkg/cmd/healthcheck/probes.go` — the `Probe` interface (`Name() string`, `Run(ctx) error`) and `ProbeLaunchConfig`. You do NOT rebuild probes here; the factory (prompt 3) passes a pre-built `HealthcheckCommand` in.
- `/workspace/pkg/factory/factory.go` — `CreateHealthcheckCommand(ctx, cfg, currentDateTimeGetter)` already builds the seven-probe `cmd.HealthcheckCommand` in fixed order (docker, image, boot, claude, mount, gh, notifications), gating gh on `cfg.PR` and notifications on `healthcheck.NotificationsConfigured(cfg)`. Prompt 3 reuses THIS function — do not duplicate probe construction.
- `/workspace/pkg/notifier/notifier.go` — `Event{ProjectName, EventType}`; existing EventType strings include `"preflight_failed"`. Use a new `"healthcheck_failed"` EventType for the gate.
- `/workspace/pkg/globalconfig/globalconfig.go` — shows the `~/.dark-factory/` host-state-dir convention (`filepath.Join(home, ".dark-factory", ...)`). The gate cache lives under the same root.

Coding-plugin docs (in-container paths — read them):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — public interface + private struct + `New*` constructor + counterfeiter annotation.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `errors.Wrap(ctx,...)` / `errors.Errorf(ctx,...)`, never `fmt.Errorf`, never `context.Background()` in `pkg/`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega, counterfeiter mocks, external `_test` package, ≥80% coverage.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-security-linting.md` — file perms: write the cache file `0600`, mkdir the cache dir `0700`/`0750`; add `#nosec` with a reason only if a linter flags an unavoidable case.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-time-injection.md` — inject `libtime.CurrentDateTimeGetter` for "now"; in tests use `SetNow()`.

Verified facts (quoted from current source):
- `cmd.HealthcheckCommand` interface: `Run(ctx context.Context, args []string) error` (from `pkg/cmd/healthcheck.go`).
- `preflight.NewChecker` signature: `NewChecker(command string, interval libtime.Duration, projectRoot string, n notifier.Notifier, projectName string, currentDateTimeGetter libtime.CurrentDateTimeGetter) Checker`.
- `notifier.Event` struct: `{ ProjectName string; EventType string; PromptName string }` (PromptName optional).
- `libtime.CurrentDateTimeGetter.Now()` returns `libtime.DateTime`; convert via `time.Time(dt)`.
</context>

<requirements>
1. Create a new package directory `/workspace/pkg/healthcheckgate/`. Add `doc.go` with the standard license header and a package comment describing: "Package healthcheckgate runs the existing healthcheck probe sequence once at daemon startup and caches success-only on the host filesystem so a restart within the cache window skips the probes."

2. Define the public interface and constructor in `pkg/healthcheckgate/gate.go`. The `Gate` is the daemon-facing surface; its single method returns nil on pass (probes green or cache hit or disabled), and a category-naming error on failure (the daemon converts this to a terminal exit):
   ```go
   //counterfeiter:generate -o ../../mocks/healthcheck-gate.go --fake-name HealthcheckGate . Gate

   // Gate runs the healthcheck probe sequence at daemon startup with success-only caching.
   type Gate interface {
       // Check runs the gate. Returns nil when the probes pass, when a fresh
       // cached success exists, or when the gate is disabled. Returns a
       // category-naming error when the probes fail (caller treats as terminal).
       Check(ctx context.Context) error
   }
   ```
   The constructor:
   ```go
   func NewGate(
       enabled bool,
       skip bool,
       interval time.Duration,
       healthcheck HealthcheckCommand,
       cache Cache,
       n notifier.Notifier,
       projectName string,
       currentDateTimeGetter libtime.CurrentDateTimeGetter,
   ) Gate
   ```
   where `HealthcheckCommand` is a 1-method interface declared locally in this package to avoid importing `pkg/cmd` (keeps the dependency graph clean and the mock small):
   ```go
   //counterfeiter:generate -o ../../mocks/healthcheck-gate-command.go --fake-name HealthcheckGateCommand . HealthcheckCommand

   // HealthcheckCommand runs the probe sequence. Satisfied by cmd.HealthcheckCommand.
   type HealthcheckCommand interface {
       Run(ctx context.Context, args []string) error
   }
   ```
   `cmd.HealthcheckCommand` (signature `Run(ctx, []string) error`) structurally satisfies this — prompt 3 passes the concrete one in.

3. Implement `Check(ctx)` in the private `gate` struct with this exact control flow (emit the slog lines verbatim — the spec ACs grep for these strings):
   - If `!enabled`: `slog.Info("healthcheck gate disabled")` and return nil. (Do NOT emit `healthcheck startup gate starting`.)
   - Else if `skip`: `slog.Info("healthcheck skipped via --skip-healthcheck")` and return nil. NO cache read, NO cache write.
   - Else: attempt a cache read (step 4). On a valid fresh hit: `slog.Info("healthcheck cache hit, skipping")` and return nil (do NOT emit `healthcheck startup gate starting`).
   - Else (miss / stale / unreadable / future-dated): `slog.Info("healthcheck startup gate starting")`, record start via `currentDateTimeGetter.Now()`, call `healthcheck.Run(ctx, []string{})`.
     - On success: compute `elapsed` from `currentDateTimeGetter.Now()` minus the start `time.Time` (so tests using a stubbed clock control it), write a success record to the cache (step 4), `slog.Info("healthcheck startup gate ok", "elapsed", elapsed)`, return nil. The `elapsed` field MUST be a `time.Duration` value (slog renders units, e.g. `elapsed=12ms` / `elapsed=1.2s`). **Do NOT call `time.Now()` directly** — `pkg/` code is forbidden from touching the wall clock per `go-time-injection.md`. In tests, stub the getter to return `start + N ms` on the second call (the `libtime.CurrentDateTimeGetter` mock's `NowReturnsOnCall(i, ...)` is the standard pattern).
     - On failure: do NOT write the cache. Fire notification:
       ```go
       _ = g.notifier.Notify(ctx, notifier.Event{ProjectName: g.projectName, EventType: "healthcheck_failed"})
       ```
       Return a category-naming error. The underlying `healthcheck.Run` error is already wrapped `healthcheck probe %q failed`; wrap it once more so the daemon-facing message starts with `healthcheck failed:`:
       ```go
       return errors.Wrap(ctx, err, "healthcheck failed")
       ```
       (`errors.Wrap` prepends `healthcheck failed: ` to the probe-category message, satisfying the spec's `^healthcheck failed: .+$` stderr shape and the category-naming requirement.)

4. Implement the on-disk cache in `pkg/healthcheckgate/cache.go`. Define:
   ```go
   //counterfeiter:generate -o ../../mocks/healthcheck-gate-cache.go --fake-name HealthcheckGateCache . Cache

   // Cache stores success-only healthcheck results on the host filesystem.
   type Cache interface {
       // Fresh reports whether a cached success exists and is younger than interval.
       // A missing, unreadable, malformed, or future-dated cache file is reported as
       // not-fresh (and, for unreadable/malformed/future cases, logs a single warning).
       Fresh(ctx context.Context, key string, interval time.Duration, now time.Time) bool
       // Write records a success at `now` under `key`. Errors are logged, not returned —
       // a cache-write failure must not abort daemon startup.
       Write(ctx context.Context, key string, now time.Time)
   }

   func NewFileCache() Cache
   ```
   - Cache directory: `filepath.Join(home, ".dark-factory", "healthcheck-cache")` where `home, _ := os.UserHomeDir()`. Create it with `os.MkdirAll(dir, 0700)` on `Write`.
   - Cache filename: `healthcheck-<key>.json` where `<key>` is the SHA256 hex digest computed by the helper in step 5. (Filename contains only the hex digest, so it is filesystem-safe.)
   - Stored value: JSON `{"checkedAt": "<RFC3339Nano timestamp>", "success": true}`. Only success records are ever written (the gate never calls `Write` on failure).
   - `Fresh`:
     - Read the file. `os.IsNotExist` → return false silently (clean miss).
     - Any other read error OR JSON unmarshal error → `slog.Warn("healthcheck cache unreadable, re-running")` and return false.
     - Parse `checkedAt`. If `now.Before(checkedAt)` (cached timestamp in the future → clock skew) → `slog.Warn("healthcheck cache timestamp in future, re-running")` and return false.
     - If `now.Sub(checkedAt) < interval` → return true (fresh). Else return false (stale).
     - Guard: if `interval <= 0`, return false (no caching).
   - `Write`: `MkdirAll` the dir (log+return on error), marshal the record, `os.WriteFile(path, data, 0600)` (log on error). Never panic, never return.

5. Add the cache-key helper in `pkg/healthcheckgate/cache.go` (exported so prompt 3 / tests can call it):
   ```go
   // CacheKey returns the SHA256 hex digest of "<containerImage>:<projectName>:<intervalSeconds>".
   // intervalSeconds is the interval expressed in whole seconds. Stable across runs for a
   // given (image, project, interval) triple so different repos never collide.
   func CacheKey(containerImage, projectName string, interval time.Duration) string {
       raw := fmt.Sprintf("%s:%s:%d", containerImage, projectName, int64(interval/time.Second))
       sum := sha256.Sum256([]byte(raw))
       return hex.EncodeToString(sum[:])
   }
   ```
   Imports: `crypto/sha256`, `encoding/hex`, `fmt`.

6. Run `make generate` (or the project's counterfeiter target) so the three mocks are generated under `/workspace/mocks/`. If the project uses `go generate ./...`, run that. Confirm `ls /workspace/mocks/ | grep -i 'healthcheck-gate'` shows the generated fakes. If counterfeiter is unavailable, hand-write minimal fakes is NOT acceptable — instead document the blocker in `## Improvements` and STOP with `status: failed`.

7. Tests (external `package healthcheckgate_test`, Ginkgo/Gomega, ≥80% coverage) in `pkg/healthcheckgate/`:
   - `gate_test.go`:
     - disabled (`enabled=false`) → `Check` returns nil; the fake `HealthcheckCommand.RunCallCount()` is 0; the fake `Cache.FreshCallCount()` is 0.
     - skip (`skip=true`) → returns nil; `Run` not called; `Cache.FreshCallCount()` and `Cache.WriteCallCount()` both 0.
     - cache hit (`Cache.FreshReturns(true)`) → returns nil; `Run` not called; `Write` not called.
     - cache miss + probes pass (`Fresh` returns false, `Run` returns nil) → returns nil; `Run` called once with empty args; `Cache.WriteCallCount()` == 1.
     - cache miss + probes fail (`Run` returns a non-nil error) → returns a non-nil error whose `.Error()` has prefix `healthcheck failed:`; `Cache.WriteCallCount()` == 0; `notifier.Notify` fired once with `EventType == "healthcheck_failed"` (use a counterfeiter notifier fake or a tiny recording fake).
   - `cache_test.go` (the boundary/round-trip test required by the spec — exercise the real filesystem cache through `Write` then `Fresh`):
     - Point the cache at a temp dir (see step 8). `Write` then `Fresh` within interval → true. `Fresh` after interval elapsed (advance `now`) → false. Missing file → false. Corrupt file (write `"{garbage"` to the expected path) → false + the warning path. Future timestamp (write a record with `checkedAt` ahead of `now`) → false. `interval <= 0` → false.
     - `CacheKey` is deterministic: same inputs → same digest; differing image/project/interval → different digests.

8. To make `NewFileCache` testable without touching the real `$HOME`, **inject the cache root as a constructor parameter** — do NOT use a package-level `var cacheRoot` overridden via `export_test.go` (that pattern is the "Test-Only Package-Level Mutable State" anti-pattern called out in `go-composition.md`). Change the constructor:
   ```go
   // NewFileCache returns a Cache rooted at the given directory.
   // The factory (prompt 3) passes filepath.Join(home, ".dark-factory", "healthcheck-cache").
   // Tests pass GinkgoT().TempDir().
   func NewFileCache(root string) Cache
   ```
   The cache implementation joins `root` with `healthcheck-<key>.json`. No globals, no export_test hooks, no AfterEach restore. The factory (prompt 3) is responsible for resolving `$HOME`; the cache only knows about its injected root.

9. Do NOT wire the gate into the daemon, the factory, or `main.go` in this prompt — that is prompt 3. Do NOT modify `pkg/preflight`, `pkg/runner/runner.go`, or `pkg/factory/factory.go` here.
</requirements>

<constraints>
- Copied from spec: Do NOT cache failed healthcheck results — operator action requires re-running the gate on next start.
- Copied from spec: Reuse the existing healthcheck probe sequence — no parallel probe implementation. (This prompt holds a `HealthcheckCommand` interface; prompt 3 injects the factory-built one. Do NOT re-declare probes here.)
- Copied from spec: Cache keyed by SHA of `<containerImage>:<projectName>:<healthcheckInterval-seconds>`, success records only, on host filesystem.
- Copied from spec: Cache file corrupted / unreadable → treat as cache miss; re-run; log a single warning `healthcheck cache unreadable, re-running`.
- Copied from spec: Clock skew makes cached timestamp appear in the future → treat as cache miss; re-run; log a single warning.
- Copied from spec: Failure semantics mirror preflight terminal failure — category-naming error + notification; never cached.
- Use `errors.Wrap(ctx,...)` / `errors.Errorf(ctx,...)`. Never `fmt.Errorf`, never `context.Background()` in `pkg/`.
- Cache file perms `0600`; cache dir perms `0700`. Add `#nosec` with a reason only if a linter demands it.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass.
</constraints>

<verification>
Run in `/workspace`:
```bash
make generate
ls /workspace/mocks/ | grep -i 'healthcheck-gate'
make precommit
```
(Repo is `-mod=mod`, has no `vendor/`; `make precommit` already covers tests, lint, vet, coverage. Never pass `-mod=vendor`.)
- `make precommit` must exit 0.
- `pkg/healthcheckgate` coverage ≥80%.
- The cache round-trip test (`Write` then `Fresh`) must pass against a real temp-dir filesystem.
- Confirm gate-failure path returns an error with prefix `healthcheck failed:` and fires the `healthcheck_failed` notification.
</verification>
