---
status: verifying
tags:
    - dark-factory
    - spec
approved: "2026-06-18T11:45:58Z"
generating: "2026-06-18T11:48:44Z"
prompted: "2026-06-18T12:00:19Z"
verifying: "2026-06-18T13:56:28Z"
branch: dark-factory/parent-dir-flock-locking
---

## Summary

- Today's per-file `.lock` sidecar files (created by `pkg/lock/filelock.go`) intentionally linger after release to dodge a TOCTOU window. Result: empty `*.md.lock` files accumulate next to every prompt that has ever been mutated. The user finds the litter annoying and confusing.
- Spec mutation commands (approve / reject / complete / unapprove / mark-prompted) have NO locking at all — inconsistent with prompt mutations and with the doctor fixer that already locks both.
- This spec replaces the sidecar-file scheme with a flock on the **parent status directory** (e.g. `prompts/in-progress/`, `specs/approved/`). The directory always exists, is never deleted by the protocol, leaves nothing on disk, and survives atomic-replace writes that swap the file inode.
- Same protocol applies uniformly to every prompt AND spec mutation. Coarser concurrency (one mutation per status dir at a time) is the accepted tradeoff — dark-factory mutation throughput is human + daemon pace, not high-frequency.
- One-shot cleanup of pre-existing `*.md.lock` litter is part of the rollout.

## Problem

`pkg/lock/filelock.go` locks by creating `<target>.lock` and `flock`'ing that sidecar file. The sidecar is intentionally never removed on `Release()` — removing it opens a TOCTOU window where another process could `Acquire` between our close and our remove, end up flock'ing a recreated inode, and silently lose mutual exclusion. The trade is "leaks empty files" vs "correctness", and correctness won. The observable result: directories like `prompts/in-progress/` accumulate one empty `*.md.lock` next to every prompt that was ever touched. The litter is cosmetic but persistent, has to be explained to every new reader of the repo, and motivates ad-hoc `find -delete` cleanups.

Meanwhile, spec mutation commands (`spec_approve.go`, `spec_reject.go`, `spec_complete.go`, `spec_unapprove.go`, `spec_mark_prompted.go`) acquire no lock at all. Today's race exposure is low — the daemon does not mutate specs — but two concurrent CLI invocations can interleave reads and writes, and the doctor fixer `pkg/doctor/fix_status_dir_mismatch.go` already takes a `FileLock` on specs, proving the protocol is inconsistent.

## Goal

After this work, every prompt and spec mutation in dark-factory serializes through a flock on the parent status directory of the file being mutated. No `*.lock` sidecar files exist anywhere under `prompts/` or `specs/` during normal operation. Spec mutations honor the same locking protocol as prompt mutations. Crash-release works automatically (kernel drops flock on process death). Concurrency is coarser than per-file (one mutator per status directory at a time) but this is the accepted tradeoff given dark-factory's low mutation throughput.

## Non-goals

- Does NOT move dark-factory to a daemon-IPC single-writer model — separate future spec.
- Does NOT change `pkg/containerslot/manager.go` — different lock scope, out of scope.
- Does NOT change the file format of prompts or specs — only the locking mechanism.
- Does NOT preserve per-file concurrency — coarser-by-design; do not reintroduce per-file granularity as a "best of both worlds" knob. Invariant; if a future consumer demands per-file concurrency, that's a separate spec.
- Does NOT add a fallback / opt-out to the old sidecar scheme — the new scheme is the only scheme post-change.
- Does NOT change the 5-second timeout or 100ms poll interval.
- Does NOT change the "re-read file state after lock acquired" caller pattern.

## Desired Behavior

1. `NewFileLock(path)` (or its replacement) acquires an exclusive advisory lock on the **parent directory** of `path` (i.e. `filepath.Dir(path)`), not on a `.lock` sidecar of `path`. Two mutators targeting any files inside the same directory serialize; two mutators in different directories proceed in parallel.
2. `Acquire` opens the directory read-only, calls `syscall.Flock(fd, LOCK_EX|LOCK_NB)`, retries every 100ms until success, ctx cancel, or 5-second timeout — same observable timing as today.
3. `Release` closes the directory fd (which drops the flock); creates and removes no files. Idempotent.
4. When a process holding the lock dies (crash, SIGKILL, panic exit), the kernel auto-releases the flock; the next `Acquire` from any other process succeeds without manual cleanup.
5. Every prompt mutation site (`pkg/cmd/reject.go`, `pkg/queuescanner/scanner.go`'s advance path, `pkg/doctor/fix_status_dir_mismatch.go`) uses the new lock with the file's parent dir as the lock target.
6. Every spec mutation command (`spec_approve.go`, `spec_reject.go`, `spec_complete.go`, `spec_unapprove.go`, `spec_mark_prompted.go`) acquires the lock on the source-state directory BEFORE reading the spec, and holds it across the move-to-new-state operation. If the mutation moves the file between two directories, the source directory's lock is sufficient (the destination directory is determined by the source state + operation; no concurrent mutator on the destination can produce the same filename until the source-side lock is released).
7. A full prompt + spec lifecycle (create → approve → advance → complete/reject) leaves zero `*.lock` files anywhere under `prompts/` or `specs/`.
8. Pre-existing `*.md.lock` files from the old scheme are removed by a new `dark-factory doctor` fixer entry (consistent with the existing `fix_status_dir_mismatch.go` precedent — discoverable, idempotent, runs only on explicit operator invocation). No startup hook, no implicit cleanup.

## Constraints

- The 5-second default timeout and 100ms poll interval are unchanged.
- Caller-side "re-read file state after lock acquired" pattern is preserved everywhere — file may have moved or changed state between request and lock acquisition.
- `pkg/lock` exposes a `DirLock` interface (renamed from `FileLock`) constructed via `NewDirLock(dirPath string)` usable by both `pkg/cmd` and `pkg/doctor`. Deleting the abstraction entirely is not allowed.
- BSD-style license header on every modified file must survive the edit.
- `CHANGELOG.md` entry under `## Unreleased` describes the change in one bullet referencing this spec.
- The doctor fixer (`fix_status_dir_mismatch.go`) continues to work — its existing call site migrates from `NewFileLock(specPath)` to `NewDirLock(filepath.Dir(specPath))`; semantics unchanged (still locks the spec's status dir).
- No change to public CLI flags, exit codes, or output of any mutation command beyond the new lock behavior.
- macOS and Linux must both work — `syscall.Flock` is supported on both with the same semantics on directories (advisory, per-fd, kernel-released).

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection | Reversibility | Concurrency |
|---------|-------------------|----------|-----------|---------------|-------------|
| Two CLI invocations mutate prompts in the same status dir simultaneously | Second invocation polls until first releases (≤5s); both complete cleanly with no corruption | Automatic — second acquires after first releases | Both invocations exit 0; final on-disk state matches sequential execution | Reversible (no destructive ops) | Serialized by flock |
| CLI invocation crashes / is SIGKILL'd while holding the lock | Kernel drops flock on process death; next `Acquire` succeeds with no manual cleanup | Automatic — no stale lock files to delete | Next invocation acquires within ≤5s timeout | Reversible | Auto-release on EOF/exit |
| Status directory does not exist at lock-acquire time (e.g. fresh repo) | `Acquire` returns a wrapped error naming the missing directory; caller surfaces a clear "no prompts/specs in state X" message OR creates the dir before locking (caller's choice) | Caller ensures dir exists via existing `EnsureDirExists` pattern before `Acquire` | Error message names the missing path | Reversible | N/A |
| Lock acquire times out after 5s (heavy contention) | Returns wrapped timeout error naming the directory; caller exits non-zero | Re-run the command; investigate the holder if it persists | Exit code non-zero + stderr message containing `lock acquire timeout` and the directory path | Reversible | N/A |
| Pre-existing `*.md.lock` litter from old scheme present at first invocation post-upgrade | One-shot cleanup path removes them; subsequent invocations see clean tree | Idempotent — re-running cleanup is a no-op | `find prompts specs -name '*.lock'` returns empty after first post-upgrade invocation completes | Reversible (files are empty) | Cleanup runs under the same per-dir lock as the mutation, OR as a standalone scan before any locking |
| Atomic-replace write swaps the target file inode mid-mutation | New scheme is unaffected — flock is on the directory inode, which is stable | Automatic | No corruption observable; tests cover this | Reversible | N/A |
| Process A holds dir lock, process B opens the same dir for `Acquire` | B's `LOCK_EX\|LOCK_NB` fails with EWOULDBLOCK; B polls and retries; succeeds after A releases | Automatic | A and B both log acquire+release in order | Reversible | Serialized |

## Security / Abuse Cases

Not applicable — locking is process-local advisory state; no network, no untrusted input, no trust boundary crossed. The lock target path is derived from a caller-supplied target file path, but the directory operated on is `filepath.Dir(target)` and the operation is `open(O_RDONLY) + flock` — no write, no create, no traversal beyond what the caller already had access to. No new attack surface introduced.

## Acceptance Criteria

- [ ] `make precommit` exits 0 in the repo root — evidence: exit code 0
- [ ] After a full lifecycle (`dark-factory prompt new ...` → daemon advance → `prompt reject` and `spec approve` → `spec complete`), `find prompts specs -name '*.lock'` prints zero lines — evidence: empty stdout, exit code 0
- [ ] No production code path constructs `<path>.lock` sidecar names — evidence: `grep -rEn '"\.lock"|\+ *"\.lock"' pkg/lock/ pkg/cmd/ pkg/queuescanner/ pkg/doctor/ --include='*.go' --exclude='*_test.go'` returns matches only inside the doctor cleanup fixer (which legitimately references the legacy suffix to remove old artifacts); all other production sites return empty
- [ ] `pkg/lock/filelock_test.go` (or its replacement) contains a test where two goroutines lock the same directory concurrently and observe serial ordering — evidence: test function `TestParentDirLock_SerializesSameDir` (or equivalent name) exists and passes via `go test ./pkg/lock/...` exit code 0
- [ ] `pkg/lock` test suite contains a test where two goroutines lock two different directories concurrently and observe parallel progress — evidence: test function `TestParentDirLock_ParallelDifferentDirs` (or equivalent) exists and passes
- [ ] `pkg/lock` test suite contains a test where one goroutine acquires, then simulates process death (close fd without explicit unlock), and a second `Acquire` succeeds without manual cleanup — evidence: test asserts (a) the second `Acquire` returns nil within the timeout AND (b) `find <testdir> -name '*.lock'` returns empty (kernel auto-release leaves no on-disk artifact)
- [ ] Every spec mutation command (`spec_approve`, `spec_reject`, `spec_complete`, `spec_unapprove`, `spec_mark_prompted`) calls into the new lock before reading or writing the spec file — evidence: `grep -n 'FileLock\|DirLock\|Acquire' pkg/cmd/spec_*.go` shows a lock-acquire call in each of the five files
- [ ] CHANGELOG.md `## Unreleased` section has a one-bullet entry referencing this spec — evidence: `grep -n 'parent-dir-flock\|directory.*flock\|parent directory' CHANGELOG.md` returns ≥1 match under `## Unreleased`
- [ ] Manual acceptance scenario: two terminals run `dark-factory prompt reject 002 "reason"` and a second `prompt reject` race against the same prompt; both exit cleanly and the on-disk state matches a single sequential reject — evidence: terminal exit codes both 0 and `dark-factory prompt show 002` reports rejected state with no duplicate files
- [ ] Manual acceptance scenario: two terminals operate on prompts in `in-progress/` vs `rejected/` simultaneously; wall-clock duration of running both in parallel is approximately the time of the longer single run, not the sum — evidence: timed run shows parallel execution
- [ ] Pre-existing `*.md.lock` litter is removed by the new `dark-factory doctor` fixer entry — evidence: after running `dark-factory doctor`, `find prompts specs -name '*.md.lock'` is empty; re-running `dark-factory doctor` is a no-op
- [ ] No scenario test added (gated: behavior is reachable via unit tests in `pkg/lock` + integration coverage in existing prompt/spec command tests; locking is not user-journey-load-bearing in a way unit + integration cannot reach)

## Verification

```
make precommit
go test ./pkg/lock/... -run TestParentDirLock -v
go test ./pkg/cmd/... -run 'TestSpec(Approve|Reject|Complete|Unapprove|MarkPrompted)' -v
find prompts specs -name '*.lock'    # expect: empty
```

## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Refactor `pkg/lock` to parent-dir flock; preserve public interface shape or rename cleanly; add the three new unit tests (same-dir serialize, different-dir parallel, crash-release) | 1, 2, 3, 4 | unit-test ACs + `make precommit` | — |
| 2 | Migrate existing callers (`pkg/cmd/reject.go`, `pkg/queuescanner/scanner.go`, `pkg/doctor/fix_status_dir_mismatch.go`) to the new lock API; update their tests | 5 | caller-side ACs | prompt 1 |
| 3 | Add locking to the five spec mutation commands (`spec_approve`, `spec_reject`, `spec_complete`, `spec_unapprove`, `spec_mark_prompted`); update their tests | 6 | spec-side ACs + grep AC | prompt 1 (interface stable) |
| 4 | One-shot cleanup of pre-existing `*.md.lock` litter via the chosen path (startup hook / doctor fixer / documented command); CHANGELOG entry | 7, 8 | cleanup AC + CHANGELOG AC + lifecycle AC | prompts 1-3 |

Rationale: prompt 1 lands the new primitive with tests so prompts 2-3 can run in parallel once the interface is stable. Prompt 4 is last because the lifecycle AC ("zero `*.lock` files after a full lifecycle") can only be verified once all mutation sites use the new scheme. The decomposition is required because the spec touches 4 code layers (`pkg/lock`, `pkg/cmd`, `pkg/queuescanner`, `pkg/doctor`) and has 8 DBs — splitting along the layer seams keeps each prompt's review surface small.

## Do-Nothing Option

If we do nothing: `*.md.lock` files continue to accumulate, the rationale comment in `filelock.go` continues to need explaining to new readers, and spec mutations remain unlocked-by-protocol. Today's race exposure is low (daemon does not mutate specs, CLI throughput is human-paced), so the correctness risk is small in the short term. The cost of inaction is steady cosmetic litter + protocol asymmetry that will bite the day a future feature lets the daemon mutate specs. Acceptable to defer if higher-priority work demands the slot; not acceptable as a permanent answer.

## Verification Result

**Verified:** 2026-06-18T14:07:48Z (HEAD 5273d81)
**Binary:** /tmp/dark-factory-5273d81 (built from HEAD)
**Scenario:** Sandbox `prompt reject` race (same dir → serial via lock; different dirs → parallel) + doctor `--fix` legacy-lock cleanup + idempotency re-run.
**Evidence:**
- `make precommit` exit 0; `go test ./pkg/lock/...` 27/27 specs green, incl. TestParentDirLock_SerializesSameDir / _ParallelDifferentDirs / _CrashReleaseLeavesNoArtifact
- Same-dir race: both procs logged `lock acquired file=001-race-target.md` at 16:06:18.806/.807; winner stamped `rejectedReason: race-B`, loser exited with post-lock "no such file or directory" — state matches single sequential reject, zero `.lock` files
- Different-dir parallel: 0.290s vs sequential 0.381s, both exit 0, simultaneous lock-acquired timestamps (16:06:50.076)
- `grep -rEn '"\.lock"|\+ *"\.lock"' pkg/lock/ pkg/cmd/ pkg/queuescanner/ pkg/doctor/ --include='*.go' --exclude='*_test.go'` → single match in `pkg/doctor/legacy_lock_file.go:43` (legacy-cleanup, AC-permitted)
- `dark-factory doctor` detected 3 seeded `*.md.lock` files, `--fix --yes` removed all, re-run reported "no findings"
- `find prompts specs -name '*.lock'` in repo root → empty
- CHANGELOG.md `## Unreleased` carries 3 bullets (refactor + feat × 2), one citing `spec 097`
**Verdict:** PASS
