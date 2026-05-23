---
status: approved
spec: [087-bug-clone-worktree-move-not-applied-to-original]
created: "2026-05-23T00:00:00Z"
queued: "2026-05-23T09:14:52Z"
branch: dark-factory/bug-clone-worktree-move-not-applied-to-original
---

<summary>
- Unit tests for `syncPromptFileToOriginalRepo` cover three branches: (a) destination already exists → idempotent no-op; (b) source present and destination absent → file moved; (c) both source AND destination absent → returns `clone-sync-mismatch` error.
- Failure-injection unit test: when the destination path already exists AS A DIRECTORY, `os.Rename` errors with `EISDIR`. The test asserts that `cloneWorkflowExecutor.Complete` returns nil (success-with-warning) AND the captured log contains `clone-sync-mismatch`. Injection is robust under root.
- Integration test (clone): drives `cloneWorkflowExecutor.Complete` end-to-end against a real bare remote + clone, then asserts: (a) original's `prompts/completed/<id>.md` exists; (b) original's `prompts/in-progress/<id>.md` does NOT exist; (c) `git -C <original> log origin/master..HEAD --oneline` returns empty (no local commits).
- Integration test (worktree): same shape using `worktreeWorkflowExecutor.Complete`.
- Branch + direct existing tests are NOT modified. The spec-086 invariant test (`branchWorkflowExecutor moves prompt before commit`) keeps asserting tip-commit shape unchanged.
- All test descriptions literally contain the spec's grep substrings: `sync prompt file to original repo`, `clone-sync-mismatch`, `idempotent`.
- Unit tests for the unexported helper live in NEW file `pkg/processor/workflow_helpers_internal_test.go` (`package processor`). Integration tests APPEND to existing `workflow_executor_clone_test.go` and `workflow_executor_worktree_test.go` (`package processor_test`).
</summary>

<objective>
After this prompt lands, ginkgo tests prove: (a) the new helper behaves correctly across its three branches; (b) the failure path returns success-with-warning and emits the right log line; (c) clone + worktree workflows produce the correct ORIGINAL-repo state and zero local commits; (d) direct + branch workflows are unchanged.
</objective>

<context>
Read first (no edits yet):

- `CLAUDE.md` — project conventions.
- `~/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega patterns.
- `specs/in-progress/087-bug-clone-worktree-move-not-applied-to-original.md` — the spec's ACs are the source of truth.
- `prompts/in-progress/<id>-bug-087-sync-prompt-file-to-original-repo.md` — sibling implementation prompt; introduces `syncPromptFileToOriginalRepo` (unexported) in `pkg/processor/workflow_helpers.go`.

Existing test files and packages (do NOT change their package declarations; append where needed):

- `pkg/processor/workflow_executor_direct_test.go` — `package processor` (internal). Helpers in this file: `realGitReleaser`, `osFileMover`, `setupRealGitRepo`, `writePromptFile`, `writeFile`, `readPromptStatus`, `fileExists`. DO NOT modify this file.
- `pkg/processor/workflow_executor_branch_test.go` — `package processor_test` (external). DO NOT modify this file.
- `pkg/processor/workflow_executor_clone_test.go` — `package processor_test` (external). Has `setupBareRemoteWithClone`. APPEND new `Describe` blocks here.
- `pkg/processor/workflow_executor_worktree_test.go` — `package processor_test` (external). Has `setupBareRemoteWithWorktree`. APPEND new `Describe` blocks here.
- `pkg/processor/workflow_helpers_internal_test.go` — does NOT exist. CREATE as `package processor` (internal) for unexported-helper unit tests.

Helper-visibility rule: `syncPromptFileToOriginalRepo` is unexported, so its unit tests MUST live in `package processor`. Integration tests in `package processor_test` exercise the helper INDIRECTLY via the public `Complete` method.
</context>

<requirements>

### 1. Create `pkg/processor/workflow_helpers_internal_test.go`

Package: `package processor` (internal — required because the helper is unexported).

Add a single `Describe` block: `syncPromptFileToOriginalRepo`. Inside it, three `It` blocks:

#### 1a. "is idempotent when destination already exists"

The `It` description MUST contain `idempotent`.

Setup: temp dir with `prompts/in-progress/` and `prompts/completed/` directories. Write a file at `completedPath`. Leave `promptPath` absent.

Construct a real `*prompt.Manager` using `prompt.NewManager(...)` with `&osFileMover{}` (the one defined in `workflow_executor_direct_test.go`, accessible because both files are `package processor`).

Call `syncPromptFileToOriginalRepo(ctx, mgr, promptPath, completedPath)`. Assert:

```go
Expect(err).NotTo(HaveOccurred())
Expect(fileExists(completedPath)).To(BeTrue())  // destination still there
```

#### 1b. "moves file from in-progress to completed when source exists"

Setup: file at `promptPath` with `status: committing` frontmatter; no file at `completedPath`.

Call the helper. Assert:

```go
Expect(err).NotTo(HaveOccurred())
Expect(fileExists(promptPath)).To(BeFalse())
Expect(fileExists(completedPath)).To(BeTrue())
Expect(readPromptStatus(completedPath)).To(Equal("completed"))
```

#### 1c. "returns clone-sync-mismatch error when both source and destination are absent"

The `It` description MUST contain `clone-sync-mismatch`.

Setup: directories exist; both `promptPath` and `completedPath` absent.

```go
err := syncPromptFileToOriginalRepo(ctx, mgr, promptPath, completedPath)
Expect(err).To(HaveOccurred())
Expect(err.Error()).To(ContainSubstring("clone-sync-mismatch"))
Expect(err.Error()).To(ContainSubstring(promptPath))
Expect(err.Error()).To(ContainSubstring(completedPath))
```

### 2. Append failure-injection test to `pkg/processor/workflow_executor_clone_test.go`

Describe block: `cloneWorkflowExecutor sync failure`. One `It` block:

#### 2a. "emits clone-sync-mismatch WARN and returns success when sync fails after push"

The `It` description MUST literally contain `clone-sync-mismatch`.

Setup steps:

1. `bareDir, originalDir := setupBareRemoteWithClone(GinkgoT())`.
2. In `originalDir`, create `prompts/in-progress/001-test.md` with `status: committing` frontmatter.
3. In `originalDir`, pre-create `prompts/completed/001-test.md` AS A DIRECTORY:
   ```go
   completedPath := filepath.Join(originalDir, "prompts", "completed", "001-test.md")
   Expect(os.MkdirAll(completedPath, 0750)).To(Succeed())  // EISDIR injection
   ```
4. Install a capturing slog handler:
   ```go
   logBuf := &bytes.Buffer{}
   prevDefault := slog.Default()
   slog.SetDefault(slog.New(slog.NewTextHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))
   DeferCleanup(func() { slog.SetDefault(prevDefault) })
   ```
5. chdir into `originalDir` (and `DeferCleanup` restores the previous CWD).
6. Build a `cloneWorkflowExecutor` using the same wiring pattern as the existing `cloneWorkflowExecutor moves prompt before commit` test in this file (inline construction is fine — do NOT extract a new helper).
7. Call `executor.Complete(ctx, ctx, pf, "test commit", promptPath, completedPath)`.

Assertions:

```go
Expect(err).NotTo(HaveOccurred(), "Complete MUST return nil (success-with-warning), NOT propagate the rename error")
logs := logBuf.String()
Expect(logs).To(ContainSubstring("clone-sync-mismatch"))
Expect(logs).To(ContainSubstring(promptPath))
Expect(logs).To(ContainSubstring(completedPath))
// Remote was pushed successfully despite local sync failure (push happens BEFORE the sync attempt):
out, gErr := exec.CommandContext(ctx, "git", "-C", bareDir, "branch", "--list", "dark-factory/001-test").CombinedOutput()
Expect(gErr).NotTo(HaveOccurred())
Expect(strings.TrimSpace(string(out))).NotTo(BeEmpty())
```

### 3. Append integration test to `pkg/processor/workflow_executor_clone_test.go`

Describe block: `cloneWorkflowExecutor syncs prompt file to original repo`. One `It` block:

#### 3a. "syncs prompt file to original repo after successful push (sync prompt file to original repo)"

The `It` description MUST literally contain `sync prompt file to original repo`.

Setup:

1. `bareDir, originalDir := setupBareRemoteWithClone(GinkgoT())`.
2. In `originalDir`, create `prompts/in-progress/001-test.md` with `status: committing` frontmatter.
3. In `originalDir`, write `code.go` (the work file).
4. chdir into `originalDir` (DeferCleanup restores).
5. Build a `cloneWorkflowExecutor` with the same wiring pattern as the existing `moves prompt before commit` test in this file.

Execute:

```go
err := executor.Complete(ctx, ctx, pf, "test commit", promptPath, completedPath)
Expect(err).NotTo(HaveOccurred())
```

Assert ORIGINAL-repo state:

```go
// (a) completed/ file present in ORIGINAL
_, err = os.Stat(completedPath)
Expect(err).NotTo(HaveOccurred(), "completed file MUST exist in ORIGINAL after sync")

// (b) in-progress/ file absent in ORIGINAL
_, err = os.Stat(promptPath)
Expect(os.IsNotExist(err)).To(BeTrue(), "in-progress file MUST NOT exist in ORIGINAL after sync")

// (c) ORIGINAL has NO local commits ahead of origin/master (filesystem-only sync, no second commit)
out, err := exec.CommandContext(ctx, "git", "-C", originalDir, "log", "origin/master..HEAD", "--oneline").CombinedOutput()
Expect(err).NotTo(HaveOccurred())
Expect(strings.TrimSpace(string(out))).To(BeEmpty(), "ORIGINAL repo MUST NOT have local commits ahead of origin/master after sync")
```

### 4. Append integration test to `pkg/processor/workflow_executor_worktree_test.go`

Same as req 3 but driving `worktreeWorkflowExecutor.Complete` and using `setupBareRemoteWithWorktree`. `It` description MUST contain `sync prompt file to original repo`.

### 5. Append failure-injection test to `pkg/processor/workflow_executor_worktree_test.go`

Same as req 2 but driving `worktreeWorkflowExecutor.Complete`. `It` description MUST contain `clone-sync-mismatch`.

### 6. Do NOT modify `workflow_executor_direct_test.go` or `workflow_executor_branch_test.go`

These files cover direct + branch workflows, which are out of scope for spec 087. Touching them risks regressing spec-086 invariants. Verify with `git diff HEAD -- pkg/processor/workflow_executor_direct_test.go pkg/processor/workflow_executor_branch_test.go` (must be empty after this prompt).

### 7. Verify grep gates and run tests locally before declaring done

After all tests are added:

```bash
ginkgo -v --dry-run ./pkg/processor/ 2>&1 | grep -iE 'sync prompt file to original repo'   # ≥2 lines (clone + worktree integration)
ginkgo -v --dry-run ./pkg/processor/ 2>&1 | grep -iE 'clone-sync-mismatch'                  # ≥3 lines (unit 1c + clone failure + worktree failure)
ginkgo -v --dry-run ./pkg/processor/ 2>&1 | grep -iE 'idempotent'                            # ≥1 line (unit 1a)
ginkgo -v --dry-run ./pkg/processor/ 2>&1 | grep -iE 'move.*before.*commit'                  # ≥4 lines (spec-086 invariant unchanged: direct, branch, clone, worktree)
go test ./pkg/processor/ -count=1 -timeout 120s
```

If any grep returns fewer matches than indicated, fix the test description (not the regex).

</requirements>

<constraints>
- Tests MUST use real `exec.Command("git", ...)` for git assertions. No stubs for git verifications.
- Failure injection MUST use directory-at-destination (`os.MkdirAll(completedPath, 0750)`) so `os.Rename` errors with `EISDIR`. DO NOT use `os.Chmod 0555` (silently passes under root in CI containers).
- Log capture MUST use a real `slog.Handler` (e.g. `slog.NewTextHandler` writing to a `bytes.Buffer`) with `slog.SetDefault`. Restore the previous default in `DeferCleanup`.
- The unit-test file `workflow_helpers_internal_test.go` MUST be `package processor` (internal).
- Integration-test additions MUST keep their existing `package processor_test` declarations.
- All test descriptions that the spec greps for MUST literally contain the substring (case-insensitive OK): `sync prompt file to original repo`, `clone-sync-mismatch`, `idempotent`.
- DO NOT modify `workflow_executor_direct_test.go` or `workflow_executor_branch_test.go`.
- DO NOT modify `mocks/`.
- The spec-086 invariant tests (`directWorkflowExecutor moves prompt before commit`, `branchWorkflowExecutor moves prompt before commit`, `cloneWorkflowExecutor moves prompt before commit`, `worktreeWorkflowExecutor moves prompt before commit`) MUST all still pass unchanged.
- Errors wrap via `errors.Wrap` / `errors.Wrapf` if any new error wrapping happens; never `fmt.Errorf`.
- Do NOT commit — dark-factory handles git.
</constraints>

<verification>
```bash
make precommit

go test ./pkg/processor/ -count=1 -timeout 120s
ginkgo -v --dry-run ./pkg/processor/ 2>&1 | grep -iE 'sync prompt file to original repo'   # ≥2 lines
ginkgo -v --dry-run ./pkg/processor/ 2>&1 | grep -iE 'clone-sync-mismatch'                  # ≥3 lines
ginkgo -v --dry-run ./pkg/processor/ 2>&1 | grep -iE 'idempotent'                            # ≥1 line
ginkgo -v --dry-run ./pkg/processor/ 2>&1 | grep -iE 'move.*before.*commit'                  # ≥4 lines (spec-086 invariant)

# Regression guard: direct + branch test files unchanged
git diff HEAD pkg/processor/workflow_executor_direct_test.go pkg/processor/workflow_executor_branch_test.go  # empty diff

# Existing prompt-package tests unaffected
go test ./pkg/prompt/... -count=1
```

`make precommit` MUST exit 0. Each grep MUST return at least the indicated number of lines.

Live verification (scenario 002 replay against `/tmp/new-dark-factory`) is the spec-verification step performed at spec-complete time per `docs/spec-verification.md`. It is intentionally NOT part of this prompt's verification.
</verification>
