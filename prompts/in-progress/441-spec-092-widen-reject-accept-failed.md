---
status: failed
spec: [092-daemon-blocked-queue-ux]
container: dark-factory-blocked-queue-exec-441-spec-092-widen-reject-accept-failed
dark-factory-version: v0.174.1-dirty
created: "2026-06-02T19:24:09Z"
queued: "2026-06-02T20:16:12Z"
started: "2026-06-02T20:17:26Z"
completed: "2026-06-02T20:25:06Z"
branch: dark-factory/daemon-blocked-queue-ux
lastFailReason: 'validate completion report: completion report status: failed'
---

<summary>

- `dark-factory prompt reject NNN --reason "<text>"` accepts prompts whose frontmatter is `status: failed` (currently rejected with "cannot reject" error).
- The widened path preserves the pre-execution semantics for `idea` / `draft` / `approved` — same file move, same exit code, same console output.
- On a `failed` reject, the file's frontmatter is rewritten with three new fields written in a single YAML pass: `status: rejected`, `originalStatus: failed`, and `rejectedReason: <text>`. The two new fields use the `originalStatus` (camelCase) and `rejected_reason` (snake_case — already present) YAML tags.
- The rewrite path is YAML-safe for hostile reasons: a reason containing `:` and `\n` is quoted by the YAML marshaller; the frontmatter round-trips identically. Verified by a unit test that round-trips `"a: b\nc"` through `pf.Save` → `pf.Load` and asserts equality.
- The widened reject path is mid-action crash-safe: if the process dies between `os.Rename` and `pf.Save`, a re-run of `prompt reject NNN --reason "<text>"` completes the frontmatter rewrite idempotently and exits 0 (failure mode 5 in spec).
- Adds a new helper method on `*prompt.PromptFile` (`StampRejectedWithOriginal(reason, originalStatus string)`) that the new reject path calls instead of `StampRejected(reason)`. The existing `StampRejected` stays unchanged so the `idea`/`draft`/`approved` path is a single-line edit.
- New `OriginalStatus` field on `prompt.Frontmatter` with the `yaml:"originalStatus,omitempty"` tag. The struct tag is `omitempty` so prompts rejected from pre-execution states (where the field would be empty) do not see a new line in their frontmatter.
- Coverage: ginkgo specs cover each reject-from path (idea/draft/approved/failed) plus a "reject-of-failed-preserves-original" round-trip test, a "re-run after partial move is idempotent" test, and a hostile-reason test asserting frontmatter parse round-trip via `pm.Load`.
- No new CLI flag, no new top-level subcommand, no change to `AvailablePromptStatuses`.

</summary>

<objective>
Widen `dark-factory prompt reject` so that prompts in the `failed` state can be moved to `rejected/` with `dark-factory prompt reject NNN --reason "<text>"`, recording the original `failed` status and the operator's reason in frontmatter. Eliminate the manual `git mv failed→completed` workaround that today corrupts the audit trail.
</objective>

<context>
Read `/workspace/CLAUDE.md` first for project conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` for the Interface → Constructor → Struct → Method convention.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` for the `bborbe/errors` API (`errors.Wrap`, `errors.Errorf`, never `fmt.Errorf`, always pass `ctx`).
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` for Ginkgo v2 / Gomega patterns and Counterfeiter mock reuse.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` for `Create*` factory wiring.

Files to read end-to-end before editing:
- `/workspace/specs/in-progress/092-daemon-blocked-queue-ux.md` — full spec, especially `Desired Behavior #2` and AC `reject-accepts-failed`, `reject-preserves-pre-exec`, `frontmatter-safety`
- `/workspace/pkg/cmd/reject.go` lines 49-100 — current `Run` + `rejectByID` + `IsRejectable()`-driven pre-execution check; the widening touches lines 76-82 (the `IsRejectable()` gate)
- `/workspace/pkg/cmd/reject_test.go` — existing ginkgo tests to mirror; the new path extends the same `Describe` blocks
- `/workspace/pkg/prompt/prompt.go` lines 50-77 — `PromptStatus` constants and `AvailablePromptStatuses` (do NOT modify)
- `/workspace/pkg/prompt/prompt.go` lines 244-263 — `Frontmatter` struct; new `OriginalStatus` field is added here
- `/workspace/pkg/prompt/prompt.go` lines 164-168 — `IsRejectable()` definition (do NOT modify; the widening is in the cmd layer)
- `/workspace/pkg/prompt/prompt.go` lines 442-449 — `StampRejected(reason string)` method that the new helper will be modelled on
- `/workspace/pkg/prompt/prompt.go` lines 347-375 — `Save()` method; the YAML safety guarantee lives here (calls `yaml.Marshal` which auto-quotes strings with `:` or newlines)
- `/workspace/pkg/prompt/prompt.go` lines 569-575 — `Specs()` accessor (used in prompt 2 to read the spec id for per-spec predecessor lookup; do NOT modify here)
- `/workspace/pkg/factory/factory.go` line 1377-1395 — `CreateRejectCommand` factory; the constructor signature does NOT change (widen is a pure logic change inside `Run`)
- `/workspace/main.go` line 363 — `runPromptCommand` switch case for `reject` (no change needed; subcommand surface stays the same)
- `/workspace/pkg/cmd/prompt_finder.go` — `FindPromptFileInDirs(ctx, id, dirs ...string)` signature; variadic — the widening adds `r.rejectedDir` as a fourth arg
- `/workspace/pkg/cmd/spec_reject.go` lines 195-217 — `rejectLinkedPrompt` (the only known sibling reject path; out-of-scope for this prompt — it's the spec-reject cascade, not a per-prompt reject)
- `/workspace/pkg/lock/locker.go` lines 28-32 — `Locker` interface; the spec's Failure Modes table references the project lock, but the current `reject` command does not acquire it. Acquisition of the project lock is OUT OF SCOPE for this prompt (flagged as a follow-up if the reviewer requires it). The "concurrent reject + advance" AC in spec 092 is verified by prompt 2's test, which uses a Ginkgo + goroutine test against the project lock — see `/workspace/pkg/lock/filelock.go`.

</context>

<requirements>

## 0. Sibling-coverage check (fail-closed gate)

Before editing any file, verify no UNKNOWN sibling reject path exists. Run:

```
grep -rn 'func.*[Rr]eject' /workspace/pkg/cmd/
```

Expected output (exactly these matches — known and accounted for):
- `pkg/cmd/reject.go` — `NewRejectCommand`, `(r *rejectCommand) Run`, `(r *rejectCommand) rejectByID` (the function this prompt edits)
- `pkg/cmd/spec_reject.go` — `NewSpecRejectCommand`, `(s *specRejectCommand) Run`, `rejectSpec`, `findLinkedPrompts`, `preflight`, `rejectLinkedPrompt` (the spec-reject cascade; calls `pf.StampRejected` on linked prompts that are in pre-execution state — OUT OF SCOPE here, NOT widened to accept `failed`)

If the grep output contains any other reject function (e.g. `RejectByPath`, `BatchReject`, `RejectMany`, `rejectAll`, etc.) — STOP. Do NOT proceed with edits. Report the unexpected sibling to the operator and escalate. The reason: a second per-prompt reject entry point would silently bypass the widened gate, leaving a known regression in tree. This prompt edits `rejectByID` only.

Additionally confirm that `rejectLinkedPrompt` is invoked ONLY from `(s *specRejectCommand) rejectSpec` (no per-prompt CLI surface). Run:

```
grep -rn 'rejectLinkedPrompt' /workspace/
```

Expected: only call site is inside `pkg/cmd/spec_reject.go`. If found elsewhere, STOP and escalate.

## 1. Add `OriginalStatus` field to `prompt.Frontmatter`

In `/workspace/pkg/prompt/prompt.go`, edit the `Frontmatter` struct (lines 244-263) to add a new field. Place it immediately AFTER `Status` and BEFORE `Specs` to keep the on-disk YAML layout readable. The new field uses the `yaml:"originalStatus,omitempty"` tag — camelCase, matches the spec's `originalStatus: failed` AC verbatim, `omitempty` so prompts rejected from pre-execution states do not see a new line in their frontmatter.

```go
// Frontmatter represents the YAML frontmatter in a prompt file.
type Frontmatter struct {
    Status             string   `yaml:"status"`
    OriginalStatus     string   `yaml:"originalStatus,omitempty"`
    Specs              SpecList `yaml:"spec,omitempty,flow"`
    Summary            string   `yaml:"summary,omitempty"`
    // ... existing fields unchanged ...
}
```

Do NOT modify the `AvailablePromptStatuses` set, the `promptTransitions` map, the `IsRejectable()` / `IsPreExecution()` helpers, or any existing field's YAML tag. The new field is purely additive.

## 2. Add `StampRejectedWithOriginal` helper to `*prompt.PromptFile`

In `/workspace/pkg/prompt/prompt.go`, immediately AFTER the existing `StampRejected` method (lines 442-449), add a new method:

```go
// StampRejectedWithOriginal sets the rejected timestamp and reason, marks status as rejected,
// and preserves the prompt's prior status (typically "failed") in the originalStatus field.
// Used by the reject command when rejecting a prompt from a non-pre-execution state.
func (pf *PromptFile) StampRejectedWithOriginal(reason, originalStatus string) {
    if pf.Frontmatter.Rejected == "" {
        pf.Frontmatter.Rejected = pf.now().UTC().Format(time.RFC3339)
    }
    pf.Frontmatter.RejectedReason = reason
    pf.Frontmatter.Status = string(RejectedPromptStatus)
    pf.Frontmatter.OriginalStatus = originalStatus
}
```

The existing `StampRejected(reason string)` STAYS unchanged. The two methods differ only in whether they write `OriginalStatus` — the new helper writes it; the old helper does not. Both methods are safe to call repeatedly (the `Rejected` timestamp is idempotent via the `== ""` check).

## 3. Widen the `IsRejectable` gate in `pkg/cmd/reject.go`

Edit `/workspace/pkg/cmd/reject.go`. The current `rejectByID` function (lines 61-100) checks `!status.IsRejectable()` at line 76 and rejects everything that is not `idea` / `draft` / `approved` with a "cannot reject" error. Replace the gate to ALSO accept the `failed` state.

### 3a. Refactor the status check (lines 72-82)

Replace the existing block:

```go
status := prompt.PromptStatus(pf.Frontmatter.Status)
if status == prompt.RejectedPromptStatus {
    return errors.Errorf(ctx, "%s is already rejected", filepath.Base(path))
}
if !status.IsRejectable() {
    return errors.Errorf(
        ctx,
        "cannot reject prompt with status %q — pre-execution states only (idea, draft, approved)",
        pf.Frontmatter.Status,
    )
}
```

with:

```go
status := prompt.PromptStatus(pf.Frontmatter.Status)
if status == prompt.RejectedPromptStatus {
    return errors.Errorf(ctx, "%s is already rejected", filepath.Base(path))
}
if !status.IsRejectable() && status != prompt.FailedPromptStatus {
    return errors.Errorf(
        ctx,
        "cannot reject prompt with status %q — allowed: idea, draft, approved, failed",
        pf.Frontmatter.Status,
    )
}
```

The new error message names `failed` in the allowed list. Do NOT change `IsRejectable()` itself — that helper is used elsewhere and the spec's Non-goals say no generalisation into a state-machine refactor.

### 3b. Branch the frontmatter write (line 84)

Replace the single `pf.StampRejected(reason)` call with the branch:

```go
if status == prompt.FailedPromptStatus {
    pf.StampRejectedWithOriginal(reason, string(prompt.FailedPromptStatus))
} else {
    pf.StampRejected(reason)
}
```

The order matters: `status` is the value captured BEFORE the stamp (it's a local variable; the stamp changes `pf.Frontmatter.Status` to `rejected`, but the local `status` variable still holds the pre-stamp value). Verify by reading the code path: `status := prompt.PromptStatus(pf.Frontmatter.Status)` at line 72 captures the pre-stamp value; the `StampRejected*` call mutates `pf.Frontmatter.Status` to `rejected` but the local `status` retains `failed`. This is the intended behavior — branch on the pre-stamp local.

### 3c. The remainder of `rejectByID` is unchanged

The `Save` call (line 85), the `os.MkdirAll(rejectedDir, 0750)` (line 89), the `os.Rename` (line 94), and the `fmt.Printf` (line 98) stay as-is. The new path is observable in the same way: the operator sees `rejected: <basename>` on stdout and the file at `prompts/rejected/<basename>`.

## 4. Idempotency on partial-move crash (Failure Mode 5)

The spec's Failure Mode 5 says a mid-action crash that moves the file to `rejected/` but fails the frontmatter rewrite must be safely resumable. The current `rejectByID` calls `FindPromptFileInDirs(ctx, id, r.inboxDir, r.inProgressDir)` — `rejectedDir` is NOT in that list. To make a re-run safe, add `r.rejectedDir` to the search list.

Edit `/workspace/pkg/cmd/reject.go` line 62. Change:

```go
path, err := FindPromptFileInDirs(ctx, id, r.inboxDir, r.inProgressDir)
```

to:

```go
path, err := FindPromptFileInDirs(ctx, id, r.inboxDir, r.inProgressDir, r.rejectedDir)
```

`FindPromptFileInDirs` (at `/workspace/pkg/cmd/prompt_finder.go:15`) has the signature `func FindPromptFileInDirs(ctx context.Context, id string, dirs ...string) (string, error)` — variadic, so adding the fourth arg is a one-line edit.

### 4a. Search-order precedence contract

The canonical search-order is `inbox → in-progress → completed → rejected`. The current call passes `inboxDir, inProgressDir` only; this prompt widens it to `inboxDir, inProgressDir, rejectedDir`. (Note: `completedDir` is intentionally NOT in this call — the reject command never operates on completed prompts; only `failed`, pre-execution, and the new "resume after partial move" path are in scope. The completed dir is excluded by design.)

ID-collision across dirs is precluded at write-time by frontmatter status validation: a prompt cannot simultaneously satisfy `status == failed` in `in-progress/` and `status == rejected` in `rejected/`. The factory invariant that puts files into directories matching their frontmatter status enforces this. If a future writer breaks this invariant, `findFilesInDirs` already returns an ambiguity error when more than one file matches the same numeric prefix — that error surfaces to the operator rather than silently picking a winner. The search-order above is therefore canonical for the well-formed case; the ambiguity error covers the malformed case. Do NOT add a tie-break by directory order.

### 4b. Re-run semantics

With this change, a re-run of `prompt reject 226 --reason "x"` after a partial move:
1. `FindPromptFileInDirs` finds the file in `rejectedDir/`
2. `pf.Frontmatter.Status` is `failed` (the partial move did not rewrite frontmatter)
3. The widened gate accepts `failed`
4. The branch calls `StampRejectedWithOriginal` which writes `rejected`, `rejected_reason: x`, and `originalStatus: failed`
5. The same `Save` call rewrites the file in place at the same path in `rejectedDir/`
6. Exit 0

When the partial move has already done the rewrite (i.e. frontmatter already has `status: rejected`, `rejectedReason: <text>`, `originalStatus: failed`), step 2 sees `status: rejected`, the `if status == prompt.RejectedPromptStatus` early-return at line 73 returns "already rejected" — this is a behavior change vs. the pre-widening code path. The new behavior is correct: re-running on an already-rejected prompt should report "already rejected" rather than silently no-op, and the spec Failure Mode 5 is satisfied because the file is already in the desired state.

## 5. Add ginkgo tests in `pkg/cmd/reject_test.go`

Extend the existing `Describe("RejectCommand", ...)` block. Do NOT create a new file. The new tests go inside the same `Describe`. Mirror the existing fixture setup at the top of the file (lines 30-46).

All assertions on rewritten frontmatter MUST round-trip through `prompt.Manager.Load` and assert on typed `pf.Frontmatter.*` fields, NOT on `ContainSubstring("originalStatus:")` of the raw bytes. String-match assertions are fragile against YAML formatting drift (quoting, key order, indent). Construct the manager once per test using the same `prompt.NewManager(...)` constructor invocation as line 44 of the existing test file.

Add these new test cases (each as a separate `It` block):

### 5a. Reject from `failed` state (AC: `reject-accepts-failed`)

```go
It("moves failed prompt from in-progress to rejected and writes originalStatus: failed", func() {
    promptFile := filepath.Join(inProgressDir, "226-spec-056-foo-failed.md")
    Expect(os.WriteFile(
        promptFile,
        []byte("---\nstatus: failed\n---\n# Failed prompt"),
        0600,
    )).To(Succeed())

    err := rejectCmd.Run(ctx, []string{"226-spec-056-foo-failed.md", "--reason", "orphan from sibling worktree"})
    Expect(err).NotTo(HaveOccurred())

    // Source gone
    _, err = os.Stat(promptFile)
    Expect(os.IsNotExist(err)).To(BeTrue())

    // File present in rejected/ — assert via typed frontmatter, not string-match
    dest := filepath.Join(rejectedDir, "226-spec-056-foo-failed.md")
    pm := prompt.NewManager("", "", "", "", nil, libtime.NewCurrentDateTime())
    pf, err := pm.Load(ctx, dest)
    Expect(err).NotTo(HaveOccurred())
    Expect(pf.Frontmatter.Status).To(Equal("rejected"))
    Expect(pf.Frontmatter.OriginalStatus).To(Equal("failed"))
    Expect(pf.Frontmatter.RejectedReason).To(Equal("orphan from sibling worktree"))
})
```

### 5b. Pre-execution path is unchanged (AC: `reject-preserves-pre-exec`)

Verify that the `draft` path still emits `status: rejected` and `rejected_reason:` but leaves `OriginalStatus` empty (so `omitempty` drops the field from on-disk YAML). Use a `draft` fixture, run the existing reject flow, load the rejected file via `pm.Load`, and assert on typed fields:
- `pf.Frontmatter.Status == "rejected"`
- `pf.Frontmatter.RejectedReason == "<text>"`
- `pf.Frontmatter.OriginalStatus == ""` (round-tripped through parse — proves the field was omitted from disk and not silently defaulted)

### 5c. Hostile reason round-trip (AC: `frontmatter-safety`)

```go
It("round-trips a hostile reason containing colon and newline through frontmatter", func() {
    promptFile := filepath.Join(inProgressDir, "226-hostile.md")
    Expect(os.WriteFile(
        promptFile,
        []byte("---\nstatus: failed\n---\n# Hostile"),
        0600,
    )).To(Succeed())

    err := rejectCmd.Run(ctx, []string{"226-hostile.md", "--reason", "a: b\nc"})
    Expect(err).NotTo(HaveOccurred())

    // Read it back via a fresh PromptFile
    dest := filepath.Join(rejectedDir, "226-hostile.md")
    pm := prompt.NewManager("", "", "", "", nil, libtime.NewCurrentDateTime())
    pf, err := pm.Load(ctx, dest)
    Expect(err).NotTo(HaveOccurred())

    Expect(pf.Frontmatter.RejectedReason).To(Equal("a: b\nc"))
    Expect(pf.Frontmatter.OriginalStatus).To(Equal("failed"))
    Expect(pf.Frontmatter.Status).To(Equal("rejected"))
})
```

The `prompt.NewManager` constructor for the read-back: copy the constructor invocation used at line 44 of the existing test file.

### 5d. Re-run after partial move is idempotent (Failure Mode 5)

```go
It("completes frontmatter rewrite on re-run after partial move to rejected/", func() {
    promptFile := filepath.Join(rejectedDir, "226-partial.md")
    // Simulate the partial-move state: file is in rejected/ but frontmatter still says failed
    Expect(os.MkdirAll(rejectedDir, 0750)).To(Succeed())
    Expect(os.WriteFile(
        promptFile,
        []byte("---\nstatus: failed\n---\n# Partial"),
        0600,
    )).To(Succeed())

    err := rejectCmd.Run(ctx, []string{"226-partial.md", "--reason", "complete it"})
    Expect(err).NotTo(HaveOccurred())

    pm := prompt.NewManager("", "", "", "", nil, libtime.NewCurrentDateTime())
    pf, err := pm.Load(ctx, promptFile)
    Expect(err).NotTo(HaveOccurred())
    Expect(pf.Frontmatter.Status).To(Equal("rejected"))
    Expect(pf.Frontmatter.OriginalStatus).To(Equal("failed"))
    Expect(pf.Frontmatter.RejectedReason).To(Equal("complete it"))
})
```

### 5e. Re-run on already-rewritten file reports "already rejected"

```go
It("returns 'already rejected' on a re-run after full rewrite", func() {
    promptFile := filepath.Join(rejectedDir, "226-done.md")
    Expect(os.MkdirAll(rejectedDir, 0750)).To(Succeed())
    Expect(os.WriteFile(
        promptFile,
        []byte("---\nstatus: rejected\noriginalStatus: failed\nrejected_reason: x\n---\n# Done"),
        0600,
    )).To(Succeed())

    err := rejectCmd.Run(ctx, []string{"226-done.md", "--reason", "again"})
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("already rejected"))
})
```

### 5f. Reject from `idea` does NOT emit `originalStatus`

```go
It("rejects an idea prompt without writing originalStatus", func() {
    promptFile := filepath.Join(inboxDir, "005-idea.md")
    Expect(os.WriteFile(promptFile, []byte("---\nstatus: idea\n---\n# Idea"), 0600)).To(Succeed())

    err := rejectCmd.Run(ctx, []string{"005-idea.md", "--reason", "no"})
    Expect(err).NotTo(HaveOccurred())

    dest := filepath.Join(rejectedDir, "005-idea.md")
    pm := prompt.NewManager("", "", "", "", nil, libtime.NewCurrentDateTime())
    pf, err := pm.Load(ctx, dest)
    Expect(err).NotTo(HaveOccurred())
    Expect(pf.Frontmatter.Status).To(Equal("rejected"))
    Expect(pf.Frontmatter.OriginalStatus).To(Equal(""))
})
```

## 6. Counterfeiter mocks — reuse, do NOT regenerate

The `RejectCommand` interface signature is unchanged. The existing `mocks/reject-command.go` fake still satisfies the interface. Do NOT add a new mock. Do NOT run `make generate` for this prompt — the existing `mocks/reject-command.go` is sufficient and no diff should result.

## 7. Run `cd /workspace && make precommit`

All checks must pass. `make precommit` runs the lint + vet + test pipeline; `make test` is included. The new test cases above must compile (any signature drift in `prompt.Frontmatter` or `prompt.PromptFile` will surface here) and pass (the new path must be wired correctly). The existing tests must still pass (the pre-execution path is a single-line branch — no other code path changes).

</requirements>

<constraints>

- Do NOT introduce a new `prompt.PromptStatus` constant. The `failed` status already exists at `/workspace/pkg/prompt/prompt.go:64`. The widening is purely a cmd-layer gate change.
- Do NOT modify `IsRejectable()` on `PromptStatus`. That helper is used by other call sites and the spec Non-goals forbid generalising into a state-machine refactor.
- Do NOT change the public signature of `cmd.NewRejectCommand`, `cmd.RejectCommand`, `cmd.PromptManager`, or `factory.CreateRejectCommand`. The widening is internal to `rejectByID`.
- Do NOT modify `promptTransitions` in `/workspace/pkg/prompt/prompt.go`. The spec Non-goals explicitly say "Do NOT introduce a new prompt status" and "Do NOT generalise into a state-machine refactor."
- Do NOT add the project lock (`lock.NewLocker`) to `rejectCommand.Run` in this prompt. The spec's Failure Modes table asserts the lock semantics, but the current reject command does not acquire the project lock — and the concurrent-reject+advance AC is verified by prompt 2's Ginkgo + goroutine test against `lock.NewFileLock` in `/workspace/pkg/lock/filelock.go`. The reviewer should decide whether to add the lock acquisition in a follow-up spec.
- Do NOT modify `/workspace/pkg/prompt/prompt.go` outside of the `Frontmatter` struct and the new `StampRejectedWithOriginal` method. The `Save` method's YAML marshaller is what makes the hostile-reason round-trip work — do not add manual quoting/escaping.
- Do NOT modify `spec_reject.go` `rejectLinkedPrompt` to accept `failed`. The spec-reject cascade is out of scope; only `rejectByID` is widened.
- Do NOT add a `--force` flag. The widened path accepts `failed` unconditionally; other statuses still produce the "cannot reject" error.
- Do NOT build or run the `dark-factory` binary for verification. `make precommit` (which runs `make test`) is the sole verification path. Behavioural assertions belong in the Ginkgo specs above, not in a shell smoke test.
- Do NOT commit — dark-factory handles git.
- File mode `0600` for any new test-fixture frontmatter writes; `0750` for directories the test creates. Project convention.
- Test file mode `0600` on the rejected file; the existing `Save` method writes with `0600` (verified at `/workspace/pkg/prompt/prompt.go:361`).
- Branch is `dark-factory/daemon-blocked-queue-ux` (the spec's branch per frontmatter). Do not switch branches.

</constraints>

<verification>

Run from the repo root inside the YOLO container:

```
cd /workspace && make precommit
```

Exit code 0 required. `make precommit` runs lint, vet, and `make test` — all six new Ginkgo specs (5a–5f) must pass, all existing tests must still pass, and the diff must be lint-clean.

Static spot checks (run after `make precommit` is green; each grep should print the indicated number of matches):

```
cd /workspace
grep -nE 'OriginalStatus' pkg/prompt/prompt.go                        # >= 3 matches: Frontmatter field decl, StampRejectedWithOriginal assignment, method comment
grep -nE 'StampRejectedWithOriginal' pkg/cmd/reject.go                # 1 match (the call inside rejectByID)
grep -nE 'FailedPromptStatus' pkg/cmd/reject.go                       # 2 matches: the gate predicate, the StampRejectedWithOriginal argument
grep -nE 'r\.rejectedDir' pkg/cmd/reject.go                           # >= 2 matches: existing MkdirAll call + new FindPromptFileInDirs arg
grep -cE '^\s*It\(' pkg/cmd/reject_test.go                            # >= 6 more than the baseline (six new It blocks)
ls mocks/reject-command.go && echo "mock present"                     # mock unchanged (single line of output)
```

Sibling-coverage re-check (must match Requirement 0 — verifies no new reject function was introduced by side-effect):

```
grep -rn 'func.*[Rr]eject' /workspace/pkg/cmd/
```

The set of matches must be identical to the set enumerated in Requirement 0 (no additions, no removals). If a new match appears, fix the regression before declaring done.

</verification>
