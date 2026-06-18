---
status: approved
spec: [097-parent-dir-flock-locking]
created: "2026-06-18T12:12:00Z"
queued: "2026-06-18T12:10:24Z"
branch: dark-factory/parent-dir-flock-locking
---

<summary>

- The five spec mutation commands (approve, reject, complete, unapprove, mark-prompted) gain locking — today they mutate spec files with no lock at all, inconsistent with prompt mutations and the doctor fixer.
- Each command now acquires a directory lock on the spec's source-state directory BEFORE reading the spec, and holds it across the move-to-new-state operation.
- After acquiring the lock, each command re-reads the spec file (the same post-lock re-read pattern prompt-reject already uses), since the spec may have moved or changed between the find and the acquire.
- Two concurrent CLI invocations against specs in the same directory now serialize (second waits up to 5 seconds); invocations against different directories proceed in parallel.
- On lock-acquire timeout the command exits non-zero with a clear error naming the directory; no silent stall.
- All five commands' constructors gain a lock-factory parameter (defaulting to the real directory lock) and a timeout (defaulting to 5 seconds); the factory wiring and all tests are updated to match.
- No `.lock` sidecar files are created — the lock is on the directory inode.

</summary>

<objective>
Add directory-scoped locking to the five spec mutation commands — `spec_approve.go`, `spec_reject.go`, `spec_complete.go`, `spec_unapprove.go`, `spec_mark_prompted.go` — so each acquires a `DirLock` on the spec's source-state directory before reading the file and holds it across the move. Re-read the spec after acquiring. Update the factory wiring and all command tests.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read these coding-plugin docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — error wrapping
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — zero-logic `Create*` factories, constructor pattern
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega, coverage >=80%

Read these source files end-to-end before editing:
- `/workspace/pkg/lock/filelock.go` — AFTER prompt 1: `DirLock` interface + `NewDirLock(dirPath string) DirLock`. Confirm the signature before use.
- `/workspace/pkg/cmd/reject.go` — the REFERENCE PATTERN for adding a lock to a command. Note: constructor takes `fileLockFactory func(path string) lock.DirLock` (after prompt 2) and `lockTimeout time.Duration`; both default when nil/zero (`if fileLockFactory == nil { fileLockFactory = lock.NewDirLock }`, `if lockTimeout == 0 { lockTimeout = 5 * time.Second }`). The lock is acquired with `fl := r.fileLockFactory(filepath.Dir(path))`, `defer fl.Release(ctx)` with a warn-on-release-failure, then the file is re-loaded. Mirror this shape exactly in the five spec commands.
- `/workspace/pkg/cmd/spec_approve.go` — source dir is the inbox: `path, _ := FindSpecFile(ctx, s.inboxDir, id)`. Lock `filepath.Dir(path)` (== `s.inboxDir`). Then `spec.Load`, transition check, save, `os.MkdirAll(s.inProgressDir)`, `os.Rename(path, dest)`.
- `/workspace/pkg/cmd/spec_reject.go` — source from `FindSpecFileInDirs(ctx, id, s.specsInboxDir, s.specsInProgressDir)`. Lock `filepath.Dir(path)`. Holds across `spec.Load`, linked-prompt preflight + reject, save, rename to rejected.
- `/workspace/pkg/cmd/spec_complete.go` — source from `FindSpecFileInDirs(ctx, id, s.inboxDir, s.inProgressDir, s.completedDir)`. Lock `filepath.Dir(path)`. Holds across load, transition, save, rename to completed.
- `/workspace/pkg/cmd/spec_unapprove.go` — source from `FindSpecFile(ctx, s.inProgressDir, id)`. Lock `filepath.Dir(path)` (== `s.inProgressDir`). Holds across load, transition, save, rename to inbox, renumber.
- `/workspace/pkg/cmd/spec_mark_prompted.go` — source from `FindSpecFileInDirs(ctx, args[0], s.inboxDir, s.inProgressDir, s.completedDir)`. Lock `filepath.Dir(path)`. Holds across load, status transition, save (no rename).
- `/workspace/pkg/factory/factory.go` — `CreateSpecApproveCommand` (~1478), `CreateSpecUnapproveCommand` (~1491), `CreateSpecRejectCommand` (~1535), `CreateSpecCompleteCommand` (~1560), `CreateSpecMarkPromptedCommand` (~1573). ALL FIVE must pass the new constructor args.
- `/workspace/pkg/cmd/spec_approve_test.go`, `spec_reject_test.go`, `spec_complete_test.go`, `spec_unapprove_test.go`, `spec_mark_prompted_test.go` — the tests. Note `spec_approve_test.go` and `spec_complete_test.go` have a SECOND constructor call (around lines 127 and 129) for a fixed-clock variant; `spec_mark_prompted_test.go` has one at line 229. Every constructor call site must be updated.
- `/workspace/specs/in-progress/097-parent-dir-flock-locking.md` — `Desired Behavior` 6, `Failure Modes` table (lock timeout, missing-dir, crash-release), `Acceptance Criteria` (grep AC: each of the five files must show a lock-acquire call).

SOURCE-DIR LOCK RULE (from spec Desired Behavior 6): lock the SOURCE-state directory before reading. The source directory is `filepath.Dir(path)` where `path` is whatever `FindSpecFile`/`FindSpecFileInDirs` returns. The destination directory needs no separate lock — "no concurrent mutator on the destination can produce the same filename until the source-side lock is released."

</context>

<requirements>

For ALL FIVE commands, follow the reject.go pattern. Use these EXACT field/parameter names for consistency: struct field `dirLockFactory func(dirPath string) lock.DirLock` and `lockTimeout time.Duration`; constructor params of the same names appended to the END of each existing constructor signature.

## 1. spec_approve.go

1.1. Add imports `log/slog`, `time`, and `github.com/bborbe/dark-factory/pkg/lock` (group the local import with the existing `github.com/bborbe/dark-factory/pkg/spec`).

1.2. Add struct fields `dirLockFactory func(dirPath string) lock.DirLock` and `lockTimeout time.Duration` to `specApproveCommand`.

1.3. Append `dirLockFactory func(dirPath string) lock.DirLock` and `lockTimeout time.Duration` to `NewSpecApproveCommand`. In the body, default them:
```go
if dirLockFactory == nil {
	dirLockFactory = lock.NewDirLock
}
if lockTimeout == 0 {
	lockTimeout = 5 * time.Second
}
```
Assign both to the struct.

1.4. In `Run`, AFTER `path, err := FindSpecFile(...)` (so the lock target is known) and BEFORE `spec.Load`, acquire the lock on the source directory and defer release:
```go
fl := s.dirLockFactory(filepath.Dir(path))
if err := fl.Acquire(ctx, s.lockTimeout); err != nil {
	return errors.Wrap(ctx, err, "acquire spec approve lock")
}
defer func() {
	if relErr := fl.Release(ctx); relErr != nil {
		slog.Warn("spec approve: lock release failed", "dir", filepath.Dir(path), "error", relErr.Error())
	}
}()
```
The existing `spec.Load(ctx, path, ...)` becomes the post-lock re-read — no other change to the load/transition/save/rename sequence.

## 2. spec_reject.go

2.1. Add imports `log/slog`, `time`, `github.com/bborbe/dark-factory/pkg/lock` (it already imports `prompt` and `spec`; `filepath` is already imported).

2.2-2.3. Add the same two fields and constructor params + defaults as section 1 (struct `specRejectCommand`, constructor `NewSpecRejectCommand`).

2.4. In `rejectSpec`, AFTER `path, err := FindSpecFileInDirs(...)` and BEFORE `spec.Load`, acquire `fl := s.dirLockFactory(filepath.Dir(path))` with the same Acquire/defer-release block (message "acquire spec reject lock", warn "spec reject: lock release failed"). The lock is held across the linked-prompt preflight, the linked-prompt rejects, the spec save, and the rename — do NOT release early. The existing load is the post-lock re-read.

## 3. spec_complete.go

3.1. Add imports `log/slog`, `time`, `github.com/bborbe/dark-factory/pkg/lock`.

3.2-3.3. Same two fields + constructor params + defaults (`specCompleteCommand`, `NewSpecCompleteCommand`).

3.4. In `Run`, AFTER `path, err := FindSpecFileInDirs(...)` and BEFORE `spec.Load`, acquire `fl := s.dirLockFactory(filepath.Dir(path))` (message "acquire spec complete lock", warn "spec complete: lock release failed"). Hold across load/transition/save/rename.

## 4. spec_unapprove.go

4.1. Add imports `log/slog`, `time`, `github.com/bborbe/dark-factory/pkg/lock` (it already imports `prompt`, `spec`; `filepath` present).

4.2-4.3. Same two fields + constructor params + defaults (`specUnapproveCommand`, `NewSpecUnapproveCommand`).

4.4. In `Run`, AFTER `path, err := FindSpecFile(ctx, s.inProgressDir, id)` and BEFORE `spec.Load`, acquire `fl := s.dirLockFactory(filepath.Dir(path))` (message "acquire spec unapprove lock", warn "spec unapprove: lock release failed"). Hold across load/transition/save/rename-to-inbox AND the `RenumberSpecsAfterRemoval` call — renumber mutates the same source directory, so it MUST run under the lock.

## 5. spec_mark_prompted.go

5.1. Add imports `log/slog`, `time`, `github.com/bborbe/dark-factory/pkg/lock` (it already imports `spec`; `filepath` present).

5.2-5.3. Same two fields + constructor params + defaults (`specMarkPromptedCommand`, `NewSpecMarkPromptedCommand`).

5.4. In `Run`, AFTER `path, err := FindSpecFileInDirs(...)` and BEFORE `spec.Load`, acquire `fl := s.dirLockFactory(filepath.Dir(path))` (message "acquire spec mark-prompted lock", warn "spec mark-prompted: lock release failed"). Hold across the load/status-switch/save. The early `already prompted` return path is INSIDE the lock (after the post-lock load) — that is correct; the defer releases on return.

## 6. Update the factory wiring (ALL FIVE Create* functions)

In `/workspace/pkg/factory/factory.go`, append the two new args to each of the five `cmd.NewSpec*Command(...)` calls. Pass the real defaults so production behavior is the standard lock:
- `CreateSpecApproveCommand`: append `lock.NewDirLock, 0` to `cmd.NewSpecApproveCommand(...)`.
- `CreateSpecUnapproveCommand`: append `lock.NewDirLock, 0`.
- `CreateSpecRejectCommand`: append `lock.NewDirLock, 0`.
- `CreateSpecCompleteCommand`: append `lock.NewDirLock, 0`.
- `CreateSpecMarkPromptedCommand`: append `lock.NewDirLock, 0`.

Confirm `lock` is already imported in factory.go (it is — `lock.NewDirLock` is referenced after prompt 1). The factory functions remain zero-logic (no branches) per `go-factory-pattern.md`; passing `lock.NewDirLock, 0` is construction-only.

## 7. Update the command tests (every constructor call site)

For EACH of the five `spec_*_test.go` files, update EVERY `cmd.NewSpec*Command(...)` call to pass the two new trailing args. Pass `nil, 0` to use the real default lock against the test's temp directories (the temp dirs exist, so the real flock works and exercises the production path). Specifically update:
- `spec_approve_test.go` — both call sites (~line 41 and ~line 127).
- `spec_complete_test.go` — both call sites (~line 44 and ~line 129).
- `spec_reject_test.go` — the call site (~line 51).
- `spec_unapprove_test.go` — the call site (~line 52).
- `spec_mark_prompted_test.go` — both call sites (~line 45 and ~line 229).

7.1. Add at least ONE new test per command that exercises the lock-timeout path (Failure Mode "Lock acquire times out"): inject a `dirLockFactory` that returns a `*mocks.LockDirLock` (the mock renamed in prompt 1) with `AcquireReturns(errors.New("boom"))`, pass a short `lockTimeout`, and assert `Run` returns an error mentioning the command's lock message (e.g. "acquire spec approve lock"). Use `mocks "github.com/bborbe/dark-factory/mocks"` import. This is the boundary test that proves the new lock is actually wired into each command (level-1 coverage of the new seam). Example for approve:
```go
It("returns an error when the lock cannot be acquired", func() {
	fakeLock := &mocks.LockDirLock{}
	fakeLock.AcquireReturns(stderrors.New("boom"))
	cmdWithLock := cmd.NewSpecApproveCommand(
		specsDir, inProgressDir, completedDir, libtime.NewCurrentDateTime(),
		func(string) lock.DirLock { return fakeLock }, 100*time.Millisecond,
	)
	specFile := filepath.Join(specsDir, "001-x.md")
	Expect(os.WriteFile(specFile, []byte("---\nstatus: draft\n---\n# X"), 0600)).To(Succeed())
	err := cmdWithLock.Run(ctx, []string{"001-x.md"})
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("acquire spec approve lock"))
})
```
Add the imports the new test needs to each test file: `time`, `lock "github.com/bborbe/dark-factory/pkg/lock"`, `mocks "github.com/bborbe/dark-factory/mocks"`, and a `stderrors "errors"` alias for the plain error (or use `fmt.Errorf`-free `errors.New` via the stdlib `errors` aliased as `stderrors`, matching the project convention in `/workspace/pkg/queuescanner/scanner.go` which imports `stderrors "errors"`). Adapt the example's args to each command's actual constructor parameter order.

7.2. Add at least ONE new test per command asserting the happy path STILL leaves no `.lock` file in the source directory after a successful mutation (lifecycle precursor):
```go
matches, _ := filepath.Glob(filepath.Join(specsDir, "*.lock"))
Expect(matches).To(BeEmpty())
```
(For commands whose source dir is in-progress, glob that dir instead.) This pins the no-sidecar invariant per command.

## 8. Grep AC

Confirm each of the five spec command files has a lock-acquire call (spec AC):
```bash
grep -n 'DirLock\|Acquire' pkg/cmd/spec_approve.go pkg/cmd/spec_reject.go pkg/cmd/spec_complete.go pkg/cmd/spec_unapprove.go pkg/cmd/spec_mark_prompted.go
```
Each file must show a `dirLockFactory(...)` construction and an `.Acquire(` call.

</requirements>

<constraints>

- Lock the SOURCE-state directory only (`filepath.Dir(path)`); do NOT add a second lock on the destination directory (spec Desired Behavior 6).
- The lock is acquired BEFORE the first `spec.Load` and held across the move-to-new-state operation (and across `RenumberSpecsAfterRemoval` in unapprove). Do NOT release early.
- Preserve the post-lock re-read pattern: the `spec.Load` after acquiring is the authoritative read (spec Constraint).
- The 5-second timeout and 100ms poll interval are unchanged (spec Non-goal). Default `lockTimeout` to `5 * time.Second` when zero.
- No change to public CLI flags, exit codes, or output of any mutation command beyond the new lock behavior (spec Constraint). The success `fmt.Printf` lines stay byte-stable.
- Go has NO default parameters — every caller of the five constructors MUST be updated in THIS prompt (factory.go: 5 sites; test files: all sites enumerated in section 7). Leaving any caller un-updated breaks compilation.
- Factory functions stay zero-logic (no branches/loops) per `go-factory-pattern.md` — just pass `lock.NewDirLock, 0`.
- Errors wrapped with `bborbe/errors` (`errors.Wrap(ctx, err, "...")`) — never `fmt.Errorf`, never `context.Background()` in pkg/. For the plain sentinel in tests, use stdlib `errors.New` via a `stderrors "errors"` alias (project convention).
- BSD-style license header on every modified file must survive (spec Constraint).
- Coverage for `pkg/cmd` must stay >= 80%; the new lock-timeout and no-sidecar tests cover the added paths.
- Do NOT modify `pkg/lock/filelock.go` (owned by prompt 1). If `lock.NewDirLock` / `mocks.LockDirLock` are absent, STOP and report `status: failed` with "DirLock primitive not yet deployed (prompt 1)".
- Do NOT add the cleanup fixer (prompt 4) or a CHANGELOG entry (prompt 4 owns it).
- Do NOT commit — dark-factory handles git.

OPEN QUESTION (resolved here, surfaced for reviewer): the spec does not mandate exposing the lock factory as a constructor parameter, but the reject command precedent does exactly this (nil-defaulting factory + zero-defaulting timeout) and it is the only way to unit-test the timeout path without real contention. This prompt follows that precedent. If the reviewer prefers an internal-only lock (no constructor param), that is a strictly smaller change but loses the timeout unit test — flagged, not chosen.

</constraints>

<verification>

Run from the repo root inside the YOLO container:

```bash
cd /workspace && make test
```

All packages pass. Then the spec ACs:

```bash
cd /workspace
grep -n 'DirLock\|Acquire' pkg/cmd/spec_approve.go pkg/cmd/spec_reject.go pkg/cmd/spec_complete.go pkg/cmd/spec_unapprove.go pkg/cmd/spec_mark_prompted.go   # lock-acquire in each of the 5
go test ./pkg/cmd/... -run 'TestSpec(Approve|Reject|Complete|Unapprove|MarkPrompted)' -v 2>/dev/null | tail -10
go test ./pkg/cmd/... -v 2>/dev/null | grep -iE 'lock|prompted|approve|reject|complete|unapprove' | tail -20
grep -rn 'NewSpecApproveCommand\|NewSpecRejectCommand\|NewSpecCompleteCommand\|NewSpecUnapproveCommand\|NewSpecMarkPromptedCommand' pkg/factory/factory.go   # 5 call sites, each with lock.NewDirLock, 0
```

Then full precommit:

```bash
cd /workspace && make precommit
```

Exit code 0 required. On single-target failure, fix and re-run only that target, then re-run `make precommit` once.

Coverage:
```bash
cd /workspace && go test -coverprofile=/tmp/cover.out ./pkg/cmd/... && go tool cover -func=/tmp/cover.out | tail -1
```
Expect >= 80%.

</verification>
