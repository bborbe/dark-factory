---
status: completed
spec: [105-refuse-startup-on-unfinished-pipeline]
summary: Added a ninth `missing-completed-prompt` doctor detector (range scan over prompts/ reusing prompt.NumberFromFilename), wired it into checker.Check, removed the `not a dark-factory project` hard error so Check works as a clean/dirty oracle, with unit tests and four end-to-end fixtures; make precommit exits 0.
execution_id: dark-factory-pipelinegate-exec-494-spec-105-doctor-missing-completed-detector
dark-factory-version: v0.193.0
created: "2026-09-15T07:47:56Z"
queued: "2026-09-15T07:54:23Z"
started: "2026-09-15T07:54:24Z"
completed: "2026-09-15T08:01:02Z"
---

<summary>

- `dark-factory doctor` gains a ninth detection category that reports a **gap in `prompts/completed/`** — a prompt number that has no file in `completed/` — by number, so a blocked queue can no longer look like an idle queue.
- The detector reports both shapes of the gap: a number whose prompt is sitting in `in-progress/` (the 186/187 shape seen on vault-cli), and a number that exists in neither tree (a never-created gap that no directory listing can reveal).
- A contiguous `prompts/completed/` tree keeps producing zero findings — the new category never fires on a healthy project.
- The scan no longer refuses to run on a repo that has a dark-factory config but no pipeline directories, so it can answer "is this project clean?" for any caller.
- The existing eight detectors and their tests are untouched; only the check-level test that encoded the removed hard error is rewritten.
- The detector reuses the same rule the execution guard uses to read a prompt number, so the two can never disagree about what counts as a gap.
- No auto-repair is added: `doctor --fix` keeps its existing behaviour and the new category is reported, not remediated.

</summary>

<objective>
Give `dark-factory doctor` a ninth detector that names the prompt numbers missing from `prompts/completed/`, and make `checker.Check` tolerate a project with no pipeline tree, so that (a) the one leftover class that silently blocks execution becomes visible, and (b) prompt 2's daemon startup gate can reuse the checker as a clean/dirty oracle instead of re-deriving state.
</objective>

<context>

Read `/workspace/CLAUDE.md` first for project conventions.

Read the parent spec end-to-end — it is the contract for this prompt:
- `/workspace/specs/in-progress/105-refuse-startup-on-unfinished-pipeline.md` — Summary, Problem, Goal (the shared definition of "unfinished", item 1), Non-goals, Acceptance Criteria AC1–AC5 and AC9, and the ⚠️ block above the ACs (assert on `Finding.Category`, never on raw output).

Read these files END-TO-END before editing:
- `/workspace/pkg/doctor/doctor.go` — `Category` type + the eight `Category*` constants, `Finding` struct (`Category`, `TargetPaths`, `SpecID`, `Detail`, `FixCommand`), `Checker` interface, `Deps` struct (all eight lifecycle dirs + `SpecLister`, `PromptManager`, `CurrentDateTimeGetter`, `VerifyingStaleHours`), `NewChecker(deps Deps) Checker`, and `checker.Check` — note the `os.Stat(c.deps.SpecsInProgressDir)` guard that returns `"not a dark-factory project: missing %s"`, and the trailing per-finding `sort.Strings(all[i].TargetPaths)`.
- `/workspace/pkg/doctor/parse_errors.go` — `scanDirsForSpecs(ctx, dirs)` and `scanDirsForPrompts(ctx, dirs)` return `.md` paths and skip non-existent dirs. Reuse `scanDirsForPrompts`; do NOT call `os.ReadDir` yourself.
- `/workspace/pkg/doctor/verifying_stale.go` and `/workspace/pkg/doctor/orphan_prompt_link.go` — the detector shape to follow: one file per category, one private `detect*` method on `*checker` returning `([]Finding, error)`, `errors.Wrap(ctx, err, "...")` on the failure path.
- `/workspace/pkg/doctor/check_test.go` — the check-level test file. Its second `It` asserts the `not a dark-factory project` hard error; that assertion is replaced by AC5.
- `/workspace/pkg/doctor/prompted_not_swept_test.go` (`createPromptFile(dir, filename, status, specRef)` helper, ~line 191) and `/workspace/pkg/doctor/duplicate_spec_numbers_test.go` (`createSpecFile`, ~line 164) — reuse these helpers from your new test file; do not duplicate them.
- `/workspace/pkg/prompt/prompt.go` — the regexps at the top of the file (`hasNumberPrefixRegexp` `^\d{3}-`, `extractNumberPrefixRegexp` `^(\d{3})-`), `extractNumberFromFilename(filename string) int` (~line 1623, returns -1 when there is no prefix), `allPreviousCompleted` (~line 1632) and `findMissingCompleted(ctx, completedDir, n)` (~line 1670) — the guard that this detector mirrors. Note both guard helpers live in the unexported space and are reached through `Manager`.

Read these coding-plugin docs (in-container paths — the prompt runs inside a YOLO container):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-doc-best-practices.md`

</context>

<requirements>

## 1. Export the prompt-number rule from `pkg/prompt` (no second regex)

`pkg/doctor` must apply the **same** definition of "prompt number" that `findMissingCompleted` applies. Do not write a second regex in `pkg/doctor`.

In `/workspace/pkg/prompt/prompt.go`, immediately after `extractNumberFromFilename`, add a thin exported wrapper:

```go
// NumberFromFilename extracts the numeric prefix from a prompt filename
// (three digits followed by a dash, e.g. "186-stuck.md" -> 186).
// Returns -1 when the filename carries no such prefix.
//
// Exported so pkg/doctor can apply the same definition of "prompt number"
// that AllPreviousCompleted and FindMissingCompleted use. A second regex
// elsewhere would be a second definition, free to drift.
func NumberFromFilename(filename string) int {
	return extractNumberFromFilename(filename)
}
```

Do NOT rename or modify `extractNumberFromFilename` — it has in-package callers (`Prompt.Number`, `allPreviousCompleted`, `findMissingCompleted`). Do NOT change `allPreviousCompleted` or `findMissingCompleted` in any way: the detector reads the same state the guard reads; it does not change how the guard decides.

## 2. Add the ninth category constant

In `/workspace/pkg/doctor/doctor.go`, after `CategoryLegacyLockFile`, add:

```go
// CategoryMissingCompletedPrompt indicates a prompt number below the highest
// number present anywhere in prompts/ that has no file in prompts/completed/.
// This is the gap that makes the execution guard refuse every spec-less prompt
// below it (reason=previous-prompt-not-completed).
const CategoryMissingCompletedPrompt Category = "missing-completed-prompt"
```

Keep the existing GoDoc-comment style (`// CategoryX indicates ...`).

## 3. Write the detector in a new file `pkg/doctor/missing_completed_prompt.go`

Signature (private method on `*checker`, one file per category, mirroring `detectVerifyingStale`):

```go
func (c *checker) detectMissingCompletedPrompt(ctx context.Context) ([]Finding, error)
```

Algorithm — implement exactly this, in this order:

1. `allPaths, err := scanDirsForPrompts(ctx, []string{c.deps.PromptsInboxDir, c.deps.PromptsInProgressDir, c.deps.PromptsCompletedDir, c.deps.PromptsCancelledDir})` — wrap a failure with `errors.Wrap(ctx, err, "scan prompt dirs")`.
2. `completedPaths, err := scanDirsForPrompts(ctx, []string{c.deps.PromptsCompletedDir})` — wrap a failure with `errors.Wrap(ctx, err, "scan completed dir")`.
3. Build `pathsByNumber map[int][]string` from `allPaths`: `n := prompt.NumberFromFilename(filepath.Base(path))`; skip `n < 0` (filenames with no numeric prefix); append the path under `n`.
4. Build `completedNumbers map[int]bool` from `completedPaths` with the same extraction, skipping `n < 0`.
5. If `len(pathsByNumber) == 0`, return `nil, nil` — an empty tree has no range to scan and must not report anything.
6. `lo` = the smallest key of `pathsByNumber`, `hi` = the largest key.
7. For `n := lo; n < hi; n++`: if `completedNumbers[n]` is true, continue; otherwise emit one finding. **`hi` itself is never scanned** — the highest number present is the newest/in-flight prompt, and reporting it would flag a prompt that is legitimately not completed yet. Numbers below `lo` are never reported either.
8. Finding fields (one finding per missing number, ascending):
   - `Category: CategoryMissingCompletedPrompt`
   - `TargetPaths: pathsByNumber[n]` — the prompt files that carry this number, or an empty slice when the number has no file anywhere (the never-created gap). Do NOT sort here: `Check` already sorts every finding's `TargetPaths`.
   - `SpecID: ""` — this is not a spec finding.
   - `Detail:` exactly `"prompt number " + strconv.Itoa(n) + " is missing from " + c.deps.PromptsCompletedDir` (e.g. `prompt number 186 is missing from prompts/completed`). The number MUST appear in `Detail`: prompt 2's daemon gate names the offending numbers by rendering `Detail`, and the `Detail` string is the only carrier for the never-created-gap shape.
   - `FixCommand:` **shape-dependent**, because the two gap shapes have different remedies. When `pathsByNumber[n]` is non-empty (the prompt file exists somewhere — the 186/187 shape), emit exactly `"dark-factory prompt requeue " + strconv.Itoa(n)`. When it is empty (the never-created gap, nothing to requeue), emit exactly `"no file for prompt number " + strconv.Itoa(n) + "; renumber the blocking prompt into a free slot below it, then re-run dark-factory doctor"`.

     Two hard rules here, both verified against the installed binary on 2026-09-15:
     1. **Only emit a subcommand that exists.** `dark-factory prompt requeue <n>` exists. `dark-factory prompt unlink` and `dark-factory spec renumber` **do not** (`error: unknown prompt subcommand: unlink`, `error: unknown spec subcommand: renumber`) — yet `orphan_prompt_link.go` and `duplicate_spec_numbers.go:85` both emit them today, and `docs/running.md:272` documents the second. Do not copy that pattern, and do not "fix" those two files — they are out of scope for this prompt.
     2. **The number must be visible in the fix line.** `cmd/doctor.go` prints `TargetPaths` + `FixCommand` and never prints `Detail`, so both branches above embed `n` literally.

     `requeue` is the correct remedy for the first shape per `docs/troubleshooting.md`, which names requeue-or-renumber as the only two remedies that actually clear this block (`prompt cancel` refuses on a `failed` prompt; `prompt reject` moves to `rejected/`, which is still not `completed/`).

Constraints on the implementation: no writes, no `os.ReadDir`/`filepath.Walk` (use `scanDirsForPrompts`), no new `Deps` fields, no changes to `Finding`, no caching, and no change to `doctor.Checker`'s signature. Import `github.com/bborbe/dark-factory/pkg/prompt` for `NumberFromFilename`.

## 4. Invoke the detector from `checker.Check`

In `Check`, after the `detectLegacyLockFiles` block and **before** `scanParseErrors`, add:

```go
missingCompleted, err := c.detectMissingCompletedPrompt(ctx)
if err != nil {
    return nil, errors.Wrap(ctx, err, "detect missing completed prompts")
}
all = append(all, missingCompleted...)
```

Parse-error findings stay last (the existing comment above `Check` says so). Update that stale doc comment — it currently claims `Check runs all six detectors`; make it read `all nine detectors` and keep the "Parse-error findings are appended last." sentence.

## 5. Remove the uninitialized-project hard error (AC5)

Delete the guard block at the top of `Check`:

```go
// Verify the project is initialized.
if _, err := os.Stat(c.deps.SpecsInProgressDir); os.IsNotExist(err) {
    return nil, errors.Errorf(ctx, "not a dark-factory project: missing %s", c.deps.SpecsInProgressDir)
}
```

`Check` becomes a clean/dirty oracle: a directory with `.dark-factory.yaml` but no pipeline tree yields **no findings and no error**. Every existing detector already tolerates missing directories (`scanDirsForSpecs`/`scanDirsForPrompts`/`spec.Lister` skip `os.IsNotExist`; `detectLegacyLockFiles` skips missing dirs), so nothing else needs to change. Remove the now-unused `os` import from `doctor.go` (verify with `grep -n 'os\.' pkg/doctor/doctor.go` — the `Stat` call is its only use in that file). Do NOT add a replacement error for any other path; do NOT touch the CLI layer, `pkg/project.FindRoot` still owns the "not a dark-factory project" message for a directory with no `.dark-factory.yaml` at all.

## 6. Tests

**New file `/workspace/pkg/doctor/missing_completed_prompt_test.go`** (`package doctor_test`, Ginkgo v2 + Gomega, `Describe("MissingCompletedPrompt", ...)`), following the fixture style of `orphan_prompt_link_test.go`: `tempDir := GinkgoT().TempDir()`, `os.MkdirAll(..., 0750)`, a `doctor.Deps` built from the temp dirs (reuse `createPromptFile(dir, filename, status, specRef)` with `specRef` empty — spec-less prompts are deliberate: a `spec:` field pointing at an absent spec raises `orphan-prompt-link` and makes the fixture pass for the wrong reason). Cover at least:

1. **AC2 — gap present in `in-progress/` (the 186/187 shape).** `completed/` holds `185-blocked.md` (`status: completed`) and `188-later.md` (`status: completed`); `in-progress/` holds `186-stuck.md` (`status: failed`) and `187-queued.md` (`status: approved`). Assert: exactly two findings whose `Category` equals `doctor.CategoryMissingCompletedPrompt`; one names 186 and the other names 187 (assert on `Detail`); no finding of any other category is produced by the fixture (the category set is exactly `{missing-completed-prompt}`); and, as an explicit assertion, the finding count is 2 — i.e. numbers below the lowest number present (1…184) are NOT reported.
2. **AC1 — the constant is surfaced by `Check`.** In the same fixture, assert the finding's `Category` equals `doctor.CategoryMissingCompletedPrompt` and `Expect(string(doctor.CategoryMissingCompletedPrompt)).To(Equal("missing-completed-prompt"))`.
3. **AC3 — gap absent from both trees (never-created).** `completed/` holds `185-blocked.md`, `187-queued.md`, `188-later.md` (all `status: completed`); `in-progress/` is empty; nothing anywhere is numbered 186. Assert: exactly one finding of the new category, it names 186, and its `TargetPaths` is empty. Note: 187 must be present in `completed/` for this fixture to yield exactly one finding — with 187 absent as well, the range scan correctly reports 187 too (two findings), which is not the shape AC3 asserts.
4. **AC4 — contiguous tree.** `completed/` holds `001-prompt.md` … `010-prompt.md` (`status: completed`), nothing else. Assert: `Check` returns no findings at all.
5. **Highest number in flight.** `completed/` holds `001-prompt.md`, `002-prompt.md`; `in-progress/` holds `003-running.md` (`status: executing`). Assert: no findings (the range stops below the highest number present, so the in-flight prompt is never reported).
6. **Empty tree.** All four prompt dirs exist and are empty. Assert: no findings, no error.

**Update `/workspace/pkg/doctor/check_test.go`.** Replace the second `It` (`returns an error when specs/in-progress/ does not exist`) with a test that builds `Deps` against a bare temp dir — create NO pipeline directories at all — and asserts `Expect(err).NotTo(HaveOccurred())` and `Expect(findings).To(BeEmpty())`. Keep the existing `PromptManager`/`SpecLister` construction so `Check` runs every detector. Leave the first `It` (`returns an empty slice (not nil) when there are no anomalies`) untouched.

Do NOT modify any other detector's test file — the existing eight detectors' tests must pass untouched.

## 7. CHANGELOG

Append ONE bullet to `## Unreleased` in `/workspace/CHANGELOG.md` (create the section under the intro block if absent; do NOT disturb existing version sections):

```
- feat: add a `missing-completed-prompt` detector to `dark-factory doctor` — reports prompt numbers below the highest number present in `prompts/` with no file in `prompts/completed/`, the gap that silently blocks every spec-less prompt below it; `checker.Check` no longer hard-errors `not a dark-factory project` on a tree with no pipeline dirs so it can serve as a clean/dirty oracle (spec 105 prompt 1)
```

</requirements>

<constraints>

- Only condition 1 of the spec's shared "unfinished" definition is new in this prompt. Parked prompts and parked specs are already covered by existing detectors — do NOT add or change those.
- Do NOT compute staleness from file mtime. `detectVerifyingStale` parses the `verifying` frontmatter timestamp as RFC3339 and stays untouched.
- Do NOT redesign `AllPreviousCompleted` or the spec-conditional carve-out in `pkg/prompt/prompt.go`. The detector reads the same state the guard reads; it does not change how the guard decides.
- Do NOT auto-remediate. The detector reports; `doctor --fix` keeps its current behaviour. Do NOT add a fixer branch for the new category — the fixer's existing `default` branch already skips categories it cannot repair.
- Do NOT change `doctor`'s exit code contract for initialized repos (findings → exit 1), and do NOT change `cmd/doctor.go`'s output format.
- Do NOT add config fields, CLI flags, or tunables of any kind.
- Do NOT modify `pkg/pipelinegate`, `pkg/runner`, `pkg/factory`, or `main.go` — the daemon startup gate is prompt 2.
- All error wrapping via `github.com/bborbe/errors` (`errors.Wrap(ctx, err, "...")`, `errors.Errorf(ctx, "...")`). Never `fmt.Errorf`, never `context.Background()` inside `pkg/`.
- External test packages (`package doctor_test`), Ginkgo v2 + Gomega, ≥80% statement coverage on the new code. File modes: `0600` for fixture files, `0750` for fixture directories.
- Do NOT commit — dark-factory handles git. Existing tests must still pass; `make precommit` must exit 0.

</constraints>

<verification>

```bash
cd /workspace

# 1. Compiles and the touched packages pass
go build -mod=mod ./...
# expected: exit 0

go test -mod=mod ./pkg/doctor/... ./pkg/prompt/...
# expected: PASS (all pre-existing doctor + prompt tests included)

# 2. Structural checks
ls /workspace/pkg/doctor/missing_completed_prompt.go
# expected: the file is listed

grep -n 'CategoryMissingCompletedPrompt' /workspace/pkg/doctor/doctor.go
# expected: >= 1 line (the constant declaration)

grep -n 'detectMissingCompletedPrompt' /workspace/pkg/doctor/doctor.go
# expected: >= 1 line (the call site inside Check)

grep -n 'func NumberFromFilename' /workspace/pkg/prompt/prompt.go
# expected: 1 line

! grep -q 'not a dark-factory project' /workspace/pkg/doctor/doctor.go
# expected: exit 0 (the hard error is gone; the message survives only in pkg/project)

! grep -q '"os"' /workspace/pkg/doctor/doctor.go
# expected: exit 0 (the os import was removed with its only use)

grep -rn 'os.ReadDir\|filepath.Walk' /workspace/pkg/doctor/missing_completed_prompt.go
# expected: no output (the detector reuses scanDirsForPrompts)

# 3. End-to-end against freshly built fixtures (container-executable).
#    Build the binary from THIS tree, never the installed one.
go build -mod=mod -o /tmp/new-dark-factory .
# expected: exit 0

# 3a. Fixture 186/187 — gap present in in-progress (AC1 + AC2)
FIX=/tmp/df-fixture-186187
rm -rf "$FIX"
mkdir -p "$FIX/prompts/completed" "$FIX/prompts/in-progress" "$FIX/specs/in-progress"
printf 'workflow: direct\n' > "$FIX/.dark-factory.yaml"
printf -- '---\nstatus: completed\nspec: \n---\n# Prompt\n' > "$FIX/prompts/completed/185-blocked.md"
printf -- '---\nstatus: completed\nspec: \n---\n# Prompt\n' > "$FIX/prompts/completed/188-later.md"
printf -- '---\nstatus: failed\nspec: \n---\n# Prompt\n' > "$FIX/prompts/in-progress/186-stuck.md"
printf -- '---\nstatus: approved\nspec: \n---\n# Prompt\n' > "$FIX/prompts/in-progress/187-queued.md"
(cd "$FIX" && /tmp/new-dark-factory doctor) > /tmp/df-fix-a.out 2>&1; echo "EXIT=$?"
# expected: EXIT=1

grep -oE '^[a-z][a-z-]+$' /tmp/df-fix-a.out | sort -u
# expected: exactly one line — missing-completed-prompt.
# Any other category header means the fixture is red for the WRONG reason
# (see the spec's warning block) — fix the fixture, not the assertion.

grep -qF '186' /tmp/df-fix-a.out && grep -qF '187' /tmp/df-fix-a.out && echo "numbers 186+187 named"
# expected: numbers 186+187 named

# 3b. Fixture gap-both — 186 exists in NEITHER tree (AC3)
FIX=/tmp/df-fixture-gap-both
rm -rf "$FIX"
mkdir -p "$FIX/prompts/completed" "$FIX/specs/in-progress"
printf 'workflow: direct\n' > "$FIX/.dark-factory.yaml"
printf -- '---\nstatus: completed\nspec: \n---\n# Prompt\n' > "$FIX/prompts/completed/185-blocked.md"
printf -- '---\nstatus: completed\nspec: \n---\n# Prompt\n' > "$FIX/prompts/completed/187-queued.md"
printf -- '---\nstatus: completed\nspec: \n---\n# Prompt\n' > "$FIX/prompts/completed/188-later.md"
(cd "$FIX" && /tmp/new-dark-factory doctor) > /tmp/df-fix-b.out 2>&1; echo "EXIT=$?"
# expected: EXIT=1

grep -oE '^[a-z][a-z-]+$' /tmp/df-fix-b.out | sort -u
# expected: exactly one line — missing-completed-prompt

grep -qF '186' /tmp/df-fix-b.out && echo "186 named"
# expected: 186 named
! grep -qF '187' /tmp/df-fix-b.out
# expected: exit 0 (187 IS in completed/ and must not be reported)

# 3c. Fixture clean — contiguous tree must STAY green (AC4)
FIX=/tmp/df-fixture-clean
rm -rf "$FIX"
mkdir -p "$FIX/prompts/completed" "$FIX/specs/in-progress"
printf 'workflow: direct\n' > "$FIX/.dark-factory.yaml"
for n in 001 002 003 004 005 006 007 008 009 010; do
  printf -- '---\nstatus: completed\nspec: \n---\n# Prompt\n' > "$FIX/prompts/completed/$n-prompt.md"
done
(cd "$FIX" && /tmp/new-dark-factory doctor); echo "EXIT=$?"
# expected: `no findings` and EXIT=0

# 3d. Fixture noprompts — no pipeline tree at all (AC5)
FIX=/tmp/df-fixture-noprompts
rm -rf "$FIX"
mkdir -p "$FIX"
printf 'workflow: direct\n' > "$FIX/.dark-factory.yaml"
(cd "$FIX" && /tmp/new-dark-factory doctor); echo "EXIT=$?"
# expected: `no findings` and EXIT=0
# (before this change: EXIT=1 with `not a dark-factory project: missing specs/in-progress`)

# 4. Final gate
make precommit
# expected: exit 0
```

Before finishing, re-run the whole `<verification>` block and confirm every expectation holds, then walk AC1–AC5 and AC9 of the spec against the change and state in the completion report how each one is satisfied. Any AC you cannot demonstrate is a failure to report, not to paper over.

</verification>

<!-- OPEN QUESTION for the human reviewer (not for the executing agent):
     AC3's prose says "`185` and `188` exist in `completed/`" and expects ONE finding naming 186.
     A range scan that starts at the lowest number present and stops below the highest reports
     BOTH 186 and 187 in that fixture (neither is in completed/), so the fixture above places
     187 in completed/ — the minimal fixture that satisfies "exactly one finding naming 186".
     If the intent was instead "report only the lowest never-created gap", AC2's "two findings"
     would contradict it, so the fixture was resolved toward AC2. Confirm at audit time.
     Related, out of scope here: docs/running.md's "Detection categories" table still lists only
     six categories (it never gained `legacy-lock-file`/`parse-errors`); no AC asks for it.
-->
