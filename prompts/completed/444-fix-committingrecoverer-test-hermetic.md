---
status: completed
spec: [093-bug-committingrecoverer-tests-commit-real-repo]
summary: 'Made pkg/committingrecoverer Ginkgo suite hermetic: shared BeforeEach now sandboxes every spec into a temp git repo, added assertNotInRealRepo guard that fails when cwd resolves to the real repo, and added a negative-evidence spec proving the guard fires. All 13 specs pass; make precommit exits 0; hermeticity proof shows zero new commits and unchanged working-tree status with a dirty real repo.'
container: dark-factory-exec-444-fix-committingrecoverer-test-hermetic
dark-factory-version: v0.177.1
created: "2026-06-10T10:00:00Z"
queued: "2026-06-10T09:43:50Z"
started: "2026-06-10T09:46:39Z"
completed: "2026-06-10T10:04:53Z"
---

<summary>

- The committingrecoverer test suite no longer commits the developer's real working tree onto their checked-out branch during `make precommit`.
- Every spec that can reach a package-level git mutation now runs inside an isolated sandbox temp git repo, not the real repository the tests live in.
- Running the suite against a dirty real checkout leaves that checkout's commit history and working-tree status byte-for-byte unchanged.
- A suite-level guard fails the run loudly (non-zero exit, message naming the real-repo path) if any spec ever reaches a git mutation while the working directory is still inside the real repository.
- No production behavior changes: the recoverer still commits the real container working directory at runtime; only the test isolation changes.
- The constructor signature and all production wiring are untouched — this is a test-only fix.
- A cross-package audit confirms no other test package can reach the same escape.

</summary>

<objective>
Make the `pkg/committingrecoverer` Ginkgo suite hermetic with respect to git state so that running it never stages, commits, or reads dirtiness from the real repository it is checked out in. This stops a confirmed data-corruption bug where the suite committed the developer's uncommitted work onto a live PR branch with the message `Test prompt` whenever the tree was dirty mid-`precommit`. A structural guard makes any future regression of this property fail loudly at test time instead of silently committing real work.
</objective>

<context>
Read `/workspace/CLAUDE.md` first for project conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` for the Ginkgo v2 / Gomega conventions used across the repo.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` for `bborbe/errors` usage (no `fmt.Errorf`; pass `ctx`) — relevant only if you add any non-test helper.

Files to read end-to-end before editing:
- `/workspace/specs/in-progress/093-bug-committingrecoverer-tests-commit-real-repo.md` — full spec, especially Goal, Acceptance Criteria, Constraints, and Failure Modes.
- `/workspace/pkg/committingrecoverer/recoverer_test.go` — the entire test file. This is the only file you edit. Note in particular:
  - `initGitRepo(repoDir string)` — the existing sandbox-repo helper (git init + initial commit).
  - The `BeforeEach` in the `Describe("Recoverer", ...)` block — it records `originalDir`, creates `tempDir`, but does NOT `os.Chdir` into a sandbox. This is the root of the bug.
  - The `AfterEach` — it does `os.Chdir(originalDir)` and `os.RemoveAll(tempDir)`.
  - The `Recover` specs (`returns error when MoveToCompleted fails`, `returns error when CommitCompletedFile fails`, `skips work commit and succeeds when no dirty files`, `commits dirty files then succeeds`, `logs warning and continues when spec auto-complete fails`) — each already creates its own `repoDir` under `tempDir`, calls `initGitRepo`, and `os.Chdir`s into it. These are the correct pattern.
  - The `RecoverAll` specs (`stops iteration when ctx is cancelled`, `logs error and continues when Recover fails for one prompt`) — these call `Recover()` (via `RecoverAll`) and therefore reach `git.HasDirtyFiles` / `git.CommitAll`, but they do NOT chdir into a sandbox. These are the leaking specs.
  - The `Describe("Recoverer autoRelease push matrix", ...)` block — a separate `Describe` with its own inline setup that already sandboxes correctly via `initGitRepo` + `os.Chdir`.
- `/workspace/pkg/committingrecoverer/recoverer.go` — the code under test. `Recover()` calls package-level `git.HasDirtyFiles(gitCtx)` and `git.CommitAll(retryCtx, title)` against the process working directory. Do NOT modify this file (see constraints).
- `/workspace/pkg/git/root.go` — `ResolveGitRoot(ctx context.Context) (string, error)` runs `git rev-parse --show-toplevel`. Use this (or an equivalent inline `git rev-parse --show-toplevel` via `exec.Command`) to implement the guard's "is cwd inside the real repo" check.
- `/workspace/pkg/git/git.go` — `HasDirtyFiles(ctx) (bool, error)` and `CommitAll(ctx, message) error` operate on the process cwd; this is why an un-sandboxed spec mutates the real repo.

Reproduction to confirm the bug exists before fixing (run from `/workspace`):
```
echo "// dirty marker" >> mocks/mocks.go
H1=$(git rev-parse HEAD)
go test -count=1 -mod=mod ./pkg/committingrecoverer/...
H2=$(git rev-parse HEAD)
echo "before=$H1 after=$H2"   # before the fix these may differ (a Test prompt commit appeared)
git checkout -- mocks/mocks.go
git reset --soft "$H1" 2>/dev/null || true   # undo any stray commit before you start editing
```
</context>

<requirements>

## Approach decision (already made — do NOT inject a git seam)

Use the **sandbox-every-spec** approach: move the sandbox temp-repo setup into the shared `BeforeEach` so every spec in the `Describe("Recoverer", ...)` block runs with cwd inside an isolated git repo, and add a suite-level guard. Do NOT change `NewRecoverer`'s signature or inject a git/committer seam. Rationale (do not re-litigate): `NewRecoverer` is called from `/workspace/pkg/factory/factory.go` and from three processor test files (`pkg/processor/processor_test.go`, `pkg/processor/processor_retry_test.go`, `pkg/processor/processor_cancel_test.go`); a signature change would ripple through all of them and risk the production recovery path, which the spec constraints forbid touching. The sandbox approach keeps runtime behavior byte-identical and matches the suite's own established `initGitRepo` + `os.Chdir` pattern.

## 1. Move sandbox setup into the shared `BeforeEach`

In `/workspace/pkg/committingrecoverer/recoverer_test.go`, edit the `BeforeEach` of the `Describe("Recoverer", ...)` block so that EVERY spec starts with cwd inside an isolated sandbox git repo created under `tempDir`:

1. Keep recording `originalDir` via `os.Getwd()` and creating `tempDir` via `os.MkdirTemp` (as today).
2. After `completedDir` is created, create a sandbox repo directory under `tempDir` (e.g. `repoDir := filepath.Join(tempDir, "repo")`, `os.MkdirAll(repoDir, 0750)`), call `initGitRepo(repoDir)`, then `Expect(os.Chdir(repoDir)).To(Succeed())`.
3. Store `repoDir` in a closure variable visible to the specs so individual specs that today create their own per-spec repo can either keep doing so or reuse the shared one. Simplest: keep the per-spec repo creation in the existing `Recover` specs untouched (they `os.Chdir` into their own repo, which is still under `tempDir` and still a sandbox), and have the shared `BeforeEach` guarantee that any spec which does NOT create its own repo (the `RecoverAll` specs, the `Load`-error spec) still starts inside a sandbox.

The net effect: after this change there is NO code path through any spec in this `Describe` block where cwd is the real repository when `Recover()` runs.

## 2. Add the suite-level real-repo guard

Add a guard that fails the suite if a spec reaches the point of a git mutation while cwd resolves to a path inside the real repository.

Design (do not deviate from the contract; implementation style is yours):

1. Before any chdir, in the `BeforeEach`, resolve and store the REAL repo root once. From `originalDir` (the cwd at suite start, which is the package source dir inside the real repo), run `git rev-parse --show-toplevel` to get the real repo's absolute toplevel path. You may call `git.ResolveGitRoot(ctx)` (from `/workspace/pkg/git/root.go`) BEFORE chdir-ing, or run `exec.Command("git", "rev-parse", "--show-toplevel")` with `cmd.Dir = originalDir`. Store it in a closure variable `realRepoRoot`.
2. Add a test helper `assertNotInRealRepo(realRepoRoot string)` (a plain function in the test file, using `Expect`/`Fail` from Gomega/Ginkgo). It resolves the CURRENT working directory's git toplevel (run `git rev-parse --show-toplevel` from cwd) and FAILS with `Fail(...)` (or `Expect(...).NotTo(Equal(...))` with a descriptive message) if that toplevel equals `realRepoRoot`. The failure message MUST name the real-repo cwd path so a future developer reading the failure understands the sandbox chdir is missing — e.g. `fmt.Sprintf("committingrecoverer spec ran with cwd inside the real repository (%s); add the sandbox chdir before reaching git.HasDirtyFiles/git.CommitAll", realRepoRoot)`.
3. Call `assertNotInRealRepo(realRepoRoot)` at the START of every spec that reaches `Recover()` (all `RecoverAll` specs and all `Recover` specs except the `Load`-error spec, which returns before any git call — but calling the guard there too is harmless and preferred for uniformity). The cleanest placement: call the guard once at the END of the shared `BeforeEach`, AFTER the shared sandbox chdir from requirement 1. Because the shared `BeforeEach` now always chdirs into a sandbox first, the guard passes for every well-behaved spec and fails only if a future spec or a regression removes the sandbox chdir.
4. The guard must NOT itself mutate any repo. It only reads `git rev-parse --show-toplevel`.

## 3. Demonstrate the guard fires (negative evidence) without leaving it enabled

The spec AC requires evidence that the guard produces a non-zero suite exit when a `Recover()`-reaching spec runs without the sandbox chdir. Satisfy this WITHOUT committing a permanently-failing test:

1. Add a focused unit test (a plain `It` or a small `Describe`) named e.g. `It("guard fails when cwd is the real repo", ...)` that:
   - Resolves `realRepoRoot` from the real repo (via `git.ResolveGitRoot` or `git rev-parse --show-toplevel` with `cmd.Dir = originalDir`).
   - Temporarily `os.Chdir`s BACK into the real repo source dir (the package dir — `originalDir`), making cwd resolve to `realRepoRoot`.
   - Asserts that `assertNotInRealRepo(realRepoRoot)` panics/fails. Because `assertNotInRealRepo` uses Ginkgo `Fail`, capture the failure with Gomega's `Expect(func(){ ... }).To(Panic())` or use `InterceptGomegaFailure(func(){ assertNotInRealRepo(realRepoRoot) })` (from `github.com/onsi/gomega`) and assert the returned error is non-nil and its message contains the real-repo path.
   - Restores cwd to the sandbox before returning so subsequent specs are unaffected (defer `os.Chdir` back to the sandbox `repoDir`, or rely on the next `BeforeEach` to re-chdir).
2. This test proves the guard fires AND that the failure message names the real-repo cwd, satisfying the guard AC, while keeping the suite green.

Prefer `gomega.InterceptGomegaFailure` for capturing the `Fail` — read `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` and existing repo tests for the idiomatic capture form; if `InterceptGomegaFailure` is unavailable in the vendored Gomega version, fall back to making `assertNotInRealRepo` return an `error` (instead of calling `Fail`) and have the shared `BeforeEach` wrap it in `Expect(err).NotTo(HaveOccurred(), <message>)`. Either shape satisfies the AC; pick the one that compiles against the vendored Gomega.

## 4. Fix the leaking `RecoverAll` specs explicitly

The two `RecoverAll` specs that reach `Recover()` — `stops iteration when ctx is cancelled` and `logs error and continues when Recover fails for one prompt` — must run inside the sandbox. After requirement 1 they automatically do (the shared `BeforeEach` chdirs into the sandbox). Confirm by reading each: neither creates its own repo, so before this change both ran against the real repo. After this change both inherit the shared sandbox. Do NOT add per-spec repo creation to them; the shared `BeforeEach` is sufficient.

Note: `stops iteration when ctx is cancelled` cancels ctx on the first `Load` and asserts the loop exits — it may or may not reach `git.HasDirtyFiles` depending on cancellation timing. Either way it must be inside the sandbox, which the shared `BeforeEach` now guarantees.

## 5. Keep the already-correct specs passing unchanged

The `Recover` specs that already create their own `repoDir` + `initGitRepo` + `os.Chdir`, and the entire `Describe("Recoverer autoRelease push matrix", ...)` block, MUST keep passing with no behavioral change. The `autoRelease push matrix` block has its own inline setup and is already hermetic; you may leave it untouched, OR optionally add the same `assertNotInRealRepo` guard call after its `os.Chdir` for defense-in-depth (optional — not required). If you add the guard there, resolve `realRepoRoot` the same way (before its `os.Chdir`).

## 6. Hermeticity self-check inside the suite (optional but recommended)

Optionally add one spec that proves the suite is hermetic by construction: capture the sandbox `repoDir`'s HEAD before and after a `Recover()` call and assert the real repo is never touched. This is optional; the spec's external verification commands (below) are the authoritative AC evidence. Do NOT add a spec that reads or mutates the real repo to prove this.

## 7. Cross-package audit (verification note, no code change)

Run and record the result of:
```
cd /workspace && grep -rln 'git.HasDirtyFiles\|git.CommitAll\|git.CommitWithRetry' pkg/ --include='*_test.go'
```
The result MUST be exactly `pkg/git/git_test.go` (the git package's own unit tests, which sandbox their repos). If any OTHER `_test.go` file appears, STOP and report it — do not attempt to fix a second package in this prompt; note it for a follow-up spec per the spec's Failure Modes table. (At spec-authoring time this grep returned exactly `pkg/git/git_test.go`.)

## 8. Run the hermeticity proof from the spec

After the code change, run the spec's hermeticity proof from `/workspace` and confirm it passes:
```
cd /workspace
echo "// dirty marker" >> mocks/mocks.go
BEFORE_HEAD=$(git rev-parse HEAD)
git status --porcelain > /tmp/status-before.txt
go test -count=1 -mod=mod ./pkg/committingrecoverer/...
AFTER_HEAD=$(git rev-parse HEAD)
git status --porcelain > /tmp/status-after.txt
test "$BEFORE_HEAD" = "$AFTER_HEAD"                  # HEAD unchanged, no new commit
diff /tmp/status-before.txt /tmp/status-after.txt    # empty diff
git log --oneline -5 | grep -c 'Test prompt'         # 0
git checkout -- mocks/mocks.go                         # clean up the marker
```
All three checks must pass: HEAD unchanged, status diff empty, zero `Test prompt` commits. If any new commit appeared, the sandbox chdir is missing from some spec — find it and fix it before declaring done.

</requirements>

<constraints>

- ONLY edit `/workspace/pkg/committingrecoverer/recoverer_test.go`. Do NOT modify `recoverer.go` or any production file.
- The public `Recoverer` interface (`RecoverAll`, `Recover`) and its runtime behavior (commit dirty work files, move prompt to completed, optionally push) MUST NOT change. This is a test-isolation fix, not a runtime-semantics change.
- Do NOT change `NewRecoverer`'s signature or inject a git workdir / committer seam. The production wiring in `pkg/factory/factory.go` and the three processor test files must remain compiling and unchanged.
- The daemon/executor production path MUST continue to commit against the real container working directory at runtime — do not weaken or gate the package-level `git.*` calls in `recoverer.go`.
- The existing `Recover` and `autoRelease push matrix` specs that already sandbox via `initGitRepo` + `os.Chdir` MUST keep passing without behavioral change.
- The placeholder prompt title `Test prompt` in `makePromptFile` may change but changing it is NOT a fix and is not required — leave it as-is unless a test needs a different title.
- The guard must read-only (`git rev-parse --show-toplevel`); it must never stage, commit, or otherwise mutate any repo.
- Do NOT add a permanently-failing test. The negative guard demonstration must capture the failure (via `InterceptGomegaFailure` or an `error`-returning guard) so the suite stays green.
- Do NOT run `go mod vendor` or `go mod tidy` — no new dependencies are added (Ginkgo, Gomega, `os/exec`, `pkg/git` are already imported or in the module).
- File mode `0600` for any new test files; `0750` for directories the test creates. Project convention (matches existing `initGitRepo` usage).
- Branch is `dark-factory/bug-committingrecoverer-tests-commit-real-repo` (from the spec frontmatter). Do not switch branches.
- Tests stay in `package committingrecoverer_test` (external) — match the existing `package` declaration at the top of the file.
- Do NOT commit — dark-factory handles git.

</constraints>

<verification>

Run from the repo root:

```
cd /workspace && make precommit
```

`make precommit` must exit 0.

Hermeticity proof (the spec's authoritative AC evidence):

```
cd /workspace
echo "// dirty marker" >> mocks/mocks.go
BEFORE_HEAD=$(git rev-parse HEAD)
git status --porcelain > /tmp/status-before.txt
go test -count=1 -mod=mod ./pkg/committingrecoverer/...
AFTER_HEAD=$(git rev-parse HEAD)
git status --porcelain > /tmp/status-after.txt
test "$BEFORE_HEAD" = "$AFTER_HEAD" && echo "HEAD-UNCHANGED-OK"
diff /tmp/status-before.txt /tmp/status-after.txt && echo "STATUS-DIFF-EMPTY-OK"
git log --oneline -5 | grep -c 'Test prompt'   # expect 0
git checkout -- mocks/mocks.go
```

Expected: `HEAD-UNCHANGED-OK`, `STATUS-DIFF-EMPTY-OK`, and `0` from the `Test prompt` count.

Spot checks:

```
cd /workspace
grep -n 'assertNotInRealRepo\|realRepoRoot' pkg/committingrecoverer/recoverer_test.go   # >= 3 lines: helper def, BeforeEach resolve, guard call
grep -n 'os.Chdir' pkg/committingrecoverer/recoverer_test.go                            # shared BeforeEach chdir present in addition to per-spec ones
grep -n 'rev-parse --show-toplevel\|ResolveGitRoot' pkg/committingrecoverer/recoverer_test.go  # >= 1 (guard's real-repo resolution)
grep -rln 'git.HasDirtyFiles\|git.CommitAll\|git.CommitWithRetry' pkg/ --include='*_test.go'   # exactly: pkg/git/git_test.go
go test -count=1 -mod=mod ./pkg/committingrecoverer/...                                  # passes, same-or-greater spec count
```

If any spot check fails, any new commit appears in the real repo, or any test fails, fix the gap before declaring done.

</verification>
</content>
</invoke>
