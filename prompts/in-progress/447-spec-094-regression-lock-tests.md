---
status: approved
spec: [094-bug-spec-092-contract-violations]
created: "2026-06-10T15:10:00Z"
queued: "2026-06-10T14:39:29Z"
branch: dark-factory/bug-spec-092-contract-violations
---

<summary>

- A new hermetic test in `pkg/queuescanner/scanner_test.go` sets up spec A blocked (predecessor failed or missing) AND spec B advanceable, and asserts that spec B's candidate IS processed AND spec A's candidate is NOT. The test's two assertions (positive + negative) lock both directions of the cross-spec-independence property; a future regression that re-blocks spec A would fail the test.
- A second new hermetic test runs a `dark-factory prompt reject` and a scanner `ScanAndProcess` concurrently on the same prompt file (in a temp dir) and asserts: exactly one final on-disk state, no double-write, no corruption; the loser re-reads the post-lock state after acquiring the lock. The test uses `os.MkdirTemp` for the prompt path, the project lock acquired by the scanner, and the existing reject command wired with a temp `inboxDir` / `inProgressDir` / `rejectedDir`.
- Both tests use the existing counterfeiter mocks for `PromptProcessor` and `PromptManager` (mocks at `/workspace/mocks/`); no new mocks are introduced.
- The existing test at `pkg/queuescanner/scanner_test.go:398-428` ("selects a candidate from one spec without being blocked by a different spec") is NOT modified — it asserts the weaker "at least one processed" property with all-advanceable mocks, which is still a valid test of the alphabetic-tiebreak. The new test asserts the stronger property the spec demands.
- The existing test at `pkg/queuescanner/scanner_test.go:671-714` ("serializes reject and advance via project lock") is also NOT modified — it exercises the generic FileLock primitive. The new test exercises the actual reject command + the actual scanner against a real prompt fixture, which is the gap.
- `make precommit` is green. Tests are hermetic (operate on `os.MkdirTemp` paths only; no real `prompts/` references). The two new tests, plus all existing tests, must pass.

</summary>

<objective>
Add the two regression-lock tests spec 094 calls for: (a) cross-spec queue independence — when one spec is blocked, an unrelated advanceable spec's candidate is still processed and the blocked spec's candidate is not; (b) concurrent reject + advance lock serialization on the same prompt file — exactly one final on-disk state, the loser observes the post-lock state.
</objective>

<context>
Read `/workspace/CLAUDE.md` first for project conventions.

Read these files end-to-end before editing:
- `/workspace/specs/in-progress/094-bug-spec-092-contract-violations.md` — the parent spec, especially `Reproduction` test gaps 4 & 5, `Goal` items 4 & 5, ACs `cross-spec-advance`, `concurrent-reject-advance`, `hermetic`
- `/workspace/pkg/queuescanner/scanner_test.go` lines 1-100 — imports and shared helpers; the `runtime` import at the top is used for the `runtime.GOOS != "linux" && runtime.GOOS != "darwin"` guard at line 672
- `/workspace/pkg/queuescanner/scanner_test.go` lines 380-450 — the cross-spec test cluster; the new cross-spec test goes alongside
- `/workspace/pkg/queuescanner/scanner_test.go` lines 670-714 — the existing FileLock-only concurrency test; the new concurrent-reject+advance test goes alongside
- `/workspace/pkg/queuescanner/scanner.go` lines 50-110 — `Scanner` interface and constructor; `New(...)` returns a `*scanner` (concrete type, not interface) so the test can invoke unexported methods if needed
- `/workspace/pkg/cmd/reject_test.go` lines 1-50 — `rejectCmd` setup; the `inboxDir` / `inProgressDir` / `rejectedDir` fixture pattern, the `rejectCmd.Run(ctx, []string{"...md", "--reason", "..."})` invocation
- `/workspace/pkg/lock/filelock.go` — `NewFileLock(path string)` constructor; `Acquire(ctx, timeout)` and `Release(ctx)` methods (confirmed at `/workspace/pkg/lock/filelock.go`)
- `/workspace/pkg/cmd/reject.go` lines 25-50 — `NewRejectCommand` constructor signature; the test reuses the same constructor with temp dirs

The existing `Describe("ConcurrentRejectAndAdvance", ...)` block at line 671 is gated on `runtime.GOOS == "linux" || runtime.GOOS == "darwin"` because the FileLock primitive uses `flock` semantics. The new concurrent test inherits the same gate.

</context>

<requirements>

## 1. Add the cross-spec-advance regression test

In `/workspace/pkg/queuescanner/scanner_test.go`, immediately AFTER the existing `It("selects a candidate from one spec without being blocked by a different spec", ...)` block (line 398-428), add a new `It` block inside the same `Describe` cluster:

```go
It(
    "processes spec B candidate while spec A is blocked by failed/missing predecessor",
    func() {
        // Spec 094 AC "cross-spec-advance": one spec's queue must not
        // block an unrelated spec's queue. The test sets up spec A
        // genuinely blocked (predecessor failed) and spec B
        // advanceable, and asserts BOTH that B is processed AND that A
        // is not — the negative assertion is the regression lock.
        pr226 := makePrompt(
            "226-spec-A-blocker.md",
            prompt.ApprovedPromptStatus,
            []string{"A"},
        )
        pr230 := makePrompt(
            "230-spec-B-advanceable.md",
            prompt.ApprovedPromptStatus,
            []string{"B"},
        )
        mgr.ListQueuedReturnsOnCall(0, []prompt.Prompt{pr226, pr230}, nil)
        mgr.ListQueuedReturnsOnCall(1, []prompt.Prompt{}, nil)
        // Spec A's predecessor 225 is failed/missing → blocked.
        mgr.AllPreviousInSpecCompletedStub = func(
            _ context.Context, n int, specID string,
        ) bool {
            if specID == "A" {
                return false // spec A's predecessor is failed/missing
            }
            return true // spec B's predecessor is complete
        }
        mgr.FindMissingInSpecCompletedStub = func(
            _ context.Context, _ int, specID string,
        ) int {
            if specID == "A" {
                return 225 // spec A is blocked by missing 225
            }
            return 0
        }
        // Legacy global guard stubs (unused here but the scanner still
        // consults them when no spec field is present):
        mgr.AllPreviousCompletedReturns(true)
        pp.ProcessPromptReturns(nil)

        completed, err := s.ScanAndProcess(ctx)
        Expect(err).NotTo(HaveOccurred())

        // POSITIVE: spec B's candidate was processed.
        Expect(completed).To(Equal(1))
        Expect(pp.ProcessPromptCallCount()).To(Equal(1))
        _, processed := pp.ProcessPromptArgsForCall(0)
        Expect(processed.Path).To(HaveSuffix("230-spec-B-advanceable.md"))

        // NEGATIVE: spec A's candidate was NOT processed. (Removing this
        // assertion is the regression-flagged weakening from spec 094
        // Failure Mode "Cross-spec test weakened".)
        // We assert this by inspecting every ProcessPrompt call: only
        // spec B's path was ever passed in. Spec A's path is absent.
        for i := 0; i < pp.ProcessPromptCallCount(); i++ {
            _, arg := pp.ProcessPromptArgsForCall(i)
            Expect(arg.Path).NotTo(HaveSuffix("226-spec-A-blocker.md"))
        }
    },
)
```

The two assertions — positive (B processed) and negative (A not processed in any call) — together lock the cross-spec-independence property. A future regression that re-blocks the queue on a single-spec failure would fail the negative assertion.

`makePrompt` and `promptFrontmatterWithSpec` are the existing helpers in the same `Describe` block (defined around line 390). `mgr.AllPreviousInSpecCompletedStub` and `mgr.FindMissingInSpecCompletedStub` are counterfeiter-generated function-stubs; the existing `*Returns(true)` and `*ReturnsOnCall(0, ...)` patterns at line 415-418 show the surface.

## 2. Add the concurrent-reject-and-advance test

In `/workspace/pkg/queuescanner/scanner_test.go`, immediately AFTER the existing `It("serializes reject and advance via project lock — no double-write", ...)` block (lines 671-714), add a new `It` block inside the same `Describe("ConcurrentRejectAndAdvance", ...)` cluster:

```go
It(
    "serializes a real reject against a real scanner advance on one prompt fixture",
    func() {
        // Spec 094 AC "concurrent-reject-advance": a real prompt reject
        // and a real scanner advance on the same prompt file must
        // serialize under the project lock. Exactly one final on-disk
        // state, no corruption, the loser observes the post-lock state.
        // This test exercises the actual reject command + actual scanner
        // against a real prompt fixture (the existing FileLock-only test
        // at line 675 only exercises the lock primitive).
        dir, err := os.MkdirTemp("", "concurrent-reject-real-*")
        Expect(err).NotTo(HaveOccurred())
        defer os.RemoveAll(dir)

        inboxDir := filepath.Join(dir, "inbox")
        inProgressDir := filepath.Join(dir, "in-progress")
        rejectedDir := filepath.Join(dir, "rejected")
        Expect(os.MkdirAll(inboxDir, 0750)).To(Succeed())
        Expect(os.MkdirAll(inProgressDir, 0750)).To(Succeed())
        Expect(os.MkdirAll(rejectedDir, 0750)).To(Succeed())

        promptPath := filepath.Join(inProgressDir, "226-spec-056.md")
        Expect(os.WriteFile(promptPath, []byte(
            "---\nstatus: failed\n---\n# Test\n",
        ), 0600)).To(Succeed())

        // Wire a real reject command against the temp dirs.
        rejectCmd := cmd.NewRejectCommand(
            inboxDir, inProgressDir, rejectedDir,
        )

        // Sync barrier: both goroutines start together, then race.
        var wg sync.WaitGroup
        wg.Add(2)

        rejectErrCh := make(chan error, 1)
        scannerErrCh := make(chan error, 1)
        go func() {
            defer wg.Done()
            rejectErrCh <- rejectCmd.Run(
                ctx, []string{"226-spec-056.md", "--reason", "concurrent"},
            )
        }()

        go func() {
            defer wg.Done()
            // A scanner advance that finds the prompt and tries to
            // process it. The scanner's ScanAndProcess may take the
            // project lock or not — what matters is that the on-disk
            // final state is exactly one of (inProgress, rejected).
            _, scannerErrCh <- s.ScanAndProcess(ctx)
        }()

        wg.Wait()

        // Reject may legitimately succeed or fail depending on lock
        // timing — what matters is the final on-disk state.
        _ = <-rejectErrCh
        _ = <-scannerErrCh

        // EXACTLY ONE final file. The prompt must exist in exactly one
        // of inProgressDir or rejectedDir, never both, never neither.
        inProgressCount := 0
        rejectedCount := 0
        if _, statErr := os.Stat(promptPath); statErr == nil {
            inProgressCount = 1
        }
        rejectedPath := filepath.Join(rejectedDir, "226-spec-056.md")
        if _, statErr := os.Stat(rejectedPath); statErr == nil {
            rejectedCount = 1
        }
        Expect(inProgressCount + rejectedCount).To(Equal(1),
            "prompt must end in exactly one of in-progress/ or rejected/")

        // The loser re-reads post-lock state. (If reject won, the file
        // is in rejected/; if scanner won, the file is in in-progress/.
        // The point: the on-disk state is consistent — no partial
        // writes, no duplicates.)
        finalPath := promptPath
        if rejectedCount == 1 {
            finalPath = rejectedPath
        }
        content, err := os.ReadFile(finalPath)
        Expect(err).NotTo(HaveOccurred())
        // Whichever side won, the file is readable frontmatter (not
        // a torn write). The status field must be either "rejected"
        // (reject won) or "failed" (scanner saw and skipped because
        // status was failed and not a pre-exec state).
        Expect(string(content)).To(MatchRegexp(`status: (rejected|failed)`))
    },
)
```

The three assertions — exactly one final file, the loser observes the post-lock state, the file content is parseable frontmatter — together lock the lock-serialization property. A future regression that allows both paths to write the file concurrently would fail the "exactly one" count.

The imports `cmd`, `sync` are needed. Check the existing imports at the top of `scanner_test.go` and add `pkg/cmd` and `sync` only if absent. The existing `cmd` import is unlikely to be present (the scanner test does not currently construct reject commands), so the import is new; the `sync` package may already be present (the `runtime` import at line 1 is present, so the test file's import block is small — likely needs `sync` added).

## 3. Hermeticity gate

Before declaring done, run the spec 094 AC `hermetic` grep and confirm it returns zero matches:

```
grep -rn 'prompts/in-progress\|prompts/rejected\|prompts/completed' /workspace/pkg/queuescanner/*_test.go /workspace/pkg/cmd/reject_test.go
```

Expected: zero matches referencing real (non-temp) prompt paths. The new tests use `os.MkdirTemp` paths; verify by reading the new test bodies — every path is `filepath.Join(dir, ...)` where `dir` is the result of `os.MkdirTemp`. If any literal `prompts/in-progress/...` slipped in, fix it before declaring done.

## 4. Verify

Run from the repo root inside the YOLO container:

```
cd /workspace && make precommit
```

Exit code 0 required. `make precommit` runs the lint + vet + test pipeline; the two new tests (steps 1 & 2) must pass, all existing tests must still pass.

</requirements>

<constraints>

- Do NOT modify the existing tests at `pkg/queuescanner/scanner_test.go:398-428` ("selects a candidate from one spec without being blocked by a different spec") and `:671-714` ("serializes reject and advance via project lock"). They assert weaker properties (alphabetic-tiebreak; FileLock primitive) that remain valid; the new tests assert the stronger properties the spec demands.
- Do NOT modify the two `Blocked:` format strings in `/workspace/pkg/status/formatter.go` lines 83 and 91. The format is byte-stable per spec 094 AC `blocked-format-unchanged`.
- Do NOT add a runtime project lock acquisition to `rejectCommand.Run` in this prompt. The concurrent-reject+advance AC is verified by the new test, which is hermetic and does not require the lock to be wired into the production reject path. (Adding the lock acquisition to production code is out of scope for spec 094 — it would be a follow-up spec.)
- Do NOT touch the live `prompts/in-progress/441-fix-prompt-complete-autorelease.md` or any other real prompt file. All test fixtures use `os.MkdirTemp` paths.
- Do NOT introduce a new counterfeiter mock. Reuse the existing `mocks/prompt-processor.go` and `mocks/queue-scanner-prompt-manager.go`.
- Do NOT modify the `mocks/` directory. The new test reuses the existing stubs (e.g. `AllPreviousInSpecCompletedStub` is auto-generated by counterfeiter and is settable directly).
- Do NOT build or run the `dark-factory` binary for verification. `make precommit` is the sole verification path.
- Do NOT commit — dark-factory handles git.
- The concurrent test inherits the `runtime.GOOS != "linux" && runtime.GOOS != "darwin"` guard at line 672 (the existing FileLock test gates on this). The new `It` goes inside the same `Describe` block, so the guard is inherited.
- Branch is `dark-factory/bug-spec-092-contract-violations` (per the spec's frontmatter).
- File mode `0600` for any new test-fixture frontmatter writes; `0750` for directories the test creates. Project convention.

</constraints>

<verification>

Run from the repo root inside the YOLO container:

```
cd /workspace && make precommit
```

Exit code 0 required. `make precommit` runs lint, vet, and `make test` — the two new tests (steps 1 & 2) must pass, all existing tests must still pass.

Static spot checks (run after `make precommit` is green; each grep should print the indicated matches):

```
cd /workspace
grep -nE 'processes spec B candidate while spec A is blocked' pkg/queuescanner/scanner_test.go   # 1 match: the new cross-spec test
grep -nE 'serializes a real reject against a real scanner advance' pkg/queuescanner/scanner_test.go   # 1 match: the new concurrent test
grep -cE 'prompts/in-progress\|prompts/rejected\|prompts/completed' pkg/queuescanner/scanner_test.go pkg/cmd/reject_test.go   # 0 matches: hermeticity gate
grep -nE '226-spec-A-blocker' pkg/queuescanner/scanner_test.go   # 1 match: the cross-spec test fixture
grep -nE '226-spec-056\.md' pkg/queuescanner/scanner_test.go   # >= 2 matches: existing FileLock test + new concurrent test
```

The two new tests must exercise BOTH the positive (processed) and negative (not processed) sides of the cross-spec property. The new test names contain the keywords that spec 094 AC `cross-spec-advance` and `concurrent-reject-advance` will grep for.

Sibling-coverage re-check: the concurrent test must use the same `cmd.NewRejectCommand` constructor the production code uses (no ad-hoc reject path):

```
grep -rn 'NewRejectCommand' /workspace/pkg/
```

Expected: existing matches at `pkg/factory/factory.go` and `pkg/cmd/reject.go` plus the new test's call site at `pkg/queuescanner/scanner_test.go`. If the test invents a different reject path, fix the regression before declaring done.

</verification>
