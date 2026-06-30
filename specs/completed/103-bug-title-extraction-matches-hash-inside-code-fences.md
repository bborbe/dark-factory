---
status: completed
approved: "2026-06-29T20:24:50Z"
generating: "2026-06-29T20:27:59Z"
prompted: "2026-06-29T20:33:01Z"
verifying: "2026-06-29T21:18:59Z"
completed: "2026-06-30T06:39:23Z"
branch: dark-factory/bug-title-extraction-matches-hash-inside-code-fences
---

## Summary

- `PromptFile.Title()` returns the first markdown-H1 (`# `) line from the prompt body
- Title is propagated to the workflow commit subject when the prompt produces a commit
- The current scan does not track fenced-code-block state, so any `# ` line **inside** ```` ``` ```` (e.g. YAML / shell comments quoted in a code example) is picked as the title
- Result: commits land on master with truncated, nonsensical subjects derived from inline code snippets
- Reproduction is mechanical and observed in production (hue PR #7, commit `2d1a830`)

## Problem

`Title()` is the single source of truth for the prompt's commit subject. When it picks up a YAML comment from a code fence inside the prompt body, the resulting commit message is unusable — it's a code fragment, not a sentence, often syntactically truncated mid-clause. This breaks code archaeology (`git log --oneline` is unreadable), PR review (the reviewer-bot's "commit subject vs scope" sanity check returns garbage), and conventional-commit tooling (no recognisable type prefix).

The bug is silent at prompt-author time — the auditor never opens the body the way `Title()` does — and only surfaces post-commit, by which point the commit is already on the feature branch. Recovery costs an amend + force-push, which itself is gated by branch-protection and operator approval. The bug-to-recovery cost ratio is high.

## Goal

`PromptFile.Title()` returns an empty string (triggering the existing filename fallback at `processor.go:313-316`) when the only `# ` lines in the body are inside fenced code blocks. The first real markdown H1 outside any fence still wins. Existing prompts that have a true `# heading` at the body root continue to work unchanged.

## Non-goals

- Title extraction does not need to skip indented code blocks (4-space) — fenced is sufficient for the observed bug class and the prompt-writing convention uses fences exclusively.
- Title does not need to track inline code spans (`` `# foo` ``) — `strings.HasPrefix(line, "# ")` already rejects them because they don't start with `# `.
- Title does not need to handle alternate H1 syntax (`Setext`, `===` underline) — the existing code only supports ATX (`# `), and that contract stays.
- This spec does NOT touch commit-message generation downstream of `Title()`. The fix is scoped to `Title()` returning the right value; the commit-subject path consumes it correctly already.

## Acceptance Criteria

- [ ] **AC1**: `PromptFile.Title()` skips `# ` lines inside ```` ``` ```` fences. Evidence: a new unit test in `pkg/prompt/prompt_test.go` constructs a `PromptFile` whose body contains a fenced YAML block with `# yaml-comment-line` as the only `# `-prefixed line; `pf.Title()` returns `""`.
- [ ] **AC2**: `PromptFile.Title()` returns the first `# ` line that lives OUTSIDE any fence. Evidence: a unit test where a fenced block (with its own `# ` comment) precedes a real `# Real Title` line; `pf.Title()` returns `"Real Title"`.
- [ ] **AC3**: `PromptFile.Title()` returns the first `# ` line correctly when no fenced code blocks are present. Evidence: an existing-behavior regression test where body is `# Hello\n\nbody`; `pf.Title()` returns `"Hello"`.
- [ ] **AC4**: `PromptFile.Title()` handles nested or unbalanced fences without panicking and falls back to scanning to end-of-body. Evidence: a unit test with an opening ```` ``` ```` and no closing fence; the scanner exits cleanly and the function returns `""` (no `# ` found outside the unterminated fence). Asymmetry is acceptable — closing-fence-only is treated as opening a fence.
- [ ] **AC5**: Tilde-fenced code blocks (`~~~`) are also recognised (CommonMark §4.5 fence-syntax parity — both backtick and tilde are valid fence markers; supporting both prevents the bug from re-surfacing if a prompt author switches fence styles). Evidence: a unit test with a `~~~yaml ... ~~~` fence containing a `# ` line; `pf.Title()` returns `""`.
- [ ] **AC6**: The reproduction prompt from the Reproduction section, run through `Title()`, returns `""` (filename fallback fires). Evidence: a unit test loading the reproduction prompt body verbatim; assertion `pf.Title() == ""`.

## Verification

```bash
cd ~/Documents/workspaces/dark-factory
make precommit               # ensures all new tests pass
go test ./pkg/prompt/... -run TestTitle -v   # focused run on the affected function
```

End-to-end runtime check (the bug-workflow `Reproduction-cannot-reproduce` step):

```bash
# In a scratch dark-factory project with a prompt body that quotes YAML comments
# inside a fenced block, run a prompt and inspect the commit subject:
dark-factory prompt approve <reproduction-prompt>
dark-factory daemon
# wait for completion
git log -1 --format="%s"
# expected: filename-derived fallback (e.g. "001-<prompt-name>"), NOT
# the YAML comment line
```

## Desired Behavior

1. `Title()` initializes a `bool inFence` flag.
2. While scanning the body line-by-line: when a trimmed line matches `^```` or `^~~~` (with any optional language tag), `inFence` is toggled.
3. When `inFence == true`, lines starting with `# ` are NOT considered as candidate titles.
4. The first `# ` line encountered while `inFence == false` is returned (with the `# ` prefix stripped), preserving the existing single-title behavior.
5. If no `# ` line is found outside any fence by end-of-body, return `""` — the existing filename-fallback in `processor.go:313-316` takes over.
6. Fence detection is start-of-trimmed-line only; backticks elsewhere on the line are ignored. This matches CommonMark §4.5 at the line-prefix level (no info-string parsing, no indentation-aware container blocks — the scanner only toggles `inFence` on a fence-opener / closer match).

## Constraints

- The function signature (`func (pf *PromptFile) Title() string`) MUST NOT change. Callers in `processor.go:314` rely on it.
- Existing callers (`processor.go` and any test that already asserts `Title()`) MUST continue to compile and pass.
- The fix lives entirely in `pkg/prompt/prompt.go` — do not introduce a new package or extract a fence-tracking helper into a sibling file unless needed for testability.
- No new dependencies on third-party markdown parsers. The implementation must use the stdlib `bufio.Scanner` (or equivalent line iterator) already in use.
- Lint and `make precommit` MUST stay green. No bypassing `golangci-lint` directives for the fix.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| Prompt body has a fenced block with `# yaml-comment` (the bug repro) | `Title()` returns `""`; commit subject falls back to filename | None — this is the fix |
| Prompt body has a real `# Title` and no fences | Existing behavior preserved — `Title()` returns `"Title"` | None |
| Prompt body has a fenced block with `# YAML` AND a later real `# Title` outside the fence | `Title()` returns `"Title"` | None |
| Prompt body has an unterminated opening fence (no closing ```` ``` ````) | Scanner exits at EOF with `inFence == true`; `Title()` returns `""` | Author detection: silent — they see the filename-derived fallback in `git log` and trace back to the missing closing fence. This is acceptable because (a) the prior state was also silently wrong, and (b) the filename fallback is harmless. Defensive; matches "if in doubt, suppress". |
| Prompt body is empty | `Title()` returns `""` (existing behavior) | None |
| Prompt body is binary or contains invalid UTF-8 | `bufio.Scanner` behavior is unchanged (existing failure mode) | Out of scope |

## Reproduction

**dark-factory version**: `v0.188.1` (observed in production 2026-06-29 21:46–21:48 CEST).

**Minimal repro prompt** (`prompts/test-title-bug.md`):

````markdown
---
status: draft
---

<summary>
- Demonstrates the Title() bug
</summary>

<requirements>
1. Add the following block to `k8s/some-deploy.yaml`:
   ```yaml
   # SOME_FLAG toggles between modes
   - name: SOME_FLAG
     value: "false"
   ```
2. Run `git add k8s/some-deploy.yaml`.
3. Commit with: `git commit -m "feat: add some flag"`.
</requirements>
````

**Command sequence** (paraphrased from the hue PR #7 run):

```bash
cd <dark-factory-project>
dark-factory prompt approve test-title-bug
dark-factory daemon --set hideGit=true --set autoRelease=false
# wait for container to finish
git log -1 --format="%s"
```

**Observed evidence** (verbatim from `~/Documents/workspaces/hue-revert-aquarium-heat-wave-override/.dark-factory.log`):

```
time=2026-06-29T21:46:24.910+02:00 level=INFO msg="executing prompt"
  prompt_id=002-add-summer-mode-boolean-flag spec_id="" container=""
  workflow_type=direct
  title="SUMMER_MODE toggles the aquarium light schedule between the"
```

Resulting commit (verbatim from `git log -1`):

```
commit 2d1a830a2fd59e0e892d63d6f7fa0fa4e115a573
    SUMMER_MODE toggles the aquarium light schedule between the
```

The DARK-FACTORY-REPORT summary the container actually wrote was:

```
{"status":"success",
 "summary":"Added HUE_SUMMER_MODE boolean flag that toggles aquarium light
            schedule between daytime (10:00-20:00) and evening-only
            (20:00-23:00) windows", ...}
```

— a perfectly fine sentence the daemon did NOT use. Instead the daemon used `title` (extracted by `Title()`), which had matched a `# SUMMER_MODE …` YAML comment line inside a fenced block in the prompt body.

## Expected vs Actual

**Expected** (per `pkg/prompt/prompt.go:440-450` doc comment "extracts the first # heading from the body"):

> `Title()` returns the first markdown-H1 heading from the body. A `# ` line inside a fenced code block is NOT a markdown heading — it's literal text — so it MUST be ignored.

**Actual**:

`Title()` does a naive `strings.HasPrefix(strings.TrimSpace(line), "# ")` check with no fenced-block tracking. Any line starting with `# ` after trimming wins, including YAML / shell / Python / Ruby comments quoted in a fenced code block.

## Why this is a bug

1. **Documentation contradiction**: the function's own doc comment (`prompt.go:440`) says "first `# heading`". A line inside ```` ```yaml ``` ```` is not a heading; it's an opaque literal.
2. **Markdown spec contradiction**: CommonMark §4.5 (fenced code blocks) explicitly states the contents are not parsed as markdown. A `# foo` inside a fence is text, not a heading.
3. **Downstream breakage**: the commit subject is operator-facing on every merged PR. A garbage subject degrades `git log`, `git blame`, `git bisect`, and conventional-commit tooling — all silently, with no error path to surface the problem.
4. **Operator workaround is hostile**: the only repro-avoidance today is "never put a `#`-prefixed line inside a fenced code block in a prompt body" — impossible for prompts that demonstrate YAML / shell / Dockerfile / Python config, which is the common shape of dark-factory prompts.

## Workaround

Until the fix lands, prompt authors can mitigate by:

1. Placing an explicit `# Real Title` line at the top of the prompt body (above any fenced block). First H1 wins.
2. Replacing `#` YAML comments in fenced examples with non-`#` markers (e.g. switch to JSON, or omit the comment).
3. Amending the resulting commit with the intended message before pushing (force-push gated by operator).

Mitigation (1) is the least-invasive — recommend it in `prompt-writing.md` as part of the fix's docs delta.

## Suggested Decomposition

Single prompt — the fix is a localised change to one function (`pkg/prompt/prompt.go:440`) plus its test file, with no cross-layer ripple. Acceptance Criteria 1–5 are covered by unit tests in `pkg/prompt/prompt_test.go`; AC6 is covered by replaying the documented reproduction body through the same test file.

| # | Prompt focus | Covers ACs |
|---|---|---|
| 1 | Add fence-tracking to `Title()` + unit tests covering AC1-AC6 + brief mention in `prompt-writing.md` recommending an explicit body H1 | 1, 2, 3, 4, 5, 6 |

## Do-Nothing Option

If unfixed, every dark-factory prompt body that demonstrates a `#`-commented config (YAML / shell / Dockerfile / Python) risks a garbage commit subject on the production run. Frequency depends on whether the author noticed during pre-commit review; in the hue PR #7 case the bug surfaced only at `git log` after the daemon finished. Cost-per-incident is one amend + force-push + operator approval cycle (~5–10 min if attended, blocking if not). Probability rises with the diversity of config snippets in prompts. Recommended priority: fix in the next dark-factory release.

## Verification Result

**Verified:** 2026-06-30T06:37:56Z (HEAD 5118180)
**Binary:** /tmp/dark-factory-5118180 (dark-factory dev, built from HEAD 5118180)
**Scenario:** Ginkgo specs in pkg/prompt/prompt_title_fence_test.go exercise the real prompt.Title() (no mocks) on the verbatim production reproduction body and the AC1-AC5 boundary inputs.
**Evidence:**
- `go test ./pkg/prompt/... -ginkgo.focus='Title with fenced code blocks'` → `Ran 7 of 245 Specs ... SUCCESS! -- 7 Passed | 0 Failed`
- AC6 (prompt_title_fence_test.go:81-102) loads the exact hue-PR-#7 reproduction body and asserts `pf.Title() == ""`
- Fix shipped at pkg/prompt/prompt.go:447-464 (inFence toggle on ```/~~~ trimmed-line prefix); doc comment updated at 440-446
- Consumer at processor.go:313-316 unchanged: filename fallback fires when Title() returns ""
- PR #56 merged to bborbe/dark-factory@master (merge commit 5118180, fix commit 293e3d1)
**Verdict:** PASS
