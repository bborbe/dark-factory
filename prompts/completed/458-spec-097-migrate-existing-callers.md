---
status: approved
spec: [097-parent-dir-flock-locking]
created: "2026-06-18T12:11:00Z"
queued: "2026-06-18T12:10:24Z"
branch: dark-factory/parent-dir-flock-locking
---

<summary>

- The existing lock call sites (prompt reject, the daemon queue scanner's advance path, and every doctor fixer that takes a lock) switch from locking a per-file sidecar to locking the file's parent directory.
- Each call site now passes the file's parent directory (`filepath.Dir(path)`) to the lock factory instead of the file path itself.
- The "re-read file state after acquiring the lock" pattern is preserved exactly — callers still re-load the file after the lock is held, because the file may have moved or changed between request and acquisition.
- Tests for these call sites are updated so they construct/inject locks consistently with the new directory-scoped semantics, and continue to assert the same serialization behavior they did before.
- No `.lock` sidecar files are produced by any of these paths after the change.
- Behavior, timeouts, error messages, and the daemon's `project-lock-timeout` blocked-reason handling are otherwise unchanged.
- A potential same-directory double-lock in the spec-renumber fixer is handled by de-duplicating lock targets per directory, so the fixer cannot deadlock against itself.

</summary>

<objective>
Migrate the existing lock call sites — `pkg/cmd/reject.go` (prompt reject), `pkg/queuescanner/scanner.go` (daemon advance), and every doctor fixer that acquires a lock — from per-file sidecar locking to parent-directory locking, by passing `filepath.Dir(path)` to the `DirLock` factory. Update their tests. Preserve the post-lock re-read pattern everywhere.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first for YOLO container conventions, then `/workspace/CLAUDE.md`.

Read these coding-plugin docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — error wrapping conventions
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega, counterfeiter mocks, coverage >=80%

Read these source files end-to-end before editing:
- `/workspace/pkg/lock/filelock.go` — AFTER prompt 1 lands, this exposes `DirLock` interface and `NewDirLock(dirPath string) DirLock`. The lock now `flock`s the directory passed in. Confirm the new signature before editing callers.
- `/workspace/pkg/cmd/reject.go` — the prompt reject command. The lock is acquired at the comment "Acquire the per-prompt file lock BEFORE loading the file." via `fl := r.fileLockFactory(path)`. The post-lock re-read is `pf, err := r.promptManager.Load(ctx, path)` after the acquire. The fast-fail pre-read above the lock stays as-is.
- `/workspace/pkg/queuescanner/scanner.go` — the daemon advance path in `processSingleQueued`. The lock is acquired at the comment "Acquire the per-prompt file lock right before handing the candidate to the processor." via `fl := s.fileLockFactory(pr.Path)`. On acquire failure the scanner logs `prompt.ReasonProjectLockTimeout` and ends the scan. The post-lock re-read is `pf, err := s.promptManager.Load(ctx, pr.Path)`.
- `/workspace/pkg/doctor/fix_status_dir_mismatch.go` — `applyStatusDirMismatchPath` acquires `fl := f.deps.FileLockFactory(path)`.
- `/workspace/pkg/doctor/fix_unlink.go` — contains `applyOrphanPromptLinkPath`; acquires `fl := f.deps.FileLockFactory(path)` (~line 50).
- `/workspace/pkg/doctor/fix_orphan_in_progress.go` — acquires `fl := f.deps.FileLockFactory(path)` (~line 42).
- `/workspace/pkg/doctor/fix_sweep.go` — acquires `fl := f.deps.FileLockFactory(specPath)` (~line 32).
- `/workspace/pkg/doctor/fix_renumber.go` — acquires TWO lock sites: `fl := f.deps.FileLockFactory(candidatePath)` (~line 118, inside a `for dir := range specDirs` loop) and `fl := f.deps.FileLockFactory(rn.NewPath)` (~line 143). Also note its lock-slice type `*[]lock.FileLock` (renamed to `*[]lock.DirLock` mechanically in prompt 1). READ this file fully — its locking is the most intricate of the set.
- `/workspace/pkg/doctor/releaselock.go` — `releaseLock(ctx, fl lock.DirLock, path)` (type renamed in prompt 1).
- `/workspace/pkg/cmd/reject_test.go`, `/workspace/pkg/queuescanner/scanner_test.go`, `/workspace/pkg/doctor/fixer_test.go` — the tests to update.
- `/workspace/specs/in-progress/097-parent-dir-flock-locking.md` — `Desired Behavior` 5, `Constraints` (post-lock re-read preserved; doctor fixer migrates from `NewFileLock(specPath)` to `NewDirLock(filepath.Dir(specPath))`, semantics unchanged).

KEY CONSTRAINT FROM SPEC: the factory parameter is now a DIRECTORY. The lock factory type is `func(path string) lock.DirLock` (renamed in prompt 1). The argument passed at every call site changes from the file path to `filepath.Dir(filePath)`. The factory PARAMETER NAME may remain `path` but the VALUE passed is the directory.

</context>

<requirements>

## 1. Migrate `pkg/cmd/reject.go`

1.1. At the lock-acquire site (`fl := r.fileLockFactory(path)` under the comment "Acquire the per-prompt file lock BEFORE loading the file."), change the argument to the parent directory:
```go
fl := r.fileLockFactory(filepath.Dir(path))
```
`filepath` is already imported. The post-lock `r.promptManager.Load(ctx, path)` re-read stays unchanged (still loads the FILE; only the LOCK target is the directory).

1.2. Update the doc comment on `NewRejectCommand` and the inline comments that say "per-prompt lock" / "per-prompt file lock" to say "status-directory lock" / "per-directory lock". Keep the spec-092 cross-reference ("concurrent-reject-advance").

1.3. The log line `slog.Info("lock acquired", "file", filepath.Base(path))` stays — no change.

## 2. Migrate `pkg/queuescanner/scanner.go`

2.1. At the lock-acquire site (`fl := s.fileLockFactory(pr.Path)`), change to:
```go
fl := s.fileLockFactory(filepath.Dir(pr.Path))
```
`filepath` is already imported. The post-lock `s.promptManager.Load(ctx, pr.Path)` re-read and the `ReasonProjectLockTimeout` blocked-reason handling stay unchanged.

2.2. Update the comments referencing "per-prompt file lock" to "status-directory lock" while preserving the spec-092 rationale text about the loser observing the post-lock state via re-read.

## 3. Migrate ALL doctor fixer lock call sites

There are SIX call sites across five files (confirm the full set with the grep below). EACH changes `FileLockFactory(<filePath>)` -> `FileLockFactory(filepath.Dir(<filePath>))`. The lock is on the SOURCE directory, consistent with the spec's "source-side lock is sufficient" rule. Add `path/filepath` to the import block of any file that gains a `filepath.Dir` call and does not already import it. Do NOT change `releaseLock`'s signature.

```bash
grep -rn "FileLockFactory(" /workspace/pkg/doctor/ --include='*.go' --exclude='*_test.go'
```
Expected hits to migrate:

3.1. `/workspace/pkg/doctor/fix_status_dir_mismatch.go` (in `applyStatusDirMismatchPath`): `FileLockFactory(path)` -> `FileLockFactory(filepath.Dir(path))`. `filepath` already imported. The subsequent `os.MkdirAll(expectedDir, 0750)` + audit + `os.Rename(path, dest)` are unchanged.

3.2. `/workspace/pkg/doctor/fix_unlink.go` (contains `applyOrphanPromptLinkPath`, ~line 50): `FileLockFactory(path)` -> `FileLockFactory(filepath.Dir(path))`.

3.3. `/workspace/pkg/doctor/fix_orphan_in_progress.go`: `FileLockFactory(path)` -> `FileLockFactory(filepath.Dir(path))`.

3.4. `/workspace/pkg/doctor/fix_sweep.go`: `FileLockFactory(specPath)` -> `FileLockFactory(filepath.Dir(specPath))`.

3.5. `/workspace/pkg/doctor/fix_renumber.go` — TWO call sites, AND a same-directory double-lock hazard that directory-scoped locking introduces. Read the file fully before editing.
- Site A (~line 118): inside `for _, t := range finding.TargetPaths { for _, dir := range specDirs { candidatePath := filepath.Join(dir, t); fl := f.deps.FileLockFactory(candidatePath) ...} }`. With per-FILE locking, two candidate files in the same dir produced two independent sidecar locks. With per-DIRECTORY locking, two candidate files in the SAME `dir` would both resolve to `filepath.Dir(candidatePath) == dir` — and `NewDirLock(dir).Acquire` on a SECOND fd while the FIRST is still held in the same process FAILS (flock is per-fd; `LOCK_EX|LOCK_NB` on the second fd returns EWOULDBLOCK, and after the timeout the fixer reports a spurious "lock acquire failed"). To avoid this self-deadlock, lock each DISTINCT directory at most once. Implement by tracking already-locked directories in a `map[string]struct{}` (or a `collection`-style set) across BOTH loop sites: compute `dir := filepath.Dir(candidatePath)`, skip the acquire if that dir is already in the set, otherwise acquire `lock factory(dir)`, append to `*locks`, and add the dir to the set. Carry the same set into site B so a NewPath whose dir is already locked is not re-locked.
- Site B (~line 143): `for _, rn := range renames { fl := f.deps.FileLockFactory(rn.NewPath) ...}` -> compute `dir := filepath.Dir(rn.NewPath)`; skip if `dir` already in the dedupe set; else acquire `f.deps.FileLockFactory(dir)`, append to `*locks`, add to set. The renumber fixer (and its helpers) reference `lock.FileLock` slice types — leave the type name renames to prompt 1 (already covered); this prompt only changes the call-site argument from file path to dir + adds the dedupe set.
- The dedupe set must be shared between the pre-reindex acquire (site A) and the post-reindex NewPath acquire (site B) so the renumber operation holds exactly one lock per distinct directory. Pass the set (or a small struct holding it) through both helper calls. Keep the existing `*locks` accumulation + the deferred release-all so every acquired lock is released on completion.
- If the existing helper signatures (`acquireCandidateLocks` / `acquireNewPathLocks` or similarly named) do not already share state, add a `seen map[string]struct{}` parameter to both and initialize it once in the caller. Do NOT restructure the renumber algorithm — only add the per-directory dedupe.

3.6. For each migrated doctor fixer, the post-lock behavior (Load / audit / rename) is unchanged. The fixers still lock the file's own status directory — semantics unchanged, only the on-disk lock artifact disappears and same-dir targets share one lock.

## 4. Update tests

4.1. `/workspace/pkg/cmd/reject_test.go`: if the test injects a custom `fileLockFactory` and asserts it was called with a specific argument, update the expected argument to the directory (`filepath.Dir(promptPath)`). If the test uses the default real lock (nil factory), no change beyond confirming it still passes.

4.2. `/workspace/pkg/queuescanner/scanner_test.go`: any test that injects a `fileLockFactory` capturing the requested path and asserting it equals the prompt's file path must now expect the prompt's PARENT DIRECTORY. The concurrency tests that construct real `lock.NewDirLock(...)` were already adapted in prompt 1; this prompt fixes scanner-specific factory-argument expectations. Search:
```bash
grep -n "fileLockFactory\|fakeLock\|lockMock\|NewDirLock\|FileLockFactory" /workspace/pkg/queuescanner/scanner_test.go
```

4.3. `/workspace/pkg/doctor/fixer_test.go`: any assertion on the argument passed to the injected `FileLockFactory` must expect `filepath.Dir(path)`. The injected fake `mocks.LockDirLock` (renamed in prompt 1) is unchanged in behavior. Search:
```bash
grep -n "FileLockFactory\|LockDirLock\|filepath.Dir\|fixRenumber\|renumber" /workspace/pkg/doctor/fixer_test.go
```
If a renumber test asserts a specific NUMBER of lock acquisitions (one per file), update it to expect one acquisition per distinct DIRECTORY (the dedupe added in 3.5). Add a test asserting that a renumber finding with two files in the SAME directory acquires the directory lock exactly once (proves the dedupe prevents the self-deadlock). If `fix_renumber_test.go` exists, prefer adding the dedupe assertion there.

4.4. Do NOT weaken any existing serialization or "lock acquire failed -> FailedFix" assertions. Only adapt the lock-target argument expectations, the per-directory acquisition counts, and any temp-dir setup that previously created files at sidecar paths.

## 5. No-sidecar invariant for migrated production code

After migration, confirm none of the migrated production paths construct a `.lock` name:
```bash
grep -rEn '"\.lock"|\+ *"\.lock"' /workspace/pkg/cmd/ /workspace/pkg/queuescanner/ /workspace/pkg/doctor/ --include='*.go' --exclude='*_test.go'
```
Expected: empty (the legacy `.md.lock` reference lives only in prompt 4's cleanup fixer, not yet added).

</requirements>

<constraints>

- The "re-read file state after lock acquired" caller pattern is PRESERVED everywhere — the post-lock `Load` calls stay; only the lock TARGET changes from file to directory (spec Constraint).
- The 5-second timeout and 100ms poll interval are unchanged (spec Non-goal).
- The doctor fixers' semantics are unchanged — each still locks the spec/prompt's own status directory (spec Constraint: "still locks the spec's status dir").
- No change to public CLI flags, exit codes, or output of any mutation command (spec Constraint).
- The daemon's `ReasonProjectLockTimeout` blocked-reason path and `dark-factory status` token stay byte-stable — do not alter the blocked-log strings.
- The renumber fixer MUST NOT deadlock against itself — lock each distinct directory at most once (requirement 3.5). This is a correctness requirement directly caused by switching from per-file to per-directory locking; do not skip it.
- Errors wrapped with `bborbe/errors` (`errors.Wrap(ctx, err, "...")`) — never `fmt.Errorf`, never `context.Background()` in pkg/.
- BSD-style license header on every modified file must survive (spec Constraint).
- Coverage for changed packages must stay >= 80%; do not drop existing error-path coverage.
- Do NOT modify `pkg/lock/filelock.go` in this prompt — it is owned by prompt 1. If `NewDirLock` is not present, STOP and report `status: failed` with message "DirLock primitive not yet deployed (prompt 1)".
- Do NOT add the cleanup fixer here — that is prompt 4.
- Do NOT commit — dark-factory handles git.

</constraints>

<verification>

Run from the repo root inside the YOLO container:

```bash
cd /workspace && make test
```

All packages pass. Then the targeted checks:

```bash
cd /workspace
grep -n 'fileLockFactory(filepath.Dir(' pkg/cmd/reject.go            # 1 match
grep -n 'fileLockFactory(filepath.Dir(' pkg/queuescanner/scanner.go  # 1 match
grep -rn 'FileLockFactory(filepath.Dir(' pkg/doctor/ --include='*.go' --exclude='*_test.go'   # >=5 matches (one per migrated site; renumber uses a dir var)
grep -rn 'FileLockFactory([a-zA-Z]*Path)\|FileLockFactory(path)\|FileLockFactory(specPath)\|FileLockFactory(candidatePath)\|FileLockFactory(rn.NewPath)' pkg/doctor/ --include='*.go' --exclude='*_test.go'   # 0 matches (no raw file-path acquire left)
grep -rEn '"\.lock"|\+ *"\.lock"' pkg/cmd/ pkg/queuescanner/ pkg/doctor/ --include='*.go' --exclude='*_test.go'   # 0 matches
go test ./pkg/cmd/... ./pkg/queuescanner/... ./pkg/doctor/... -v 2>/dev/null | tail -15
```

Then full precommit:

```bash
cd /workspace && make precommit
```

Exit code 0 required. On a single-target failure, fix and re-run only that target until green, then re-run `make precommit` once.

Coverage for the three changed packages:
```bash
cd /workspace && go test -coverprofile=/tmp/cover.out -mod=vendor ./pkg/cmd/... ./pkg/queuescanner/... ./pkg/doctor/... && go tool cover -func=/tmp/cover.out | tail -1
```

</verification>
