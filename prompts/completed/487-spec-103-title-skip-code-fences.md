---
status: completed
spec: [103-bug-title-extraction-matches-hash-inside-code-fences]
execution_id: dark-factory-fix-title-skips-code-fences-exec-487-spec-103-title-skip-code-fences
dark-factory-version: v0.188.1
created: "2026-06-29T20:35:00Z"
queued: "2026-06-29T20:52:19Z"
started: "2026-06-29T20:52:20Z"
completed: "2026-06-29T21:18:52Z"
branch: dark-factory/bug-title-extraction-matches-hash-inside-code-fences
---

<summary>

- The prompt title used for commit subjects no longer picks up `#`-prefixed lines that live inside fenced code blocks (e.g. a YAML or shell comment quoted in a ```` ``` ```` example).
- A real markdown H1 at the top of the prompt body still wins, exactly as before.
- If a fenced code block's `#` comment is the only `#`-line in the body, the title comes back empty and the existing filename-based fallback produces the commit subject — no more garbage subjects like "SUMMER_MODE toggles the aquarium light schedule between the".
- Both backtick (```` ``` ````) and tilde (`~~~`) fences are recognised, so switching fence styles can't re-introduce the bug.
- An unterminated (open-but-never-closed) fence is handled safely: scanning ends cleanly and the title is empty rather than crashing.
- Adds unit tests covering all six acceptance criteria, including the exact production reproduction body.
- Adds a short note to the prompt-writing guide recommending an explicit `# Title` H1 at the top of the body as the most reliable way to set the commit subject.

</summary>

<objective>
Make `PromptFile.Title()` track fenced-code-block state while scanning the prompt body so that `#`-prefixed lines inside code fences are not mistaken for the markdown H1 heading. The first true H1 outside any fence still wins; when none exists, `Title()` returns `""` and the existing filename fallback takes over. Scope is one function plus its tests, with a small docs note.
</objective>

<context>
Read `/home/node/.claude/CLAUDE.md` first, then `/workspace/CLAUDE.md`.

Read the parent spec fully:
- `/workspace/specs/in-progress/103-bug-title-extraction-matches-hash-inside-code-fences.md` — Goal, Desired Behavior (steps 1-6), Acceptance Criteria AC1-AC6, Constraints, Failure Modes table, and the Reproduction section (the verbatim repro body is reused in AC6's test).

Read these source files fully before editing:
- `/workspace/pkg/prompt/prompt.go`:
  - The current `Title()` method (the function under the doc comment `// Title extracts the first # heading from the body.`). The current body is:
    ```go
    // Title extracts the first # heading from the body.
    func (pf *PromptFile) Title() string {
        scanner := bufio.NewScanner(bytes.NewReader(pf.Body))
        for scanner.Scan() {
            line := strings.TrimSpace(scanner.Text())
            if strings.HasPrefix(line, "# ") {
                return strings.TrimPrefix(line, "# ")
            }
        }
        return ""
    }
    ```
    `bufio`, `bytes`, and `strings` are already imported in this file — no new imports are required.
  - `NewPromptFile(path string, fm Frontmatter, body []byte, currentDateTimeGetter libtime.CurrentDateTimeGetter) *PromptFile` (used by tests to build a `PromptFile` directly from a body byte slice).
  - The package-level `title(...)` helper and the `Manager.Title(ctx, path)` wrapper — these call `pf.Title()` and apply the filename fallback when it returns `""`. Do NOT change these; they already do the right thing once `Title()` returns `""`.
- `/workspace/pkg/prompt/prompt_content_test.go` — the existing `Describe("Title", ...)` block. It loads files from disk via `prompt.NewManager("", "", "", "", nil, libtime.NewCurrentDateTime()).Title(ctx, path)`. Mirror this package (`package prompt_test`), Ginkgo/Gomega style, and the `tempDir` BeforeEach/AfterEach harness for any disk-based tests. For the new pure-`Title()` tests you may instead construct a `PromptFile` directly via `prompt.NewPromptFile(...)` and call `pf.Title()` — that avoids touching disk and is the cleanest way to assert the new behavior.
- `/workspace/pkg/prompt/prompt_suite_test.go` — Ginkgo suite registration (`package prompt_test`). New Ginkgo specs in this package are auto-run.

Where the title is consumed (read-only context, do NOT modify):
- `/workspace/pkg/processor/processor.go` around `title := pf.Title()`:
  ```go
  title := pf.Title()
  if title == "" {
      title = strings.TrimSuffix(filepath.Base(pr.Path), ".md")
  }
  ```
  This is the filename fallback the fix relies on. Returning `""` from `Title()` is the desired outcome for the bug case.

Read these guides before implementing:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo/Gomega suite style, coverage ≥80%, error-path coverage.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — `## Unreleased` entry format.
</context>

<requirements>

## 1. Add fenced-code-block tracking to `Title()`

1.1. In `/workspace/pkg/prompt/prompt.go`, replace the body of `func (pf *PromptFile) Title() string` so it tracks whether the scanner is currently inside a fenced code block. Use exactly this implementation:

```go
// Title extracts the first # heading from the body.
//
// Lines inside fenced code blocks (``` or ~~~) are skipped: a `# ` line inside a
// fence is literal text per CommonMark §4.5, not a markdown heading. A fence is
// opened/closed by a line whose trimmed text starts with ``` or ~~~ (any optional
// info string is ignored). An unterminated fence is treated as open through
// end-of-body, so no `# ` line inside it is ever returned. When no `# ` heading
// exists outside any fence, returns "" and the caller's filename fallback applies.
func (pf *PromptFile) Title() string {
	scanner := bufio.NewScanner(bytes.NewReader(pf.Body))
	inFence := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}
```

Notes for the implementer:
- Fence detection is start-of-trimmed-line only (`strings.HasPrefix` on the trimmed line). Backticks or tildes elsewhere in a line are ignored — this matches the spec's Desired Behavior step 6.
- The fence toggle and the `# ` check are mutually exclusive per line: a fence-marker line is consumed by `continue` and never evaluated as a heading.
- Do NOT add a new package, helper file, or third-party markdown dependency. The change is confined to this one method (spec Constraints).
- Keep the function signature `func (pf *PromptFile) Title() string` unchanged — `processor.go` and existing tests depend on it (spec Constraints).

## 2. Unit tests covering AC1-AC6

Add a new Ginkgo `Describe` block (recommended file: a new `/workspace/pkg/prompt/prompt_title_fence_test.go`, `package prompt_test`, with the BSD license header copied from `/workspace/pkg/prompt/prompt_content_test.go`). Construct each `PromptFile` directly via `prompt.NewPromptFile("001-test.md", prompt.Frontmatter{}, []byte(body), libtime.NewCurrentDateTime())` and call `pf.Title()`. Import `libtime "github.com/bborbe/time"`, `. "github.com/onsi/ginkgo/v2"`, `. "github.com/onsi/gomega"`, and `"github.com/bborbe/dark-factory/pkg/prompt"` (mirror the import block of `prompt_content_test.go`).

Add one `It` per acceptance criterion below. Use raw-string literals for bodies; where a body must itself contain a triple-backtick fence, build it with explicit `"\n"`-joined string concatenation or a raw literal delimited so the inner backticks are preserved (a Go raw string literal cannot contain a backtick — use double-quoted strings joined with `\n`, e.g. `body := "```yaml\n# yaml-comment-line\nkey: value\n```\n"`).

2.1. **AC1 — fence-only `#` line → empty title.** Body contains a fenced YAML block whose only `#`-prefixed line is `# yaml-comment-line`, with no real heading. Assert `pf.Title()` equals `""`.
```go
body := "<requirements>\n```yaml\n# yaml-comment-line\nkey: value\n```\n</requirements>\n"
// Expect(pf.Title()).To(Equal(""))
```

2.2. **AC2 — real heading after a fence wins.** Body has a fenced block containing a `#` comment, FOLLOWED by a real `# Real Title` line outside any fence. Assert `pf.Title()` equals `"Real Title"`.
```go
body := "```yaml\n# inside fence\n```\n\n# Real Title\n\nbody\n"
// Expect(pf.Title()).To(Equal("Real Title"))
```

2.3. **AC3 — no fences, existing behavior preserved.** Body is `# Hello\n\nbody`. Assert `pf.Title()` equals `"Hello"`.

2.4. **AC4 — unterminated fence does not panic and yields empty.** Body has an opening ```` ``` ```` fence with a `# ` line inside it and NO closing fence. Assert the call does not panic (Ginkgo will fail on panic) and `pf.Title()` equals `""`.
```go
body := "```\n# heading inside unterminated fence\nstill inside\n"
// Expect(pf.Title()).To(Equal(""))
```

2.5. **AC5 — tilde fences recognised.** Body has a `~~~yaml ... ~~~` fence whose only `#` line is inside it. Assert `pf.Title()` equals `""`.
```go
body := "~~~yaml\n# tilde-fenced comment\nkey: value\n~~~\n"
// Expect(pf.Title()).To(Equal(""))
```

2.6. **AC6 — production reproduction body.** Reproduce the spec's Reproduction body verbatim (the `<requirements>` block that embeds a fenced YAML example with `# SOME_FLAG toggles between modes`). Assert `pf.Title()` equals `""`. Build the body as:
```go
body := "<summary>\n- Demonstrates the Title() bug\n</summary>\n\n" +
	"<requirements>\n" +
	"1. Add the following block to `k8s/some-deploy.yaml`:\n" +
	"   ```yaml\n" +
	"   # SOME_FLAG toggles between modes\n" +
	"   - name: SOME_FLAG\n" +
	"     value: \"false\"\n" +
	"   ```\n" +
	"2. Run `git add k8s/some-deploy.yaml`.\n" +
	"3. Commit with: `git commit -m \"feat: add some flag\"`.\n" +
	"</requirements>\n"
// Expect(pf.Title()).To(Equal(""))
```
(The leading whitespace before the inner ```` ```yaml ```` is fine: `Title()` trims each line before the fence check, so the indented fence markers are still detected.)

2.7. **Regression — indented `# ` heading still extracted (sanity).** Add one `It` with body `"  # Indented Heading\n\nbody\n"` outside any fence and assert `pf.Title()` equals `"Indented Heading"` (confirms the pre-existing `strings.TrimSpace` behavior is preserved and the fence change did not regress leading-whitespace headings).

## 3. Docs delta — recommend an explicit body H1

3.1. In `/workspace/docs/rules/prompt-writing.md`, under the `## Writing Rules` section, add a new `###` subsection titled `### Title comes from the first body H1`. Place it after the existing `### State the why, not just the what` subsection (around the rules cluster). Content (adjust prose to match the doc's voice, keep it to ~6 lines):

```markdown
### Title comes from the first body H1

The commit subject for a prompt is derived from the first markdown `# ` heading in
the body (falling back to the filename when there is none). `#`-prefixed lines
inside fenced code blocks (```` ``` ```` or `~~~`) are ignored, so a YAML or shell
comment quoted in a code example will never become the commit subject. To control
the commit subject explicitly, put a real `# Your Title` line at the very top of the
body, above any fenced block.
```

## 4. Changelog

4.1. Append to the `## Unreleased` section of `/workspace/CHANGELOG.md` (create the section at the top if it does not exist, following `changelog-guide.md`):
```
- fix: PromptFile.Title() ignores `#`-prefixed lines inside fenced code blocks so commit subjects are no longer derived from YAML/shell comments in code examples (spec 103)
```

</requirements>

<constraints>
- The function signature `func (pf *PromptFile) Title() string` MUST NOT change — callers in `processor.go` and existing tests rely on it.
- The fix lives entirely in `pkg/prompt/prompt.go` — do NOT introduce a new package or extract a fence-tracking helper into a sibling file.
- No new third-party dependencies and no markdown-parser libraries. Use only the stdlib `bufio.Scanner` / `bytes` / `strings` already imported.
- Title extraction handles ATX `# ` headings only (existing contract). Do NOT add Setext (`===` underline) support, indented-code-block (4-space) skipping, or inline-code-span handling — these are explicit Non-goals.
- Existing callers and tests (`processor.go`, `pkg/prompt/prompt_content_test.go` Title tests) MUST continue to compile and pass unchanged.
- Lint and `make precommit` MUST stay green. No `//nolint` directives added for this fix.
- BSD license header preserved on every touched/created file.
- Do NOT commit — dark-factory handles git.
</constraints>

<verification>
Run from `/workspace`:

```bash
# Focused run on the new and existing Title tests
go test ./pkg/prompt/... -run 'Title' -v

# Coverage on the changed package — Title's new branches must be covered
go test -coverprofile=/tmp/cover.out -mod=mod ./pkg/prompt/... && go tool cover -func=/tmp/cover.out | grep -E '\.Title\b'

# Confirm no new package/helper file was added for the fence logic
grep -rn 'inFence' pkg/prompt/prompt.go

# Confirm the docs note landed
grep -n 'Title comes from the first body H1' docs/rules/prompt-writing.md

# Full gate
make precommit
```

Expected: all `Title` tests pass (including the six AC tests and the indented-heading regression); `Title` shows non-zero coverage including the fence branches; `inFence` appears only in `prompt.go`; the docs subsection exists; `make precommit` exits 0.
</verification>
