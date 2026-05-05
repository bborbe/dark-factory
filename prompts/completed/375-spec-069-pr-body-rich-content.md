---
status: completed
spec: [069-pr-body-rich-content]
summary: Added PromptFile.Summary() method and enriched PR body with prompt summary, spec reference, and issue reference; updated both findOrCreatePR call sites; added unit tests and scenario 018; fixed otel vulnerability.
container: dark-factory-375-spec-069-pr-body-rich-content
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-05T17:30:00Z"
queued: "2026-05-05T17:36:06Z"
started: "2026-05-05T17:36:07Z"
completed: "2026-05-05T17:47:45Z"
branch: dark-factory/pr-body-rich-content
---

<summary>
- PR bodies now include the prompt's authored `<summary>` block so reviewers understand intent without navigating to the prompt file
- When a prompt links a spec via frontmatter `spec:`, the PR body contains a `Spec: <slug>` line
- Issue references (`issue:` frontmatter) continue to appear in the PR body unchanged
- A new `Summary()` method on `PromptFile` extracts the `<summary>` XML block from the prompt body (parallel to existing `VerificationSection()`)
- The `Automated by dark-factory` footer is always the final line of the PR body
- All four combinations of summary/no-summary × spec/no-spec produce well-formed bodies
- New scenario 018 manually verifies the PR body via `gh pr view --json body`
</summary>

<objective>
Replace the static PR body `"Automated by dark-factory"` with a rich body that prepends the prompt's `<summary>` block and adds `Spec:` / `Issue:` metadata lines — so reviewers understand the intent of a PR without opening the prompt file.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Key files to read in full before making any changes:

- `pkg/prompt/prompt.go` — read `VerificationSection()` (~line 474) as the direct pattern for `Summary()`; `Specs()` (~line 546) and `Issue()` (~line 541) are already present; `strings` is already imported
- `pkg/processor/workflow_helpers.go` — read `buildPRBody` (lines 33–39) and `findOrCreatePR` (lines 65–94) and their call site at line 245; note `"strings"` is NOT yet imported
- `pkg/processor/workflow_executor_branch.go` — read `handleBranchPRCompletion` (~line 199); it has a second call to `findOrCreatePR` at line 211 that also passes `pf.Issue()` and must be updated
- `pkg/processor/processor_pr_test.go` — read all; the test "pf.Issue() non-empty: PR body contains issue reference" asserts the OLD body format and must be updated; new test cases must be added
- `pkg/prompt/prompt_file_test.go` — read all; new `Summary()` tests go in this file alongside `PromptFile.PRURL` tests (~line 894 onward)
- `scenarios/015-spec-063-branch-pr-path.md` — read as pattern for the new scenario
</context>

<requirements>
**1. Add `Summary()` to `PromptFile` in `pkg/prompt/prompt.go`**

Add immediately after `VerificationSection()`. The method is identical in structure to `VerificationSection()` but targets `<summary>`:

```go
// Summary extracts the content of the <summary> tag from the prompt body.
// Returns an empty string if no <summary> tag is found.
func (pf *PromptFile) Summary() string {
	body := string(pf.Body)
	const openTag = "<summary>"
	const closeTag = "</summary>"
	start := strings.Index(body, openTag)
	if start == -1 {
		return ""
	}
	start += len(openTag)
	end := strings.Index(body[start:], closeTag)
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(body[start : start+end])
}
```

(`strings` is already imported in this file — no import change needed.)

---

**2. Update `buildPRBody` in `pkg/processor/workflow_helpers.go`**

Change the signature and implementation. The new body structure is:
- Section 1 (optional): trimmed summary content
- Section 2 (optional): `Spec: <slug>` line(s) and/or `Issue: <ref>` line, joined with `\n`
- Section 3 (always): `"Automated by dark-factory"`

Sections are joined with `"\n\n"`.

Old:
```go
// buildPRBody constructs the PR body, appending an issue reference when one is set.
func buildPRBody(issue string) string {
	if issue != "" {
		return "Automated by dark-factory\n\nIssue: " + issue
	}
	return "Automated by dark-factory"
}
```

New:
```go
// buildPRBody constructs the PR body from the prompt file's summary, spec links, and issue reference.
func buildPRBody(pf *prompt.PromptFile) string {
	var parts []string
	if summary := pf.Summary(); summary != "" {
		parts = append(parts, summary)
	}
	var metaLines []string
	for _, spec := range pf.Specs() {
		metaLines = append(metaLines, "Spec: "+spec)
	}
	if issue := pf.Issue(); issue != "" {
		metaLines = append(metaLines, "Issue: "+issue)
	}
	if len(metaLines) > 0 {
		parts = append(parts, strings.Join(metaLines, "\n"))
	}
	parts = append(parts, "Automated by dark-factory")
	return strings.Join(parts, "\n\n")
}
```

Add `"strings"` to the import block in `workflow_helpers.go` (it is NOT currently imported there).

---

**3. Update `findOrCreatePR` signature in `pkg/processor/workflow_helpers.go`**

Change the last parameter from `issue string` to `pf *prompt.PromptFile` and update the internal call from `buildPRBody(issue)` to `buildPRBody(pf)`:

```go
func findOrCreatePR(
	gitCtx context.Context,
	ctx context.Context,
	deps WorkflowDeps,
	branchName string,
	title string,
	pf *prompt.PromptFile,
) (string, error) {
	prURL, err := deps.PRCreator.FindOpenPR(gitCtx, branchName)
	// ... existing FindOpenPR / early-return logic unchanged ...
	prURL, err = deps.PRCreator.Create(gitCtx, title, buildPRBody(pf), branchName)
	// ...
}
```

---

**4. Update both call sites of `findOrCreatePR`**

a. `pkg/processor/workflow_helpers.go` line 245 in `handleAfterIsolatedCommit` — change:
```go
prURL, err := findOrCreatePR(gitCtx, ctx, deps, branchName, title, pf.Issue())
```
to:
```go
prURL, err := findOrCreatePR(gitCtx, ctx, deps, branchName, title, pf)
```

b. `pkg/processor/workflow_executor_branch.go` line 211 in `handleBranchPRCompletion` — change:
```go
prURL, err := findOrCreatePR(gitCtx, ctx, e.deps, featureBranch, title, pf.Issue())
```
to:
```go
prURL, err := findOrCreatePR(gitCtx, ctx, e.deps, featureBranch, title, pf)
```

---

**5. Update broken test in `pkg/processor/processor_pr_test.go`**

The test "pf.Issue() non-empty: PR body contains issue reference" (around line 213) currently asserts:
```go
Expect(body).To(ContainSubstring("Issue: BRO-42"))
Expect(body).To(Equal("Automated by dark-factory\n\nIssue: BRO-42"))
```

Remove the `Equal(...)` assertion and replace with assertions that match the new format (footer last):
```go
Expect(body).To(ContainSubstring("Issue: BRO-42"))
Expect(body).To(HaveSuffix("Automated by dark-factory"))
Expect(body).NotTo(HavePrefix("Automated by dark-factory"))
```

---

**6. Add new test cases to `pkg/processor/processor_pr_test.go`**

In the `Describe("Idempotent PR creation and deferred auto-merge"` block, add the following four test cases after the existing "pf.Issue() empty" test.

Use a helper that creates a `PromptFile` with both body and frontmatter:

```go
createFullPromptFile := func(path string, branch string, issue string, specs []string, body string) *prompt.PromptFile {
    return prompt.NewPromptFile(
        path,
        prompt.Frontmatter{
            Status: string(prompt.ApprovedPromptStatus),
            Branch: branch,
            Issue:  issue,
            Specs:  prompt.SpecList(specs),
        },
        []byte(body),
        libtime.NewCurrentDateTime(),
    )
}
```

Test cases:

a. "prompt has summary: PR body starts with summary and ends with footer"
   - Body: `"<summary>\nThis adds a caching layer.\n</summary>\n"`
   - No issue, no spec
   - Assert: body contains `"This adds a caching layer."`, body ends with `"Automated by dark-factory"`, body does NOT start with `"Automated by dark-factory"`

b. "prompt has spec in frontmatter: PR body contains Spec line before footer"
   - Specs: `["069-pr-body-rich-content"]`, no issue, body: `"# Test\n\nContent"` (no `<summary>` tag)
   - Assert: body contains `"Spec: 069-pr-body-rich-content"`, body ends with `"Automated by dark-factory"`

c. "prompt has summary + spec + issue: all three present, footer is last"
   - Body: `"<summary>\nRich summary text.\n</summary>\n"`, Specs: `["069-foo"]`, Issue: `"BRO-99"`
   - Assert:
     - body contains `"Rich summary text."`
     - body contains `"Spec: 069-foo"`
     - body contains `"Issue: BRO-99"`
     - body ends with `"Automated by dark-factory"`
     - the index of `"Rich summary text."` is before the index of `"Spec: 069-foo"`
     - the index of `"Spec: 069-foo"` is before the index of `"Issue: BRO-99"`
     - the index of `"Issue: BRO-99"` is before the index of `"Automated by dark-factory"`

d. "prompt has no summary, no spec, no issue: body is only footer" — already covered by existing "FindOpenPR returns empty" test. Do not add a duplicate.

Wire up each new test the same way as the existing issue tests: set `manager.LoadStub` to return the new prompt file, set up clone mocks, set `prCreator.FindOpenPRReturns("", nil)` and `prCreator.CreateReturns(...)`, use `newProcWorktree(false)`.

---

**7. Add `Summary()` unit tests in `pkg/prompt/prompt_file_test.go`**

Add a new `Describe("PromptFile.Summary"` block alongside the existing `PromptFile.PRURL` tests. Test cases:

a. "returns trimmed summary content when summary block is present"
   - Content: `"---\nstatus: approved\n---\n\n<summary>\nThis is the summary.\n</summary>\n"`
   - Assert: `pf.Summary()` equals `"This is the summary."`

b. "returns empty string when no summary block"
   - Content has no `<summary>` tag
   - Assert: `pf.Summary()` equals `""`

c. "returns empty string when summary open tag is present but close tag is missing"
   - Content: `"<summary>\nNo close tag"` (no `</summary>`)
   - Assert: `pf.Summary()` equals `""`

d. "returns empty string when summary block is present but empty"
   - Content: `"<summary>\n   \n</summary>"`
   - Assert: `pf.Summary()` equals `""` (TrimSpace collapses whitespace-only content)

---

**8. Write scenario `scenarios/018-spec-069-pr-body-rich-content.md`**

Follow the structure of `scenarios/015-spec-063-branch-pr-path.md`. The scenario must:

- Title: "Scenario 018: PR body contains prompt summary and spec reference"
- Status frontmatter: `draft`
- Test repo: copy of `~/Documents/workspaces/dark-factory-sandbox`
- Config: `workflow: branch`, `pr: true`
- A prompt created with a known `<summary>` block containing a unique marker string (e.g. `"dark-factory-scenario-018-marker"`)
- Prompt frontmatter: `spec: ["069-pr-body-rich-content"]`
- After `dark-factory run`:
  - Assert the PR body contains the marker text: `gh pr view --json body -q .body | grep "dark-factory-scenario-018-marker"`
  - Assert the PR body contains `Spec: 069-pr-body-rich-content`: `gh pr view --json body -q .body | grep "Spec: 069-pr-body-rich-content"`
  - Assert the PR body ends with `Automated by dark-factory`: last line check
- Include Cleanup section with branch deletion and `rm -rf "$WORK_DIR"`

---

**9. Add CHANGELOG entry**

In `CHANGELOG.md`, add under `## Unreleased`. The section does not currently exist in the file — create it as the first section, immediately above the most recent versioned heading (`## v0.149.3`):

```
- feat: Enrich PR body with prompt summary, spec reference, and issue reference
```
</requirements>

<constraints>
- Reuse `pf.Summary()` — do NOT re-parse the prompt file inside `buildPRBody`
- Do NOT change the `PRCreator.Create(ctx, title, body, branch)` interface
- Do NOT include the prompt body verbatim — that is the diff
- Do NOT include any pre-merge artifact paths (`prompts/in-progress/...`)
- The `Issue: <ref>` line must continue to appear when `pf.Issue()` is non-empty
- The PR body must always end with a single line `Automated by dark-factory`
- PR title, branch name, merge behavior, and tag/release behavior are unchanged
- Coverage ≥80% for all changed packages (`pkg/prompt`, `pkg/processor`)
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
```bash
make precommit
```

Additionally confirm coverage after the change:
```bash
go test -coverprofile=/tmp/cover.out -mod=vendor ./pkg/prompt/... ./pkg/processor/... && go tool cover -func=/tmp/cover.out | grep -E 'workflow_helpers|prompt\.go'
```
</verification>
