---
status: prompted
approved: "2026-06-10T09:36:24Z"
generating: "2026-06-10T09:43:50Z"
prompted: "2026-06-10T09:43:50Z"
branch: dark-factory/bug-committingrecoverer-tests-commit-real-repo
---

## Summary

- During `make precommit` in a dark-factory worktree, the committingrecoverer test suite commits the developer's real working tree onto their actual branch with the message `Test prompt`.
- Root cause: several Ginkgo specs reach `Recover()`, which calls package-level `git.HasDirtyFiles` / `git.CommitAll` against the **process working directory** — and those specs do NOT `os.Chdir` into a sandbox repo, so cwd is the real dark-factory checkout.
- Observed twice on real branches (commits `2e57ae5` on 2026-06-08, `a779152` on 2026-06-10), both polluting PR #19 and requiring reset + force-push to clean up.
- Fix: every spec that can reach `git.HasDirtyFiles`/`git.CommitAll` must run inside a sandbox temp git repo, and a regression guard must make the suite fail (not silently commit) if it ever runs with cwd inside the real repo.
- After this fix, running the suite against a dirty real repo produces zero new commits in that repo.

## Problem

The committingrecoverer package retries git commits for prompts stuck in `committing` status by calling package-level git functions that operate on the process working directory. Its test suite exercises `Recover()` from some specs without first chdir-ing into an isolated git repo, so those specs run against the real dark-factory checkout. Mid-`precommit` the real tree is always dirty (counterfeiter `go generate` rewrites `mocks/mocks.go` without its license header before addlicense re-adds it, plus any uncommitted developer edits), so `git.HasDirtyFiles` returns true and `git.CommitAll` commits the entire working tree onto the developer's branch with the test's placeholder title `Test prompt`. This has corrupted a live PR branch twice and silently destroyed in-flight uncommitted work, which is the worst class of test side effect: a test that mutates the developer's own repository.

## Goal

The committingrecoverer test suite is hermetic with respect to git state. No spec in the suite can commit to, or read dirtiness from, the real repository it lives in. Running the suite against a dirty real checkout leaves that checkout's commit history and working-tree status byte-for-byte unchanged. A structural guard makes any future regression of this property fail loudly at test time rather than silently committing real work.

## Acceptance Criteria

- [ ] With the real repo working tree intentionally dirtied, running the suite produces zero new commits in the real repo — evidence: `git rev-parse HEAD` is identical before and after `go test ./pkg/committingrecoverer/...`, captured via `H1=$(git rev-parse HEAD); go test ./pkg/committingrecoverer/... ; H2=$(git rev-parse HEAD); [ "$H1" = "$H2" ]` exits 0.
- [ ] With the real repo working tree intentionally dirtied, the suite does not stage, unstage, or commit the dirty files — evidence: `git status --porcelain` output is byte-identical before and after the test run (negative evidence: `diff <(before) <(after)` is empty).
- [ ] No commit with message `Test prompt` is created in the real repo by the test run — evidence: `git log --oneline --all --since=<run-start>` contains zero lines matching `Test prompt` (negative evidence).
- [ ] Every spec in `pkg/committingrecoverer/recoverer_test.go` that can reach `git.HasDirtyFiles` or `git.CommitAll` runs with cwd set to a sandbox temp git repo — evidence: `go test ./pkg/committingrecoverer/...` passes with all package-level git calls operating only on `os.MkdirTemp` paths; verified by the guard AC below firing if cwd is inside the real repo.
- [ ] A suite-level regression guard fails the suite if any spec reaches a package-level git mutation while cwd resolves to a path inside the real repository — evidence: a test/helper assertion that, when temporarily forced to run a `Recover()`-reaching spec without the sandbox chdir, fails with a message naming the real-repo cwd (demonstrated by `grep -n` on the guard helper returning ≥1, and the guard producing a non-zero suite exit in the negative demonstration).
- [ ] All previously-passing committingrecoverer specs still pass — evidence: `go test ./pkg/committingrecoverer/...` exits 0 with the same or greater spec count.
- [ ] No other test package can reach the same escape — evidence: `grep -rln 'git.HasDirtyFiles\|git.CommitAll\|git.CommitWithRetry' pkg/ --include='*_test.go'` returns exactly `pkg/git/git_test.go` (the git package's own unit tests, which sandbox their repos), and verification notes record that any indirect reach (a test driving a component that calls these functions internally, as committingrecoverer does) is covered by the suite-level guard from the AC above.
- [ ] `make precommit` exits 0 in the dark-factory module — evidence: exit code.

## Verification

```
# 1. Build / lint / full test pass
cd /Users/bborbe/Documents/workspaces/dark-factory
make precommit

# 2. Hermeticity proof: dirty the real repo, run the suite, confirm no commit and no status change
cd /Users/bborbe/Documents/workspaces/dark-factory
echo "// dirty marker" >> mocks/mocks.go            # simulate the mid-precommit dirty state
BEFORE_HEAD=$(git rev-parse HEAD)
git status --porcelain > /tmp/status-before.txt
go test ./pkg/committingrecoverer/...
AFTER_HEAD=$(git rev-parse HEAD)
git status --porcelain > /tmp/status-after.txt
test "$BEFORE_HEAD" = "$AFTER_HEAD"                  # expect: same HEAD, no new commit
diff /tmp/status-before.txt /tmp/status-after.txt    # expect: empty diff
git log --oneline -5 | grep -c 'Test prompt'         # expect: 0
git checkout -- mocks/mocks.go                        # clean up the marker
```

Expected: `make precommit` exits 0; HEAD unchanged; status diff empty; zero `Test prompt` commits.

## Reproduction

dark-factory version: `v0.177.1`

Steps (the suite as it stands today):

1. In a dark-factory worktree, ensure the working tree is dirty (any uncommitted edit, or run mid-`make precommit` where `go generate` has just rewritten `mocks/mocks.go` without its license header).
2. Run `make precommit` (or directly `go test ./pkg/committingrecoverer/...`).
3. The "stops iteration when ctx is cancelled" and "logs error and continues when Recover fails for one prompt" specs (and the `RecoverAll` specs generally) call `Recover()` → `git.HasDirtyFiles(gitCtx)` → `git.CommitAll(retryCtx, title)` with cwd = the real package source dir, because their code path runs under the `BeforeEach` that records `originalDir` and creates a `tempDir` but never `os.Chdir`s into it.
4. `git.CommitAll` stages and commits the entire dirty working tree of the real repo with `title = "Test prompt"` (from `makePromptFile`'s body `# Test prompt\n`).

Observed evidence (verbatim, from the real incidents):

- Commit `2e57ae5` (2026-06-08 17:28): message `Test prompt`; removed the license header from `mocks/mocks.go`.
- Commit `a779152` (2026-06-10 01:06): message `Test prompt`; committed the developer's uncommitted edits (`skills/watch/SKILL.md`, `skills/watch/scripts/watch.sh`) plus a headerless `mocks/mocks.go`.
- Both commits landed on PR #19's branch and were removed via `git reset` + force-push.

## Expected vs Actual

**Expected:** Test specs run in isolation. A unit/spec test never mutates the repository it is checked out in. Package-level git functions that read/mutate cwd are exercised only against sandbox temp repos. This is the implicit contract every test suite is held to, and the explicit pattern the suite already follows in the `Recover` and `autoRelease push matrix` specs via `initGitRepo(repoDir)` + `os.Chdir(repoDir)`.

**Actual:** `recoverer.go` lines 102-109 call package-level `git.HasDirtyFiles` / `git.CommitAll` against the process cwd (the factory comment at lines 41-43 states these are "used directly (not injected) because extracting a git-wrapper is out of scope"). The `BeforeEach` in `recoverer_test.go` (lines 138-155) does not chdir into a sandbox; only individual `Recover` specs do (lines 225-226, 246-247, 267-268, 293, 313-314, 377-378). The `RecoverAll` specs that reach `Recover()` therefore run against the real repo and commit its dirty tree.

## Why this is a bug

A test mutating the developer's real repository — staging and committing uncommitted in-flight work onto a live PR branch — is data corruption and history pollution caused by the test harness itself. It contradicts the suite's own established isolation pattern (the `Recover` specs sandbox via `initGitRepo` + `os.Chdir`) and the universal invariant that running tests is side-effect-free on the checkout. Confirmed by code reading and reproduced twice with verbatim commit evidence above.

## Workaround

Until the fix lands: ensure the working tree is fully clean (commit or stash all changes) before running `make precommit` in any dark-factory worktree, so `git.HasDirtyFiles` returns false and `git.CommitAll` is never reached. This is fragile because `go generate` transiently dirties `mocks/mocks.go` during `precommit` itself, so even a clean starting tree is not a guarantee.

## Constraints

- The public `Recoverer` interface (`RecoverAll`, `Recover`) and its observable behavior at runtime (commit dirty work files, move prompt to completed, optionally push) MUST NOT change. This bug is about test isolation and/or seam injection, not runtime semantics.
- The daemon/executor wiring that constructs the Recoverer via `pkg/factory` MUST continue to commit against the real container working directory at runtime — that is the intended production behavior. The fix must not break the real recovery path.
- If the structural option is chosen (inject a git workdir or a narrow committer seam into `NewRecoverer`), all existing callers in `pkg/factory` MUST be updated in the same change and the runtime path must still operate on the real cwd.
- The existing `Recover` and `autoRelease push matrix` specs that already sandbox via `initGitRepo` + `os.Chdir` MUST keep passing without behavioral change.
- The placeholder prompt title `Test prompt` is internal to the test (`makePromptFile`); it is not a frozen interface and may change, but changing it is NOT a fix — it would only change the commit message, not stop the commit.

## Failure Modes

| Trigger | Detection | Expected behavior | Reversibility | Recovery |
|---------|-----------|-------------------|---------------|----------|
| Real repo tree is dirty when a `Recover()`-reaching spec runs (the bug) | Guard assertion fires; suite exits non-zero naming the real-repo cwd | Suite fails loudly before any commit; no mutation of the real repo | Reversible (nothing committed) | Developer reads the guard message and fixes the spec's sandbox setup; no git cleanup needed |
| A new spec is added later that reaches `git.HasDirtyFiles`/`git.CommitAll` without sandboxing | Guard assertion fires at that spec's runtime | Suite fails with a message naming the offending cwd | Reversible | Author adds the sandbox chdir (or uses the injected seam) and re-runs |
| Sandbox `initGitRepo` fails to create the temp repo (disk full, permissions) | `os.MkdirTemp`/git init returns error; spec fails in setup | Spec fails in `BeforeEach`/setup, never reaching a real-repo git call | Reversible | Operator frees disk / fixes permissions and re-runs |
| `os.Chdir` back to `originalDir` fails in cleanup (concurrent dir removal) | `AfterEach` chdir returns error (currently swallowed) | Subsequent specs may run from the temp dir; guard still prevents real-repo commits because cwd is not inside the real repo | Reversible | Re-run the suite in a fresh process |
| A package other than committingrecoverer calls package-level `git.*` cwd functions from tests without sandboxing | `grep -rln` audit during verification surfaces it | That package is either confirmed safe (cannot reach a git mutation) or flagged for the same fix in a follow-up | n/a | File a follow-up spec if a second offender is confirmed |

## Suggested Decomposition

Single code layer (one test file, optionally one constructor + its factory callers). The work is small but has two viable approaches and a guard; one prompt is sufficient. The prompt-creator picks the sandbox-only vs. inject-seam approach at implementation time — both satisfy every AC.

| # | Prompt focus | Covers ACs | Depends on |
|---|---|---|---|
| 1 | Make the committingrecoverer suite hermetic (sandbox every `Recover()`-reaching spec via `BeforeEach` chdir, OR inject a git workdir/committer seam into `NewRecoverer` and mock it), add the suite-level cwd-inside-real-repo guard, audit other `git.*` cwd callers in tests | 1-8 | — |

Rationale: the two fix options are mutually exclusive implementation choices for the same behavioral outcome, so they belong in one prompt, not two. The guard and the cross-package audit are cheap additions that ride along in the same change and share the same test file.

## Do-Nothing Option

If we do not fix this, the suite continues to commit the developer's uncommitted working tree onto whatever branch is checked out, with message `Test prompt`, every time `make precommit` runs against a dirty tree — which is the normal state mid-`precommit` because `go generate` transiently strips the license header from `mocks/mocks.go`. It has already corrupted PR #19 twice and silently destroyed in-flight uncommitted edits. The cost compounds across every contributor and every worktree, and the failure is silent (a successful-looking test run that quietly mutated the repo). Not acceptable.
