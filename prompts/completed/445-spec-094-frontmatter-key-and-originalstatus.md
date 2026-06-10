---
status: completed
spec: [094-bug-spec-092-contract-violations]
summary: 'Spec 094 defects 1 & 2 remediated: renamed RejectedReason YAML tag to rejectedReason with read-compat for legacy rejected_reason key, unified rejectByID to call StampRejectedWithOriginal for all paths (pre-exec now stamps originalStatus), inverted the draft and idea test assertions to the spec contract, and added an explicit read-compat test. make precommit exits 0.'
container: dark-factory-spec092-fix-exec-445-spec-094-frontmatter-key-and-originalstatus
dark-factory-version: v0.177.1
created: "2026-06-10T15:00:00Z"
queued: "2026-06-10T14:39:29Z"
started: "2026-06-10T15:03:30Z"
completed: "2026-06-10T15:14:10Z"
branch: dark-factory/bug-spec-092-contract-violations
---

<summary>

- `dark-factory prompt reject NNN --reason "<text>"` writes the operator's reason under the YAML key `rejectedReason` (camelCase), so `yq '.rejectedReason' prompts/rejected/<NNN>-*.md` returns the reason instead of `null`.
- Files already on disk that carry the legacy `rejected_reason` key (five such files exist under `prompts/completed/`) continue to parse into the same typed field — read-compat on the legacy key, write only the new key. No on-disk migration.
- When a prompt is rejected from a pre-execution state (`idea`, `draft`, `approved`), the rewritten frontmatter now records the prior status in `originalStatus` (e.g. a draft yields `originalStatus: draft`), the same way a `failed`-path reject already records `originalStatus: failed`.
- The pre-existing test at `pkg/cmd/reject_test.go:227-247` asserted the defect (`OriginalStatus == ""` for the draft-pre case); that test is inverted to assert the spec contract (`OriginalStatus == "draft"`) and the `Describe` header is renamed to match.
- The three reject-test cases that string-match `rejected_reason:` (lines 93, 122, 301) are updated to `rejectedReason:` so the new on-disk key is what the test asserts. The path that loads a fixture written with the legacy key round-trips the legacy text into the typed field — a new explicit read-compat test covers that case.
- No new CLI flag, no new prompt status, no change to `IsRejectable()`, no change to `StampRejected` / `StampRejectedWithOriginal` signatures. `make precommit` is green.

</summary>

<objective>
Fix defects 1 and 2 of spec 094: rename the `RejectedReason` YAML tag to `rejectedReason` with read-compat for the legacy `rejected_reason` key, and make the pre-execution reject path stamp `originalStatus` the same way the `failed` path already does.
</objective>

<context>
Read `/workspace/CLAUDE.md` first for project conventions.

Read these files end-to-end before editing:
- `/workspace/specs/in-progress/094-bug-spec-092-contract-violations.md` — the parent spec, especially `Reproduction` defects 1 & 2, `Goal` items 1 & 2, ACs `reason-key`, `no-old-key`, `read-compat`, `originalStatus`, `inverted-draft-reject-test`
- `/workspace/specs/in-progress/092-daemon-blocked-queue-ux.md` — the contract this spec remediates; its AC evidence commands use `yq '.rejectedReason'` and `yq '.status, .originalStatus, .rejectedReason'` (camelCase verbatim)
- `/workspace/pkg/prompt/prompt.go` lines 244-265 — `Frontmatter` struct; the legacy `RejectedReason` field is at line 263 with tag `yaml:"rejected_reason,omitempty"`
- `/workspace/pkg/prompt/prompt.go` lines 444-463 — `StampRejected` and `StampRejectedWithOriginal`; the pre-exec path calls `StampRejected` (no `OriginalStatus`), the `failed` path calls `StampRejectedWithOriginal`
- `/workspace/pkg/cmd/reject.go` lines 60-100 — `rejectByID`; the `else` branch at line 88 calls `pf.StampRejected(reason)` for pre-exec states
- `/workspace/pkg/cmd/reject_test.go` lines 80-310 — existing ginkgo tests; lines 93, 122, 301 string-match `rejected_reason:`; lines 227-247 assert `OriginalStatus == ""` for the draft case (this is the defect)
- `/workspace/pkg/cmd/reject_test.go` line 44 — `prompt.NewManager` constructor invocation to mirror for the read-back
- `/workspace/pkg/prompt/prompt.go` lines 220-235 — `SpecList.UnmarshalYAML` precedent for implementing a custom `UnmarshalYAML` on a `Frontmatter` field that needs to accept multiple YAML keys (the read-compat pattern is the same: parse twice and pick the populated one)
- `/workspace/pkg/cmd/spec_reject_test.go` lines 98, 154, 165 — these also string-match `rejected_reason:` and must be updated in step 5b for hermetic test discipline (they exercise the spec-reject cascade, which calls `pf.StampRejected`)

YAML library is `gopkg.in/yaml.v3` (confirmed at `/workspace/go.mod:30` and `/workspace/pkg/prompt/prompt.go:28`). All struct tags use yaml.v3 conventions; `MarshalYAML` and `UnmarshalYAML` interfaces are the standard yaml.v3 hooks.

</context>

<requirements>

## 1. Add read-compat `UnmarshalYAML` for `RejectedReason`

In `/workspace/pkg/prompt/prompt.go`, immediately AFTER the `Frontmatter` struct declaration (after line 265), add a new file-level function. The function accepts the legacy snake_case `rejected_reason` key on read AND the spec-mandated camelCase `rejectedReason` key on read, populating a temporary `Frontmatter` that gets merged into the caller's struct.

The implementation pattern (mirroring the existing `SpecList.UnmarshalYAML` at lines 220-235):

```go
// UnmarshalYAML accepts both the legacy snake_case "rejected_reason" key and
// the spec-mandated camelCase "rejectedReason" key, populating whichever is
// present into the same typed field. Writes always emit "rejectedReason" —
// read-compat only, no on-disk migration. See spec 094 AC "read-compat".
type rejectedReasonFrontmatter struct {
    RejectedReasonLegacy string `yaml:"rejected_reason,omitempty"`
    RejectedReasonNew    string `yaml:"rejectedReason,omitempty"`
}
```

The cleanest read-compat strategy is to add a custom `UnmarshalYAML` on `Frontmatter` itself OR to handle the legacy key in the `load` function (lines 314-339). Pick the `load`-function approach — it is a smaller diff and keeps `Frontmatter` schema simple:

In `/workspace/pkg/prompt/prompt.go` `load` function (lines 314-339), after the successful `frontmatter.Parse` call (line 327), add a small block that, if `fm.RejectedReason` is empty, attempts a second decode of the raw frontmatter YAML using a struct with a single `rejected_reason` tag and copies the value into `fm.RejectedReason` if present. Use the existing `frontmatter.NewFormat` helper (line 326) to re-parse the original `content` bytes with the legacy struct. Concretely:

```go
// Read-compat for legacy snake_case key (spec 094). Files written before
// the camelCase migration carry `rejected_reason:` — accept that on read
// and populate the same typed field. Writes always emit `rejectedReason`.
if fm.RejectedReason == "" {
    var legacy struct {
        RejectedReason string `yaml:"rejected_reason,omitempty"`
    }
    if _, parseErr := frontmatter.Parse(bytes.NewReader(content), &legacy, yamlV3Format); parseErr == nil {
        fm.RejectedReason = legacy.RejectedReason
    }
}
```

Place this block between line 328 (the `err` check after `Parse`) and line 338 (the `pf := &PromptFile{...}` construction). The re-parse of a well-formed frontmatter is a no-op on the first attempt; on legacy files the legacy field is populated.

## 2. Rename the YAML tag to `rejectedReason`

In `/workspace/pkg/prompt/prompt.go` line 263, change the struct field tag from `yaml:"rejected_reason,omitempty"` to `yaml:"rejectedReason,omitempty"`. No other change to the `Frontmatter` struct. The new tag is camelCase, matches spec 092 AC evidence commands, and the legacy key is still readable via the read-compat block in step 1.

## 3. Make the pre-execution reject path stamp `originalStatus`

In `/workspace/pkg/cmd/reject.go` line 85-89, replace the `else` branch:

```go
if status == prompt.FailedPromptStatus {
    pf.StampRejectedWithOriginal(reason, string(prompt.FailedPromptStatus))
} else {
    pf.StampRejected(reason)
}
```

with a unified call that always stamps the prior status (the `status` local is the pre-stamp value because `StampRejected*` mutates `pf.Frontmatter.Status` to `rejected` but the local `status` retains the pre-stamp value — verified by reading lines 72 and 86 of the current code):

```go
pf.StampRejectedWithOriginal(reason, string(status))
```

This unifies both paths to call `StampRejectedWithOriginal`. The `failed` path was already calling it; the pre-exec path now also calls it, with `status` being the pre-exec value (`idea` / `draft` / `approved`). Spec 092 AC `reject-preserves-pre-exec` requires `originalStatus: draft` for a draft reject — this is now satisfied.

The `if/else` distinction is removed entirely; both branches collapse to the same call.

## 4. Invert the draft-reject test assertion

In `/workspace/pkg/cmd/reject_test.go` lines 227-247, the `Describe` block "Pre-execution reject leaves originalStatus empty" contains a test that asserts the defect (`pf.Frontmatter.OriginalStatus == ""` for a draft pre-exec reject). Edit the test:

- Rename the `Describe` header from `Pre-execution reject leaves originalStatus empty` to `Pre-execution reject stamps originalStatus`.
- Change the `It` body so that the assertion `Expect(pf.Frontmatter.OriginalStatus).To(Equal(""))` becomes `Expect(pf.Frontmatter.OriginalStatus).To(Equal("draft"))`. The rest of the test body (fixture write, `Run` invocation, file move assertion, status/reason assertions) is unchanged.

The `Describe` block at lines 312-342 ("Reject idea does not write originalStatus") currently asserts the same defect for the `idea` path. Edit that block the same way:

- Rename the `Describe` header from `Reject idea does not write originalStatus` to `Reject idea stamps originalStatus: idea`.
- Change the assertion `Expect(pf.Frontmatter.OriginalStatus).To(Equal(""))` to `Expect(pf.Frontmatter.OriginalStatus).To(Equal("idea"))`.

Both tests now encode the spec contract.

## 5. Update string-match assertions to the new YAML key

Three test files string-match the legacy `rejected_reason:` text in on-disk content. Update each to match the new key. Hermeticity gate (per spec 094 AC `hermetic`): these test files use temp dirs (no real `prompts/` references), so the edit is safe.

### 5a. `pkg/cmd/reject_test.go`

Three edits:

- Line 93: `Expect(string(content)).To(ContainSubstring("rejected_reason: not needed"))` → `Expect(string(content)).To(ContainSubstring("rejectedReason: not needed"))`
- Line 122: `Expect(string(content)).To(ContainSubstring("rejected_reason: changed direction"))` → `Expect(string(content)).To(ContainSubstring("rejectedReason: changed direction"))`
- Line 301: `"---\nstatus: rejected\noriginalStatus: failed\nrejected_reason: x\n---\n# Done"` → `"---\nstatus: rejected\noriginalStatus: failed\nrejectedReason: x\n---\n# Done"`

### 5b. `pkg/cmd/spec_reject_test.go`

Three edits:

- Line 98: `Expect(string(content)).To(ContainSubstring("rejected_reason: not needed"))` → `Expect(string(content)).To(ContainSubstring("rejectedReason: not needed"))`
- Line 154: `Expect(string(specContent)).To(ContainSubstring("rejected_reason: cancelled"))` → `Expect(string(specContent)).To(ContainSubstring("rejectedReason: cancelled"))`
- Line 165: `Expect(string(promptContent)).To(ContainSubstring("rejected_reason: cancelled"))` → `Expect(string(promptContent)).To(ContainSubstring("rejectedReason: cancelled"))`

## 6. Add an explicit read-compat test

In `/workspace/pkg/cmd/reject_test.go`, add a new `It` block inside the existing `Describe("RejectCommand", ...)` block (anywhere alongside the other tests — placing it directly after the inverted draft-reject test from step 4 reads naturally). The test loads a fixture file written with the legacy `rejected_reason` key and asserts the typed field round-trips correctly:

```go
It("loads a legacy rejected_reason key into the typed RejectedReason field", func() {
    // Read-compat: existing files on disk (spec 094 AC "read-compat") carry
    // `rejected_reason:` from before the camelCase migration. They must
    // still parse into the same typed field that new writes use.
    promptFile := filepath.Join(rejectedDir, "226-legacy-key.md")
    Expect(os.MkdirAll(rejectedDir, 0750)).To(Succeed())
    Expect(os.WriteFile(
        promptFile,
        []byte("---\nstatus: rejected\nrejected_reason: legacy text\n---\n# Legacy"),
        0600,
    )).To(Succeed())

    pm := prompt.NewManager("", "", "", "", nil, libtime.NewCurrentDateTime())
    pf, err := pm.Load(ctx, promptFile)
    Expect(err).NotTo(HaveOccurred())
    Expect(pf.Frontmatter.RejectedReason).To(Equal("legacy text"))
})
```

The test fixture is in a temp `rejectedDir` (no real `prompts/` references), so the hermeticity gate holds.

## 7. Verify

Run from the repo root inside the YOLO container:

```
cd /workspace && make precommit
```

Exit code 0 required. `make precommit` runs the lint + vet + test pipeline; the three edited `It` blocks, the unified `StampRejectedWithOriginal` call, the renamed YAML tag, and the read-compat block must all compile and pass.

</requirements>

<constraints>

- Do NOT introduce a new `prompt.PromptStatus` constant or modify `AvailablePromptStatuses`. The fix is a YAML tag rename and a unified `StampRejectedWithOriginal` call — no state-machine change.
- Do NOT modify `StampRejected` or `StampRejectedWithOriginal` signatures. Both methods stay as-is. The pre-exec path is unified to call `StampRejectedWithOriginal` (the new code, already added by spec 092 prompt 441).
- Do NOT change the public signature of `cmd.NewRejectCommand`, `cmd.RejectCommand`, `cmd.PromptManager`, or `factory.CreateRejectCommand`. The fix is internal to `rejectByID` and the `load` function.
- Do NOT migrate the five legacy files in `prompts/completed/` (`441-spec-092-widen-reject-accept-failed`, `382-spec-075-foundation`, `331-spec-058-commands`, `330-spec-058-model`, `332-spec-058-list-filtering`) on disk. Spec 094 Non-goal: "Do NOT add a runtime migration that rewrites existing `rejected_reason` files on disk — read-compat is sufficient." Read-compat is the entire remediation.
- Do NOT add a `--force` flag, an environment variable, or a config knob. The fix is a pure data-shape change.
- Do NOT touch `/workspace/pkg/spec/spec.go`. That struct's `RejectedReason` field (line 144) is for the spec file frontmatter format, distinct from the prompt file frontmatter. Spec 094's contract violation is in the prompt file, not the spec file. The two struct types are independent.
- Do NOT modify `/workspace/pkg/status/formatter.go`. The Blocked-line format is byte-stable (spec 094 AC `blocked-format-unchanged`).
- Do NOT build or run the `dark-factory` binary for verification. `make precommit` is the sole verification path.
- Do NOT commit — dark-factory handles git.
- File mode `0600` for any new test-fixture frontmatter writes; `0750` for directories the test creates. Project convention.
- Branch is `dark-factory/bug-spec-092-contract-violations` (per the spec's frontmatter).

</constraints>

<verification>

Run from the repo root inside the YOLO container:

```
cd /workspace && make precommit
```

Exit code 0 required. `make precommit` runs lint, vet, and `make test` — all three modified `It` blocks (step 4 + step 5a/5b) must pass, the new read-compat test (step 6) must pass, all existing tests must still pass, and the diff must be lint-clean.

Static spot checks (run after `make precommit` is green; each grep should print the indicated matches):

```
cd /workspace
grep -nE 'yaml:"rejectedReason' pkg/prompt/prompt.go            # 1 match: the renamed tag
grep -nE 'rejected_reason' pkg/prompt/prompt.go                  # 0 matches: the legacy tag is gone
grep -nE 'StampRejectedWithOriginal' pkg/cmd/reject.go           # 2 matches: the import is still used; the unified call
grep -nE 'pf\.StampRejected\b' pkg/cmd/reject.go                 # 0 matches: the old StampRejected call is gone
grep -nE 'rejectedReason' pkg/cmd/reject_test.go                 # >= 3 matches: updated string-match assertions
grep -nE 'rejectedReason' pkg/cmd/spec_reject_test.go            # >= 3 matches: updated string-match assertions
grep -nE 'loads a legacy rejected_reason key' pkg/cmd/reject_test.go   # 1 match: the new read-compat test
```

The `git diff` for `/workspace/pkg/status/formatter.go` MUST be empty (the Blocked-line format is locked per spec 094 AC `blocked-format-unchanged`).

Sibling-coverage re-check (the unified `StampRejectedWithOriginal` call must be the only stamp path):

```
grep -rn 'pf\.StampRejected\b' /workspace/pkg/cmd/
```

Expected: zero matches (the old `StampRejected` call is fully replaced). If any match survives, fix the regression before declaring done.

</verification>
