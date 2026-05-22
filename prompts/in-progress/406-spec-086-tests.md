---
status: approved
spec: [086-bug-prompt-move-not-pushed]
created: "2026-05-22T00:00:00Z"
queued: "2026-05-22T18:43:12Z"
branch: dark-factory/bug-prompt-move-not-pushed
---

<summary>
- Ginkgo tests verify each of the four workflow modes (direct, branch, clone, worktree) produces a single commit containing BOTH the code change AND the `R prompts/in-progress/<id>.md → prompts/completed/<id>.md` rename, by inspecting real `git log --name-status` output.
- Each per-mode test description literally contains the substring `move before commit` so the spec's acceptance grep (`ginkgo -v ./pkg/processor/ | grep -iE 'move.*before.*commit'`) returns ≥4 matching lines.
- A failure-mode test asserts that when the work commit fails AFTER the move, the prompt file is rolled back to `prompts/in-progress/<id>.md` with frontmatter `status: committing` and no file remains at `prompts/completed/`.
- A push-failure test asserts that when the push fails AFTER a successful local combined commit, the local commit IS retained (irreversible without manual intervention) and the prompt file stays at `prompts/completed/`.
- A regression test named `bro-20203 lib-crypto-divergence` drives `branchWorkflowExecutor.Complete` end-to-end against a real bare remote + working clone, simulates the PR merge, and asserts `origin/master` shows the prompt only at `completed/`.
- Tests use real `exec.Command("git", ...)` invocations (not the existing `stubWorkflowReleaser`, whose `CommitOnly` returns nil) — real git is the only way to validate rename detection.
- A shared helper `assertSingleCommitWithCodeAndRename(t, repoDir, codeFile, oldPromptPath, newPromptPath)` is extracted to keep the four per-mode tests uniform.
- Coverage check after this prompt: `go test -cover ./pkg/processor/...` shows ≥80% statement coverage on the four executor files modified by prompt 1.
</summary>

<objective>
After this prompt lands, the spec's acceptance criteria for tests are observable: four named tests prove the move-before-commit ordering for each workflow mode, one test proves rollback semantics on commit failure, one proves push-failure semantics, and one named regression test reproduces and verifies the BRO-20203 lib-crypto divergence. Every test uses real git so that rename detection is genuinely exercised — never stubbed.
</objective>

<context>
Read first (no edits yet):

- `CLAUDE.md` — project conventions.
- `~/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega patterns, including `Describe`/`Context`/`It` naming.
- `~/.claude/plugins/marketplaces/coding/docs/test-pyramid-triggers.md` — unit vs integration test selection.
- `specs/in-progress/086-bug-prompt-move-not-pushed.md` — acceptance criteria 4, 5, 6, 8, 9 originate here.
- `prompts/completed/<id-of-prompt-1>` — what prompt 1 changed (after it lands).

Existing test files and their package declarations (do not change package; append):

- `pkg/processor/workflow_executor_direct_test.go` — `package processor` (internal). Append the direct-mode test here.
- `pkg/processor/workflow_executor_branch_test.go` — `package processor_test` (external). EXISTS (≈4.1 KB). Append the branch-mode + BRO-20203 tests here as new `Describe` blocks.
- `pkg/processor/workflow_executor_clone_test.go` — does NOT exist. Create as `package processor_test` (external) to match the branch test's choice.
- `pkg/processor/workflow_executor_worktree_test.go` — does NOT exist. Create as `package processor_test` (external).

Existing helpers worth knowing (locate before reusing):

- `osFileMover` in `pkg/processor/workflow_executor_direct_test.go` — thin wrapper around `os.Rename`. Reuse for the prompt-move step. Do NOT replace with a git-aware mover; that would mask the bug.
- `stubWorkflowReleaser`, `stubAutoCompleter` in `pkg/processor/processor_internal_test.go` — DO NOT reuse `stubWorkflowReleaser.CommitOnly` (returns nil, defeats the point). For these tests, write a new real-git releaser instead (see req 1).
- `pkg/git/brancher_test.go` — shows how to set up a temp git repo with `git init` and real commits. Use the same pattern.

Coding guides:

- `~/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 conventions.
- `~/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — error wrapping in test helpers.
</context>

<requirements>

### 1. Add a shared real-git test helper

In `pkg/processor/workflow_helpers_test.go` (new file, `package processor_test`):

```go
// realGitReleaser implements Releaser using real `git` commands in a working directory.
// Used by tests that need to validate rename detection across the in-progress → completed move.
// Do not reuse stubWorkflowReleaser — its CommitOnly returns nil and does not exercise git.
type realGitReleaser struct {
    workDir string
}

func (r *realGitReleaser) CommitOnly(ctx context.Context, title string) error {
    if err := runGit(ctx, r.workDir, "add", "-A"); err != nil { return err }
    return runGit(ctx, r.workDir, "commit", "-m", title)
}

// Push, PushBranch, etc. as needed — see existing Releaser interface for the full set.
// For tests that don't push (direct/branch), Push can return nil; tests that DO push
// (clone/worktree/BRO-20203) require real `git push <remote> <branch>` against a bare remote.
```

Plus:

```go
// assertSingleCommitWithCodeAndRename asserts that HEAD on repoDir is a single new commit
// containing BOTH a modification of codeFile AND the rename oldPromptPath → newPromptPath.
// Used by all four per-mode tests to enforce the spec's git-shape contract uniformly.
func assertSingleCommitWithCodeAndRename(repoDir, codeFile, oldPromptPath, newPromptPath string) {
    // Run: git log -1 --name-status --format= (porcelain)
    // Assert output contains both:
    //   `M\t<codeFile>` (modification)
    //   `R<NN>\t<oldPromptPath>\t<newPromptPath>` (rename, similarity index ≥ 50%)
    // Also assert: `git ls-tree HEAD <oldPromptPath>` exits non-zero (file absent)
    // Also assert: `git ls-tree HEAD <newPromptPath>` exits zero, exactly one line
}
```

Both helpers must use real `exec.Command("git", ...)` invocations. No stubs.

### 2. Direct mode test — `move before commit (direct)`

Append to `pkg/processor/workflow_executor_direct_test.go` (internal package `processor`):

```go
Describe("directWorkflowExecutor moves prompt before commit", func() {
    It("produces a single commit containing both code change and prompt rename in direct mode (move before commit)", func() {
        // Setup
        repo := setupRealGitRepo()  // git init, initial commit
        promptPath := repo + "/prompts/in-progress/001-test.md"
        completedPath := repo + "/prompts/completed/001-test.md"
        codeFile := repo + "/code.go"
        writePromptFile(promptPath, "committing")
        writeFile(codeFile, "package main\n")

        deps := WorkflowDeps{
            PromptManager: realPromptManager(repo),
            Releaser:      &realGitReleaser{workDir: repo},
            AutoCompleter: stubAutoCompleter{},
            // … other deps stubbed minimally
        }
        executor := newDirectWorkflowExecutor(deps)

        // Execute
        err := executor.Complete(ctx, ctx, pf, "test commit", promptPath, completedPath)
        Expect(err).NotTo(HaveOccurred())

        // Assert
        assertSingleCommitWithCodeAndRename(repo, "code.go",
            "prompts/in-progress/001-test.md",
            "prompts/completed/001-test.md")
    })
})
```

The `It` description MUST literally contain `move before commit` (the spec's acceptance grep is case-insensitive but the substring must be present). Cross-check by running `ginkgo -v ./pkg/processor/ | grep -iE 'move.*before.*commit'` and ensuring this test's description matches.

### 3. Branch mode test — `move before commit (branch)`

Append to `pkg/processor/workflow_executor_branch_test.go` (external package `processor_test` — DO NOT change). Same structure as req 2, but using `branchWorkflowExecutor`:

```go
Describe("branchWorkflowExecutor moves prompt before commit", func() {
    It("produces a single commit on the feature branch containing both code change and prompt rename (move before commit)", func() {
        // Setup: real git repo with default branch master
        // Setup: a feature branch "test-feature"
        // Setup: real Brancher (not the mocks.Brancher) — use exec.Command for real branch ops
        // Write prompt + code, call Complete, assert single combined commit on feature branch
    })
})
```

The `Brancher` mock at `mocks.Brancher` MUST NOT be used here — it makes the test prove nothing about real branch operations. Use real `git checkout -b <feature>` via `exec.Command`.

### 4. Clone mode test — `move before commit (clone)`

Create `pkg/processor/workflow_executor_clone_test.go` (new file, `package processor_test`):

```go
Describe("cloneWorkflowExecutor moves prompt before commit", func() {
    It("produces a single commit pushed to the remote containing both code change and prompt rename (move before commit)", func() {
        // Setup: bare git repo at /tmp/.../remote.git
        // Setup: original working copy cloned from the bare remote, with prompt at prompts/in-progress/001-test.md
        // Setup: cloneWorkflowExecutor configured to clone the remote into a fresh temp dir
        // Write code change in the clone, write prompt file in the ORIGINAL
        // Execute: executor.Complete(...)
        // Assert: bare remote's HEAD on the pushed branch contains both code change and rename, as one commit.
        // Use `git ls-tree origin/<branch>` against the bare remote to assert presence/absence.
    })
})
```

Use real `git clone`, `git push` via `exec.Command`. Do NOT stub the cloner.

### 5. Worktree mode test — `move before commit (worktree)`

Create `pkg/processor/workflow_executor_worktree_test.go` (new file, `package processor_test`). Same shape as req 4 but using `git worktree add` to create the isolated working tree instead of `git clone`.

### 6. Rollback test — `commit fails after move, rolled back`

Append to `pkg/processor/workflow_executor_direct_test.go`:

```go
Describe("directWorkflowExecutor rollback on commit failure", func() {
    It("rolls back the prompt to in-progress with status committing when the work commit fails after move", func() {
        // Setup: real git repo, prompt file, code file
        // Setup: realGitReleaser variant whose CommitOnly returns errors.New("simulated commit failure")
        //        (write a small helper that delegates to real git but returns a stub error)
        // Execute: executor.Complete(...) — expect error
        Expect(err).To(MatchError(ContainSubstring("simulated commit failure")))
        // Assert: file at completedPath does NOT exist
        Expect(fileExists(completedPath)).To(BeFalse())
        // Assert: file at promptPath DOES exist
        Expect(fileExists(promptPath)).To(BeTrue())
        // Assert: frontmatter status is "committing"
        Expect(readPromptStatus(promptPath)).To(Equal("committing"))
    })
})
```

The `It` description MUST literally contain `commit fail` and `rolled back` (so the spec's grep `commit fail.*roll(back|ed)` matches).

### 7. Push-failure test — `push fails after move-commit, local commit retained`

Append to `pkg/processor/workflow_executor_clone_test.go` (since push only happens in clone/worktree):

```go
Describe("cloneWorkflowExecutor push failure after move-commit", func() {
    It("retains the local combined commit and does not roll back the prompt when push fails after move", func() {
        // Setup: real bare remote + clone, prompt + code in clone
        // Setup: Releaser whose Push returns errors.New("simulated push failure")
        // Execute: executor.Complete(...) — expect error
        Expect(err).To(MatchError(ContainSubstring("simulated push failure")))
        // Assert: the local clone's HEAD commit exists and contains both code + rename
        assertSingleCommitWithCodeAndRename(cloneDir, "code.go",
            "prompts/in-progress/001-test.md",
            "prompts/completed/001-test.md")
        // Assert: bare remote does NOT have the commit (push failed)
        Expect(remoteHasCommit(bareDir, branchName)).To(BeFalse())
    })
})
```

The `It` description MUST literally contain `push fail` and `after move` (so the spec's grep `push fail.*after.*move` matches).

### 8. BRO-20203 regression test — `bro-20203 lib-crypto-divergence`

Append to `pkg/processor/workflow_executor_branch_test.go`:

```go
Describe("BRO-20203 regression: lib-crypto-divergence on branch workflow", func() {
    It("after prompt PR merge, origin/master shows prompt only at completed/ not in-progress/ (bro-20203 lib-crypto-divergence)", func() {
        // Setup: bare remote that mirrors lib-crypto's structure (master branch with prompts/in-progress/ tree)
        // Setup: local clone with branchWorkflowExecutor
        // Setup: prompt at prompts/in-progress/001-test.md with status: committing
        // Execute: branchWorkflowExecutor.Complete drives Setup → Complete cycle producing a feature branch + commit
        // Simulate PR merge: in the bare remote, fast-forward master to the feature branch's commit
        //   git --git-dir=<bareDir> branch -f master <feature-sha>
        // Re-fetch in local clone: git fetch origin
        // Assert: `git ls-tree origin/master prompts/in-progress/` does NOT contain the prompt id
        //   (use real `exec.Command` and check exit code)
        // Assert: `git ls-tree origin/master prompts/completed/` contains the prompt id exactly once
    })
})
```

The `It` description MUST contain both `bro-20203` AND `lib-crypto-divergence` so locators can find it via either string. Drive `branchWorkflowExecutor.Complete` end-to-end — do NOT short-circuit by manually constructing the commit.

### 9. Verify coverage and grep matches

After all tests are added, run from the repo root and confirm:

```bash
ginkgo -v ./pkg/processor/ 2>&1 | grep -iE 'move.*before.*commit'   # ≥4 lines (reqs 2-5)
ginkgo -v ./pkg/processor/ 2>&1 | grep -iE 'commit fail.*roll(back|ed)'  # ≥1 line (req 6)
ginkgo -v ./pkg/processor/ 2>&1 | grep -iE 'push fail.*after.*move'      # ≥1 line (req 7)
ginkgo -v ./pkg/processor/ 2>&1 | grep -iE 'bro-20203|lib-crypto-divergence'  # ≥1 line (req 8)
go test -cover ./pkg/processor/                                            # ≥80% on changed code
```

If any grep returns zero lines, the test description does not match the spec — fix the description (not the regex).

### 10. Do NOT modify `mocks/mocks.go` or any file under `mocks/`

These are generated by `go generate`. If a new mock is genuinely needed, add a counterfeiter directive in the source and let `go generate` produce it. For these tests, real-git helpers are sufficient; no new mocks are required.

</requirements>

<constraints>
- Tests MUST use real `exec.Command("git", ...)` for commit / push / branch / clone / worktree operations. Stubbed git defeats the purpose of these tests.
- Test descriptions MUST contain the spec's grep substrings verbatim (`move before commit`, `commit fail`/`rolled back`, `push fail`/`after move`, `bro-20203`/`lib-crypto-divergence`). The greps are the acceptance check; description naming is the protocol.
- New test files MUST use `package processor_test` (external) except for `workflow_executor_direct_test.go` which is `package processor` (internal); append to it without changing the package.
- Do NOT reuse `stubWorkflowReleaser` from `processor_internal_test.go`. Its `CommitOnly` returns nil and does not invoke git.
- Do NOT use `mocks.Brancher` in the new tests — real `git checkout -b` via `exec.Command` is required.
- All existing tests MUST continue to pass.
- Do NOT commit — dark-factory handles git.
- Errors wrap via `errors.Wrap` / `errors.Wrapf`; never `fmt.Errorf`.
- `osFileMover` (real `os.Rename`) is the mover for these tests; do NOT replace it with a git-aware variant — that would mask the rename-detection bug being tested.
</constraints>

<verification>
```bash
make precommit
ginkgo -v ./pkg/processor/ 2>&1 | grep -iE 'move.*before.*commit'   # ≥4 lines
ginkgo -v ./pkg/processor/ 2>&1 | grep -iE 'commit fail.*roll(back|ed)'  # ≥1 line
ginkgo -v ./pkg/processor/ 2>&1 | grep -iE 'push fail.*after.*move'      # ≥1 line
ginkgo -v ./pkg/processor/ 2>&1 | grep -iE 'bro-20203|lib-crypto-divergence'  # ≥1 line
go test -cover ./pkg/processor/ | grep -E 'coverage: [0-9]+\.[0-9]+%'    # ≥80% on changed code
go test ./pkg/prompt/... -count=1                                         # existing prompt tests unchanged
```

`make precommit` must exit 0. Each grep above must return ≥ the indicated number of matching lines, otherwise the test description does not match the spec's acceptance regex and must be reworded.
</verification>
