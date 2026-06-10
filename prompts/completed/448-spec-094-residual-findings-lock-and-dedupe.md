---
status: completed
spec: [094-bug-spec-092-contract-violations]
summary: Per-prompt file lock wired into both the reject command and the queue-advance path; project-lock-timeout reason token now live; blocked-log dedupe regression fixed via per-key map; vacuous concurrency test rewritten as a real contention test with starter-lock fairness; legacy read-compat test extended to a full round-trip; PROMPT-side scenario checklist corrected to camelCase; CHANGELOG entry added.
container: dark-factory-spec092-fix-exec-448-spec-094-residual-findings-lock-and-dedupe
dark-factory-version: v0.177.1
created: "2026-06-10T16:30:00Z"
queued: "2026-06-10T15:59:36Z"
started: "2026-06-10T16:00:56Z"
completed: "2026-06-10T18:25:00Z"
---

<summary>

- Rejecting a prompt and the daemon's queue-advance loop now serialize on a real per-prompt lock, so the two can never write the same prompt file at the same time. Whoever loses the race re-reads the file after acquiring the lock and backs off cleanly instead of corrupting it.
- The advance loop and the reject command both emit a `lock acquired` log line, fulfilling the spec-092 acceptance criterion that required visible wait-for-lock then post-lock re-read evidence.
- The dead `project-lock-timeout` blocked-reason token is now wired: when the advance loop cannot get the lock in time, it surfaces that reason instead of silently stalling.
- The "blocked prompt" log dedupe is fixed: previously two blocked specs would alternate and both log on every poll forever; now each distinct blocked state logs once and re-logs only when its blocker changes or resolves.
- The previously-vacuous concurrency test is replaced with a real contention test: a real reject racing a real advance on a real on-disk prompt fixture, asserting the file ends in exactly one place, parses cleanly, and both sides logged that they took the lock.
- An operator-checklist line in the reject-cascade scenario is corrected to the current camelCase frontmatter key for rejected prompts.
- The legacy-key read-compat test now proves a full round-trip: after save, the new key is present on disk and the old key is gone.

</summary>

<objective>
Spec 092's acceptance criterion "Concurrent reject + advance lock semantics" was declared satisfied by a test that never exercised the lock (mocked scanner, empty queue, an outcome regex that accepted everything). Implement the lock semantics FOR REAL in both the reject command and the queue-advance path, wire the dead `ReasonProjectLockTimeout` token, fix a log-dedupe regression that re-logs blocked specs every poll cycle, and replace the vacuous test with a real contention test — plus two small doc/test corrections. This closes the four residual findings from the local PR review of branch `feature/spec-092-remediation`.
</objective>

<context>
Read `/workspace/CLAUDE.md` first for project conventions.

Read these files end-to-end before editing:

- `/workspace/specs/in-progress/092-daemon-blocked-queue-ux.md` — the originating spec; the AC at line 86 ("Concurrent reject + advance lock semantics") is the contract: exactly one final on-disk state, advance loop log shows wait-for-lock then post-lock re-read, evidence is a log grep for `lock acquired`.
- `/workspace/pkg/lock/filelock.go` — the `FileLock` primitive. `NewFileLock(path string) FileLock`; methods `Acquire(ctx context.Context, timeout time.Duration) error` and `Release(ctx context.Context) error`. The lock file is `<path>.lock`. `pkg/lock` has no internal dependencies, so both `pkg/cmd` and `pkg/queuescanner` may import it with no cycle.
- `/workspace/pkg/doctor/fix_sweep.go` — the in-repo precedent for "acquire per-file lock, then mutate". Note the shape: `fl := f.deps.FileLockFactory(specPath)`; `fl.Acquire(ctx, opts.FileLockTimeout)`; on failure append a FailedFix and return; `defer releaseLock(ctx, fl, specPath)`. Follow this acquire/defer-release ordering.
- `/workspace/pkg/doctor/fixer.go` — `FileLockTimeout` defaults to `5 * time.Second` when zero (line ~116); `FileLockFactory func(path string) lock.FileLock` is a settable dependency defaulting to `lock.NewFileLock`. This is the dependency-injection shape to mirror.
- `/workspace/pkg/cmd/reject.go` — `NewRejectCommand(inboxDir, inProgressDir, rejectedDir string, promptManager PromptManager) RejectCommand` and the `rejectCommand.rejectByID(ctx, id, reason)` method. The `Load` → status guard → `StampRejectedWithOriginal` → `Save` → `os.Rename` sequence is what must move under the lock.
- `/workspace/pkg/cmd/prompt_manager.go` — the cmd-local `PromptManager` interface.
- `/workspace/pkg/queuescanner/scanner.go` — `NewScanner(promptManager, promptProcessor, failureHandler, queueDir) Scanner`; `processSingleQueued` is where a candidate is selected (`pr = candidate; break`) and handed to `s.promptProcessor.ProcessPrompt(ctx, pr)`. `logBlockedOnce` and the single-slot `s.lastBlockedMsg` field live here. `s.skippedPrompts map[string]libtime.DateTime` is the existing per-key map pattern to mirror for the dedupe fix. `ReasonProjectLockTimeout` is referenced from `pkg/prompt`.
- `/workspace/pkg/prompt/prompt.go` — `ReasonProjectLockTimeout = "project-lock-timeout"` (in the Reason-tokens const block ~line 1007); `func NewManager(inboxDir, inProgressDir, completedDir, cancelledDir string, mover FileMover, currentDateTimeGetter libtime.CurrentDateTimeGetter) *Manager`; `PromptFile.StampRejectedWithOriginal(reason, originalStatus string)`, `PromptFile.Save(ctx) error`. `Frontmatter.RejectedReason` is the typed field; on disk new writes use `rejectedReason:` (camelCase), legacy files use `rejected_reason:`.
- `/workspace/pkg/queuescanner/scanner_test.go` — imports already include `bytes`, `sync`, `runtime`, `pkg/cmd`, `pkg/lock` (as `lockpkg`), `pkg/prompt`. The slog-capture idiom is at ~line 593 (`var logBuf bytes.Buffer; original := slog.Default(); slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, nil))); defer slog.SetDefault(original)`). The `Describe("ConcurrentRejectAndAdvance", ...)` block (~line 827) is gated on `runtime.GOOS != "linux" && runtime.GOOS != "darwin"` and holds the vacuous test "serializes a real reject against a real scanner advance on one prompt fixture" (~line 871) — THIS is the test to rewrite. The FileLock-primitive test "serializes reject and advance via project lock — no double-write" (~line 831) has a misleading comment ("It does not invoke the reject command directly") — fix or remove the comment if superseded.
- `/workspace/pkg/cmd/reject_test.go` — the read-compat test "loads a legacy rejected_reason key into the typed RejectedReason field" (~line 249) only Loads; extend it to a full round-trip. The setup (`BeforeEach` ~line 29) builds `rejectCmd` with temp dirs and `prompt.NewManager(...)`.
- `/workspace/scenarios/011-reject-spec-cascade.md` — line 91 is SPEC frontmatter (keep `rejected_reason:` — `pkg/spec/spec.go` deliberately keeps the snake_case key); line 102 is PROMPT frontmatter (must become `rejectedReason:`).
- `/workspace/pkg/git/root_test.go` — the `Skip("no resolvable git repo from source tree (hideGit container)")` pattern (~line 44). Any new test that would need to resolve the real repo root must Skip the same way. The tests in this prompt use only `os.MkdirTemp` paths and do NOT resolve the repo root, so they do not need the Skip — but do not introduce any repo-root resolution.

Call sites that MUST be updated when constructor signatures change (verified by grep):
- `NewScanner`: `/workspace/pkg/factory/factory.go:940`; `/workspace/pkg/queuescanner/scanner_test.go:69`, `:765`, `:896`; `/workspace/pkg/processor/processor_test.go:108`; `/workspace/pkg/processor/processor_cancel_test.go:69`; `/workspace/pkg/processor/processor_retry_test.go:665`.
- `NewRejectCommand`: `/workspace/pkg/factory/factory.go:1392` (`CreateRejectCommand`); `/workspace/pkg/cmd/reject_test.go:40`; `/workspace/pkg/queuescanner/scanner_test.go:915`.

</context>

<requirements>

## 1. Inject a FileLock factory into both the reject command and the scanner

Mirror the doctor package's dependency-injection shape (`pkg/doctor/fixer.go`): a settable `FileLockFactory func(path string) lock.FileLock` that defaults to `lock.NewFileLock`, plus a lock-acquire timeout that defaults to `5 * time.Second` when zero.

1.1. In `/workspace/pkg/cmd/reject.go`: add a `fileLockFactory func(path string) lock.FileLock` (import `github.com/bborbe/dark-factory/pkg/lock`) and a `lockTimeout time.Duration` to the `rejectCommand` struct. Add them as parameters to `NewRejectCommand`. The constructor must default a nil factory to `lock.NewFileLock` and a zero timeout to `5 * time.Second`, so existing simple callers can pass `nil, 0`. Decide the exact parameter order and document it in the constructor doc comment.

1.2. In `/workspace/pkg/queuescanner/scanner.go`: add the same `fileLockFactory func(path string) lock.FileLock` and `lockTimeout time.Duration` to the `scanner` struct, add them as parameters to `NewScanner`, defaulting nil → `lock.NewFileLock` and zero → `5 * time.Second`.

1.3. Update every call site listed in `<context>`. Production call sites in `/workspace/pkg/factory/factory.go` should pass `lock.NewFileLock` and a zero timeout (so the default applies) — or the factory's existing lock factory if one is already in scope; check the file. Test call sites that do not care about locking pass `nil, 0`. Do NOT skip any call site — a missed one breaks the build.

## 2. Acquire the per-prompt lock in the reject path (Finding 1, production)

In `rejectCommand.rejectByID` (`/workspace/pkg/cmd/reject.go`), after `FindPromptFileInDirs` resolves `path` and BEFORE `r.promptManager.Load(ctx, path)`:

2.1. Create the lock with `r.fileLockFactory(path)` and `Acquire(ctx, r.lockTimeout)`. On acquire failure, return a wrapped error using the project's `github.com/bborbe/errors` idiom (`errors.Wrap`/`errors.Errorf` — see the existing returns in this method). `defer` the `Release(ctx)` (follow `releaseLock`'s ignore-error-on-defer shape in `pkg/doctor` if you want, or release inline; the lock file is per-path).

2.2. After a successful acquire, emit `slog.Info("lock acquired", "file", filepath.Base(path))` (the spec-092 evidence grep is for `lock acquired`).

2.3. The existing `Load` immediately after the acquire IS the post-lock re-read: the already-rejected guard (`status == prompt.RejectedPromptStatus`) and the rejectable guard then run on fresh on-disk state. No new re-read code is needed — but confirm the `Load` happens AFTER the acquire, not before.

## 3. Acquire the per-prompt lock in the queue-advance path (Finding 1, production)

In `scanner.processSingleQueued` (`/workspace/pkg/queuescanner/scanner.go`), the selected-candidate path is where `pr = candidate; break` chooses a winner and then `s.promptProcessor.ProcessPrompt(ctx, pr)` is invoked. Wrap the candidate handoff with the same per-prompt FileLock:

3.1. After a candidate is selected and before handing it to the processor, create `s.fileLockFactory(pr.Path)` and `Acquire(ctx, s.lockTimeout)`.

3.2. On acquire timeout/failure: do NOT treat as fatal. Log the blocked state via the existing `logBlockedOnce(ctx, pr, specID, prompt.ReasonProjectLockTimeout, "")` path (this wires the currently-dead `ReasonProjectLockTimeout` token — import is already present via `pkg/prompt`), then return `(false, nil)` so the loop re-polls on the next cycle. `specID` for the selected candidate must be available — if the current control flow loses it after the `break`, capture it into a variable in the candidate loop so it is in scope here.

3.3. On successful acquire: emit `slog.Info("lock acquired", "file", filepath.Base(pr.Path))`, then perform a post-lock re-read of the prompt via `s.promptManager.Load(ctx, pr.Path)`. If the re-read shows the status is no longer advanceable (e.g. it is now `prompt.RejectedPromptStatus`, or otherwise not in a queued/approved pre-execution state), log that the candidate was skipped post-lock and `continue`/return-to-repoll (do NOT process it). Use the existing `prompt.PromptStatus` / status helpers — check how `rejectByID` and `autoSetQueuedStatus` classify statuses for the canonical set of "still advanceable" states. If the re-read confirms the prompt is still advanceable, hand it to `s.promptProcessor.ProcessPrompt(ctx, pr)` as today.

3.4. `defer` (or otherwise guarantee) `Release(ctx)` around the processor handoff so the lock is released whether processing succeeds, fails, or the candidate is skipped post-lock. Be careful with the loop structure: the lock must be released before the next poll iteration, not held across `ScanAndProcess`'s outer `for`.

## 4. Fix the blocked-log dedupe regression (Finding 2)

The single-slot `s.lastBlockedMsg string` cannot dedupe more than one blocked spec: with the per-spec `continue`, two blocked specs alternate keys (A, C, A, C…) and BOTH re-log every poll cycle; additionally `s.lastBlockedMsg = ""` reset after a found prompt re-triggers the blocked line per processed prompt.

4.1. Replace `lastBlockedMsg string` with a `map[string]struct{}` (or `map[string]bool`) keyed by the same composite key `logBlockedOnce` already builds (`file|spec|reason|missing`). Initialize it in `NewScanner` next to `skippedPrompts` (mirror that field's map-init pattern).

4.2. `logBlockedOnce` must emit the Info line only when the key is absent from the map, then record the key. A distinct blocked state logs once; it re-logs only when its composite key changes (i.e. the blocker changed).

4.3. When a blocker resolves (a previously-blocked candidate becomes advanceable and is processed), clear that candidate's entry from the map so a future re-block logs again. Do NOT wipe the entire map on every found prompt — that reintroduces the per-processed-prompt re-log bug. Clear only the resolved key(s). Remove the `s.lastBlockedMsg = ""` blanket reset.

4.4. Net behavior: Info-level "prompt blocked" fires once per distinct blocked state, not once per poll cycle, and not once per processed prompt.

## 5. Rewrite the vacuous concurrency test into a real contention test (Finding 1, test)

In `/workspace/pkg/queuescanner/scanner_test.go`, the `It("serializes a real reject against a real scanner advance on one prompt fixture", ...)` inside `Describe("ConcurrentRejectAndAdvance", ...)` (~line 871) is vacuous: the scanner side uses all-mock `localMgr` with `ListQueuedReturns([]prompt.Prompt{}, nil)` (empty queue — never touches the file) and the final assertion `MatchRegexp("status: (rejected|failed)")` accepts every outcome. Rewrite it:

5.1. Build a REAL temp-dir fixture: an `inProgressDir` containing a real prompt file (status `failed`, valid frontmatter), plus `inboxDir`/`rejectedDir`. Use `os.MkdirTemp` and mode `0750` for dirs, `0600` for the fixture file (project convention).

5.2. One contender is a REAL reject: `cmd.NewRejectCommand(inboxDir, inProgressDir, rejectedDir, prompt.NewManager("", "", "", "", nil, libtime.NewCurrentDateTime()), nil, 0)` followed by `rejectCmd.Run(ctx, []string{"<file>.md", "--reason", "concurrent"})`. (Adjust the `NewRejectCommand` argument list to the order you chose in step 1.1.)

5.3. The other contender must genuinely take the SAME per-prompt FileLock on the same path and mutate the file under it — load → stamp/change status → save → rename — so the two truly race on the lock. You may either: (a) drive the real scanner candidate path with a real `prompt.NewManager` (not mocks) and a real fixture so `processSingleQueued` takes the lock; OR (b) hand-roll a contender goroutine that does `lock.NewFileLock(path).Acquire` → load → mutate → save/rename → `Release`, mirroring exactly what the production advance path now does. Pick the option that most faithfully exercises the production lock path; option (a) is preferred because it tests the real code from step 3.

5.4. Race both contenders in goroutines with a `sync.WaitGroup`; capture each side's slog output (use the slog-capture idiom at ~line 593, capturing into a shared buffer that both goroutines write to via the default logger).

5.5. Assert ALL of:
   - Exactly one final on-disk state: the prompt file exists in exactly one of `inProgressDir` or `rejectedDir`, and the combined count is `== 1` (never both, never neither). This is the corruption regression assertion — if the lock is removed, a save-after-rename interleaving makes the file appear in BOTH dirs, and this assertion fails. Add an explicit comment naming this as the regression lock.
   - The winning file's bytes parse as valid frontmatter (Load it back via `prompt.NewManager(...).Load` and assert no error and a non-empty status), not a torn write.
   - The combined slog output contains `lock acquired` at least twice (both sides logged taking the lock).
   - The loser observed the post-lock state: assert that the losing side's behavior reflects it (e.g. the reject returned an "already rejected"/"cannot reject" error, OR the scanner's post-lock re-read skipped the now-rejected candidate). Assert on whichever loser-signal is deterministic for your chosen contender wiring.

5.6. The test stays inside the existing `runtime.GOOS` gate. It must be hermetic (temp dirs only; no real `prompts/` paths) and `-race`-safe: any interleaving must pass the "exactly one final state" assertion; only a removed lock makes the corruption case reachable.

5.7. Fix or delete the misleading comment on the FileLock-primitive test ("It does not invoke the reject command directly...") if step 5 supersedes it.

## 6. Update the stale operator checklist (Finding 3)

In `/workspace/scenarios/011-reject-spec-cascade.md`, change ONLY line 102 (PROMPT frontmatter checklist): replace `rejected_reason: scenario regression test` with `rejectedReason: scenario regression test`. Leave line 91 (SPEC frontmatter) UNCHANGED — `pkg/spec/spec.go` deliberately keeps the snake_case `rejected_reason` key for specs.

## 7. Extend the read-compat round-trip proof (Finding 4)

In `/workspace/pkg/cmd/reject_test.go`, the read-compat test (~line 249) currently only Loads a legacy `rejected_reason:` file. Extend it to prove the new-key migration on the legacy path:

7.1. After loading the legacy-key fixture, call `pf.Save(ctx)` (the loaded `*prompt.PromptFile` exposes `Save`).
7.2. Re-read the raw bytes of the file with `os.ReadFile` and assert: the bytes contain `rejectedReason:` AND do NOT contain `rejected_reason:`. This directly proves spec 094 AC "no file persisted under the old key" on the legacy read path.

</requirements>

<constraints>

- Use `github.com/bborbe/errors` for all error wrapping (`errors.Wrap` / `errors.Errorf`). No bare `return err`, no `fmt.Errorf` for wrapping.
- Tests use Ginkgo/Gomega. Use counterfeiter mocks ONLY where a real component is impractical — Finding 1's rewritten test must use real components (real reject command, real `prompt.NewManager`, real `FileLock`) on the contention path, not mocks.
- Do NOT modify `/workspace/pkg/status/formatter.go`. The `Blocked:` status output must stay byte-stable (spec 094 AC `blocked-format-unchanged`).
- All tests hermetic: `os.MkdirTemp` paths only, no references to real `prompts/in-progress`, `prompts/rejected`, or `prompts/completed`.
- The container runs with `.git` masked (hideGit). No new test may resolve the real repo root; if any code path would, guard it with the `Skip(...)` pattern from `pkg/git/root_test.go`. The tests here use only temp dirs, so no Skip is needed — just do not introduce repo-root resolution.
- Directory mode `0750`, fixture file mode `0600`.
- Do NOT introduce a new counterfeiter mock or modify the `mocks/` directory. Existing mock structs are auto-generated.
- Do NOT commit — dark-factory handles git.
- Update EVERY `NewScanner` and `NewRejectCommand` call site (enumerated in `<context>`) so the build stays green — a missed call site is a compile failure.
- Branch is `feature/spec-092-remediation`.

</constraints>

<verification>

Run from the repo root inside the YOLO container:

```
cd /workspace && make precommit
```

Exit code 0 required (lint + vet + full test suite; all existing tests plus the new/rewritten tests must pass).

Then the race-detector gate the spec demands:

```
cd /workspace && go test -race -count=1 ./pkg/queuescanner/... ./pkg/cmd/...
```

Both must exit 0.

Static spot checks (each should print the indicated matches):

```
cd /workspace
grep -rn 'lock acquired' pkg/cmd/reject.go pkg/queuescanner/scanner.go        # >= 2: both production paths log it
grep -rn 'ReasonProjectLockTimeout' pkg/queuescanner/scanner.go               # >= 1: the dead token is now wired
grep -nE 'rejectedReason: scenario regression test' scenarios/011-reject-spec-cascade.md   # 1: line 102 corrected
grep -nE 'rejected_reason: scenario regression test' scenarios/011-reject-spec-cascade.md  # 1: line 91 (SPEC) unchanged
grep -rn 'prompts/in-progress\|prompts/rejected\|prompts/completed' pkg/queuescanner/*_test.go pkg/cmd/reject_test.go   # 0: hermeticity gate
grep -c 'lastBlockedMsg' pkg/queuescanner/scanner.go                          # 0: single-slot field replaced by a map
```

Confirm the rewritten concurrency test really exercises the lock: temporarily reasoning only (do NOT commit a broken state), the "exactly one final on-disk state" assertion must be the one that would fail if the production lock from step 3 were removed. If you can articulate that interleaving, the regression lock is real.

Sibling call-site check (must compile — no orphaned old signature):

```
cd /workspace
grep -rn 'NewScanner(' pkg/ | grep -v '_test.go'        # factory.go updated to new signature
grep -rn 'NewRejectCommand(' pkg/ | grep -v '_test.go'  # factory.go CreateRejectCommand updated
```

Both production call sites must pass the new arguments. If `make precommit` compiles, all test call sites are updated too.

</verification>
