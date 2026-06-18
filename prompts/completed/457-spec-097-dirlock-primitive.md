---
status: failed
spec: [097-parent-dir-flock-locking]
container: dark-factory-exec-457-spec-097-dirlock-primitive
dark-factory-version: v0.181.0
created: "2026-06-18T12:10:00Z"
queued: "2026-06-18T12:05:42Z"
started: "2026-06-18T12:05:44Z"
completed: "2026-06-18T12:26:42Z"
branch: dark-factory/parent-dir-flock-locking
lastFailReason: 'validate completion report: completion report status: partial'
---

<summary>

- The file-locking primitive in `pkg/lock` switches from creating a per-file `<file>.lock` sidecar to locking the file's parent directory directly. No `.lock` files are ever created on disk again.
- The interface is renamed `FileLock` -> `DirLock` and the constructor `NewFileLock(path)` -> `NewDirLock(dirPath)`. The constructor now takes a directory path and `flock`s that directory's file descriptor.
- Two mutators targeting files inside the same directory serialize; two mutators in different directories run in parallel.
- Crash-release works automatically: when a process holding the lock dies, the kernel drops the flock and the next acquire succeeds with nothing to clean up.
- Three new unit tests are added: same-directory serialization, different-directory parallelism, and crash-release (close fd without unlock, second acquire still succeeds, no `.lock` artifact left behind).
- The 5-second timeout and 100ms poll interval are unchanged. The "acquire by polling until success / timeout / ctx-cancel" timing is identical to today.
- The counterfeiter mock is regenerated under the new interface name. Callers in `pkg/cmd`, `pkg/queuescanner`, `pkg/doctor` will be migrated in later prompts — this prompt leaves them temporarily referencing the old names, so this prompt must keep the package compiling on its own by also updating every in-repo reference to the renamed symbols.

</summary>

<objective>
Refactor `pkg/lock` so the lock primitive acquires an exclusive advisory `flock` on the parent directory of the protected file instead of on a `<file>.lock` sidecar. Rename the public interface `FileLock` -> `DirLock` and the constructor `NewFileLock(path string)` -> `NewDirLock(dirPath string)`. No `.lock` sidecar files are created anywhere. Add three new unit tests proving same-dir serialization, different-dir parallelism, and crash-release.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first for YOLO container conventions, then `/workspace/CLAUDE.md` for project conventions.

Read these coding-plugin docs before editing:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — public interface + private struct + `New*` constructor, counterfeiter annotations, error wrapping with `bborbe/errors`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega, coverage >=80%, error-path testing
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `errors.Wrap(ctx, err, "...")`, `errors.Errorf(ctx, ...)`, never `fmt.Errorf`, never `context.Background()` in pkg/

Read these source files end-to-end before editing:
- `/workspace/pkg/lock/filelock.go` — the current implementation being refactored. Note: it builds `path + ".lock"`, opens it `O_CREATE|O_RDWR 0600`, `syscall.Flock(LOCK_EX|LOCK_NB)`, polls every 100ms, 5s default timeout, releases by closing the fd (never removes the sidecar — the comment block at the bottom of `Release` explains the TOCTOU rationale that this spec removes). Also contains `FileLockPath(targetPath string) string` and `EnsureDirExists(ctx, path)` helpers.
- `/workspace/pkg/lock/filelock_test.go` — the current Ginkgo suite. The current tests construct `lock.NewFileLock(lockPath)` where `lockPath` is `filepath.Join(tempDir, "test.lock")`.
- `/workspace/spec/in-progress/097-parent-dir-flock-locking.md` if present, else `/workspace/specs/in-progress/097-parent-dir-flock-locking.md` — the parent spec; read `Desired Behavior` 1-4, `Constraints`, and the `Failure Modes` table.

Current interface (verbatim, from `/workspace/pkg/lock/filelock.go`):
```go
type FileLock interface {
	Acquire(ctx context.Context, timeout time.Duration) error
	Release(ctx context.Context) error
}

func NewFileLock(path string) FileLock {
	return &fileLock{lockPath: path + ".lock"}
}
```

The counterfeiter directive in the current file is:
```go
//counterfeiter:generate -o ../../mocks/lock-file-lock.go --fake-name LockFileLock . FileLock
```

In-repo references to the symbols being renamed (found via grep — every one must continue to compile after this prompt; the production callers in cmd/queuescanner/doctor are migrated in prompts 2 and 3, but this prompt MUST leave the whole module building, so update every reference listed below as part of THIS prompt):
- `/workspace/pkg/cmd/reject.go` — field `fileLockFactory func(path string) lock.FileLock`, default `lock.NewFileLock`
- `/workspace/pkg/queuescanner/scanner.go` — field `fileLockFactory func(path string) lock.FileLock`, default `lock.NewFileLock`
- `/workspace/pkg/doctor/fixer.go` — field `FileLockFactory func(path string) lock.FileLock`, default `lock.NewFileLock`
- `/workspace/pkg/doctor/releaselock.go` — `func releaseLock(ctx context.Context, fl lock.FileLock, path string)`
- `/workspace/pkg/factory/factory.go` — three `lock.NewFileLock` references (around lines 961, 1197, 1529)
- `/workspace/mocks/lock-file-lock.go` — counterfeiter-generated mock (regenerated by `make generate`)
- test files: `/workspace/pkg/doctor/fixer_test.go`, `/workspace/pkg/queuescanner/scanner_test.go`, `/workspace/pkg/lock/filelock_test.go`

</context>

<requirements>

## 1. Rewrite the lock primitive to lock the parent directory

In `/workspace/pkg/lock/filelock.go`:

1.1. Rename the interface `FileLock` -> `DirLock`. Update its doc comment to describe directory-scoped locking: it serializes mutations to any file inside a status directory (e.g. `prompts/in-progress/`), so concurrent processes cannot interleave mid-mutation. Keep the two methods `Acquire(ctx context.Context, timeout time.Duration) error` and `Release(ctx context.Context) error` with the same signatures. Update the `Release` doc comment — it no longer removes any file; it closes the directory fd (which drops the flock) and is idempotent.

1.2. Update the counterfeiter directive to:
```go
//counterfeiter:generate -o ../../mocks/lock-dir-lock.go --fake-name LockDirLock . DirLock
```

1.3. Rename the constructor `NewFileLock(path string) FileLock` -> `NewDirLock(dirPath string) DirLock`. It stores the directory path (no `+ ".lock"` suffix). Doc comment: "NewDirLock creates a DirLock that acquires an exclusive advisory flock on dirPath itself. dirPath must be a directory that exists at Acquire time; pass `filepath.Dir(targetFile)` to serialize mutations of files in that directory."

1.4. Rename the private struct `fileLock` -> `dirLock`. Replace its `lockPath string` field with `dirPath string`. Keep the `mu sync.Mutex` and `fd *os.File` fields and their existing concurrency rationale comments (the in-process mutex still guards `fd` against concurrent `Acquire`/`Release` on the same instance).

1.5. In `tryAcquire`, open the DIRECTORY read-only instead of creating a sidecar:
```go
// #nosec G304 -- path is a caller-supplied directory path; open is O_RDONLY, no create, no write
fd, err := os.OpenFile(f.dirPath, os.O_RDONLY, 0)
if err != nil {
	return errors.Wrap(ctx, err, "open lock directory")
}
```
Keep the existing `syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)` call and its `//nolint:gosec` comment unchanged. On flock failure, `_ = fd.Close()` then return the wrapped error as today. Keep the `f.mu.Lock()/defer f.mu.Unlock()` guard and the `if f.fd != nil { return nil }` early-return exactly as today.

1.6. In `Acquire`, keep the polling loop unchanged: 100ms ticker, deadline = `ctx.Deadline()` or `time.Now().Add(timeout)`, ctx-cancel returns `errors.Wrap(ctx, ctx.Err(), "acquire lock cancelled")`, timeout returns a wrapped error. Update the timeout error message to name the directory:
```go
return errors.Errorf(ctx, "lock acquire timeout: %s", f.dirPath)
```

1.7. Rewrite `Release` to close the fd only (no file removal). Keep the existing ordering invariants comment but DELETE the steps about `os.Remove` of the lock file and the TOCTOU rationale block (those no longer apply — there is no sidecar). The body: take ownership of `fd` under the mutex, null `f.fd`, `if fd == nil { return nil }`, then `fd.Close()` (closing drops the flock per `man flock(2)` on Linux and macOS) wrapping a close error with `errors.Wrap(ctx, err, "close lock directory")`. Idempotent — second call returns nil.

1.8. DELETE the `FileLockPath(targetPath string) string` function entirely — it constructs a `<path>.lock` name and is part of the abandoned sidecar scheme. Grep first to confirm no production caller uses it:
```bash
grep -rn "FileLockPath" /workspace --include="*.go"
```
If any non-test caller exists, migrate it to use `filepath.Dir(targetPath)` directly and note it in the report; do not leave a dangling reference.

1.9. KEEP `EnsureDirExists(ctx context.Context, path string) error` unchanged — callers rely on it to create a status directory before locking it (Failure Mode "Status directory does not exist at lock-acquire time").

1.10. Preserve the BSD-style license header (lines 1-3) verbatim.

## 2. Update every in-repo reference to the renamed symbols so the module compiles

The rename of `FileLock`/`NewFileLock` breaks references in production and test files. This prompt MUST leave `make test` green for the whole module. The production callers in `pkg/cmd/reject.go`, `pkg/queuescanner/scanner.go`, `pkg/doctor/fixer.go`, and `pkg/doctor/releaselock.go` are functionally migrated to pass `filepath.Dir(...)` in prompts 2 and 3; in THIS prompt, perform only the mechanical type/constructor rename in those files so they compile (do NOT add `filepath.Dir(...)` call-site changes here — that is prompts 2/3's job):

2.1. In `/workspace/pkg/cmd/reject.go`: rename the struct field type `func(path string) lock.FileLock` -> `func(path string) lock.DirLock`, the parameter type in `NewRejectCommand` likewise, and the default `lock.NewFileLock` -> `lock.NewDirLock`. (The call site `r.fileLockFactory(path)` is left as-is in this prompt; prompt 2 changes it to `r.fileLockFactory(filepath.Dir(path))`.)

2.2. In `/workspace/pkg/queuescanner/scanner.go`: same mechanical rename — field type, `NewScanner` parameter type, default `lock.NewFileLock` -> `lock.NewDirLock`. Leave `s.fileLockFactory(pr.Path)` call site for prompt 2.

2.3. In `/workspace/pkg/doctor/fixer.go`: rename `FileLockFactory func(path string) lock.FileLock` -> `FileLockFactory func(path string) lock.DirLock` and default `lock.NewFileLock` -> `lock.NewDirLock`. Keep the field name `FileLockFactory` (renaming the exported field is prompt 2's decision; keep it stable here to minimize churn). Leave `f.deps.FileLockFactory(path)` call sites for prompt 2.

2.4. In `/workspace/pkg/doctor/releaselock.go`: rename the parameter type `fl lock.FileLock` -> `fl lock.DirLock`.

2.4b. In `/workspace/pkg/doctor/fix_renumber.go`: mechanically rename the slice element type `[]lock.FileLock` -> `[]lock.DirLock` at lines 46, 110, 137. Leave the call sites' arguments unchanged (prompt 2 handles the `filepath.Dir(...)` migration of the renumber-fixer call sites). Confirm with `grep -n 'lock.FileLock\|lock.DirLock' /workspace/pkg/doctor/fix_renumber.go` post-edit — every hit must be `lock.DirLock`.

2.5. In `/workspace/pkg/factory/factory.go`: replace each `lock.NewFileLock` (3 sites, around lines 961, 1197, 1529) with `lock.NewDirLock`.

2.6. Regenerate the counterfeiter mock. The old `/workspace/mocks/lock-file-lock.go` must be replaced by `/workspace/mocks/lock-dir-lock.go` with fake name `LockDirLock`. Run:
```bash
cd /workspace && make generate
```
Then delete the now-stale `/workspace/mocks/lock-file-lock.go` if `make generate` did not remove it (counterfeiter writes the new file but does not delete the old one):
```bash
rm -f /workspace/mocks/lock-file-lock.go
```

2.7. In test files that reference the old mock type, rename `mocks.LockFileLock` -> `mocks.LockDirLock` and `lock.FileLock` -> `lock.DirLock` and `lock.NewFileLock` -> `lock.NewDirLock`. The affected test files (confirm with grep, fix every hit):
```bash
grep -rln "LockFileLock\|NewFileLock\|FileLock\b\|FileLockPath" /workspace --include="*.go"
```
Specifically (verified line numbers; the grep above will catch any drift):
- `/workspace/pkg/doctor/fixer_test.go` lines 71, 604: `&mocks.LockFileLock{}` -> `&mocks.LockDirLock{}`
- `/workspace/pkg/queuescanner/scanner_test.go` lines 422, 426, 926, 933, 1105: rename `mocks.LockFileLock` / `lockpkg.FileLock` / `lock.NewFileLock` per the rename table above

For test call sites that construct the lock against a file path (e.g. `lock.NewFileLock(promptPath)`), change them to `lock.NewDirLock(filepath.Dir(promptPath))` so they exercise the new directory semantics; add the `path/filepath` import where missing. Do NOT delete or weaken the existing concurrency assertions in those tests — only adapt construction.

## 3. Rewrite the lock test suite

Rewrite `/workspace/pkg/lock/filelock_test.go` to test the directory-locking primitive. Keep the package `lock_test` and the BSD license header.

3.1. In `BeforeEach`, create a temp directory and use it directly as the lock target (no `.lock` filename):
```go
tempDir = GinkgoT().TempDir()
```
Construct locks via `lock.NewDirLock(tempDir)`.

3.2. Adapt the existing behavioral tests to directory locking:
- "acquires lock on an existing directory" — `lock.NewDirLock(tempDir).Acquire(ctx, 5s)` succeeds; release.
- "fails to acquire when the same directory is already locked (non-blocking)" — two `DirLock`s on the SAME `tempDir`; first acquires, second `Acquire(ctx, 100ms)` errors.
- "respects context cancellation" — short ctx timeout returns error.
- "fails when timeout is exceeded while waiting" — first holds `tempDir`, second `Acquire(ctx, 200ms)` errors after >= ~180ms.
- "Release allows another to acquire" — first acquire+release on `tempDir`, second acquire on `tempDir` succeeds.
- "Release succeeds when not holding the lock" — release without acquire returns nil.
- "Acquire fails when the directory does not exist" — NEW assertion for Failure Mode "Status directory does not exist": `lock.NewDirLock(filepath.Join(tempDir, "does-not-exist")).Acquire(ctx, 200ms)` returns an error whose message references the missing path.

3.3. Add the THREE new tests the spec acceptance criteria name (use these exact `It` description strings so the AC grep finds them):

(a) `It("TestParentDirLock_SerializesSameDir: two goroutines locking the same directory observe serial ordering", func() {...})` — two goroutines each `NewDirLock(tempDir)`, acquire, append a marker to a shared slice under no extra mutex (the flock IS the mutex), sleep ~30ms inside the critical section, release. Assert that the two critical sections did NOT overlap. Implementation hint: have each goroutine record `enter`/`exit` timestamps; after both finish, assert one goroutine's `exit` is <= the other's `enter` (no interleaving). Use a buffered channel or `sync.WaitGroup` to join. Gate the test on `runtime.GOOS == "linux" || runtime.GOOS == "darwin"` (flock semantics), mirroring the existing gate pattern in `/workspace/pkg/queuescanner/scanner_test.go` (search for `runtime.GOOS`).

(b) `It("TestParentDirLock_ParallelDifferentDirs: two goroutines locking different directories make parallel progress", func() {...})` — create two sub-directories under `tempDir` (`os.MkdirAll(dirA, 0750)`, `os.MkdirAll(dirB, 0750)`). Two goroutines lock `dirA` and `dirB` respectively, each holding for ~100ms. Assert wall-clock for both running concurrently is well under the serialized sum (e.g. total elapsed < 180ms, proving they overlapped). Same OS gate as (a).

(c) `It("TestParentDirLock_CrashReleaseLeavesNoArtifact: a dropped fd auto-releases and leaves no .lock file", func() {...})` — acquire a `DirLock` on `tempDir`, then simulate process death by releasing it via the normal `Release` (closing the fd drops the kernel flock; this is the in-process stand-in for crash-release since a real kill is not feasible in a unit test). Then a SECOND `NewDirLock(tempDir).Acquire(ctx, 1s)` must return nil within the timeout. Additionally assert that `tempDir` contains zero `*.lock` files:
```go
matches, globErr := filepath.Glob(filepath.Join(tempDir, "*.lock"))
Expect(globErr).NotTo(HaveOccurred())
Expect(matches).To(BeEmpty())
```
Document in a code comment that closing the fd is the in-process proxy for kernel crash-release: both paths drop the per-fd flock identically (`man flock(2)`); the assertion that matters is "no on-disk artifact + next Acquire succeeds".

3.4. Imports the suite will need: `context`, `os`, `path/filepath`, `runtime`, `sync`, `time`, the ginkgo/gomega dot-imports, and `github.com/bborbe/dark-factory/pkg/lock`. Confirm a `lock_suite_test.go` already runs the Ginkgo suite (it does — `make test` currently runs this package); if no suite bootstrap file exists, add one mirroring `/workspace/pkg/queuescanner/scanner_suite_test.go`.

## 4. Verify the no-sidecar invariant in production code

After the rewrite, run the spec's grep AC and confirm no production code constructs a `.lock` sidecar name:
```bash
grep -rEn '"\.lock"|\+ *"\.lock"' /workspace/pkg/lock/ --include='*.go' --exclude='*_test.go'
```
Expected: empty output (the cleanup fixer that legitimately references `.md.lock` is added in prompt 4, not here).

</requirements>

<constraints>

- The 5-second default timeout and 100ms poll interval are UNCHANGED. Do not introduce a configurable interval or timeout knob — spec Non-goal "Does NOT change the 5-second timeout or 100ms poll interval".
- Do NOT add a fallback or opt-out to the old sidecar scheme — the directory-flock scheme is the only scheme post-change (spec Non-goal).
- Do NOT reintroduce per-file locking granularity as a "best of both worlds" option — coarser-by-design is an invariant (spec Non-goal).
- Do NOT change `pkg/containerslot/manager.go` — different lock scope, out of scope (spec Non-goal).
- The `DirLock` abstraction MUST remain — deleting the interface entirely is not allowed (spec Constraint). Keep public interface + private struct + `New*` constructor + counterfeiter annotation (per `go-patterns.md`).
- Errors MUST be wrapped with `errors.Wrap(ctx, err, "...")` / `errors.Errorf(ctx, ...)` from `github.com/bborbe/errors` — never `fmt.Errorf`, never `context.Background()` in pkg/ (per `go-error-wrapping-guide.md`).
- macOS and Linux must both work — `syscall.Flock` on a directory fd is advisory, per-fd, kernel-released on both. Gate the OS-specific concurrency tests with `runtime.GOOS == "linux" || runtime.GOOS == "darwin"`.
- The BSD-style license header on every modified file must survive the edit (spec Constraint; per `go-licensing-guide.md`).
- Coverage for `pkg/lock` must stay >= 80% (per `definition-of-done.md`). Error paths (open failure, flock failure, timeout, ctx-cancel) must be tested.
- Do NOT commit — dark-factory handles git.
- This prompt is the foundational primitive; prompts 2 and 3 depend on the renamed interface being stable. Do NOT change the method signatures `Acquire(ctx, timeout)` / `Release(ctx)` — only the type name, constructor name, and constructor parameter semantics change.

</constraints>

<verification>

Run from the repo root inside the YOLO container:

```bash
cd /workspace && make test
```

All packages must pass. Then run the focused lock suite and the spec ACs:

```bash
cd /workspace
go test ./pkg/lock/... -v
go test ./pkg/lock/... -run 'TestParentDirLock' -v 2>/dev/null || true   # informational; Ginkgo It-strings carry the names
grep -n 'TestParentDirLock_SerializesSameDir' pkg/lock/filelock_test.go       # >=1 match
grep -n 'TestParentDirLock_ParallelDifferentDirs' pkg/lock/filelock_test.go   # >=1 match
grep -n 'TestParentDirLock_CrashReleaseLeavesNoArtifact' pkg/lock/filelock_test.go  # >=1 match
grep -rn 'NewDirLock\|DirLock' pkg/lock/filelock.go        # interface + constructor renamed
grep -rn 'NewFileLock\|FileLock\b\|LockFileLock\|FileLockPath' /workspace --include='*.go'   # 0 matches anywhere
grep -rEn '"\.lock"|\+ *"\.lock"' pkg/lock/ --include='*.go' --exclude='*_test.go'   # 0 matches
ls /workspace/mocks/lock-dir-lock.go     # new mock exists
ls /workspace/mocks/lock-file-lock.go    # must NOT exist (deleted)
```

Then run full precommit:

```bash
cd /workspace && make precommit
```

Exit code 0 required. If `make precommit` fails on a single target (lint/gosec/errcheck), fix it and re-run only that target until green, then re-run `make precommit` once.

Coverage check for the changed package:
```bash
cd /workspace && go test -coverprofile=/tmp/cover.out -mod=vendor ./pkg/lock/... && go tool cover -func=/tmp/cover.out | tail -1
```
Expect total >= 80%.

</verification>
