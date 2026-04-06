---
status: completed
container: dark-factory-264-embed-suffix-templates
dark-factory-version: v0.104.0
created: "2026-04-06T16:15:14Z"
queued: "2026-04-06T16:23:38Z"
started: "2026-04-06T16:23:40Z"
completed: "2026-04-06T16:24:05Z"
---

<summary>
- Prompt suffix text currently built via Go string concatenation moves to embedded markdown files
- Four new template files hold the exact text content that was previously inline
- A new Go file uses go:embed to load each template as a string variable
- Existing public functions keep their signatures but internally read from embedded templates
- Parameterized functions (ValidationSuffix, ValidationPromptSuffix) use placeholder substitution
- MarkerStart and MarkerEnd constants remain as Go constants in suffix.go
- Pure refactor with zero behavior change — all existing tests pass unchanged
</summary>

<objective>
Extract the hardcoded prompt suffix strings from Go string concatenation in pkg/report/suffix.go into embedded markdown template files under pkg/report/prompts/, following the //go:embed pattern used in the trading project. The public API stays identical.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `pkg/report/suffix.go` — current implementation with four suffix functions and two marker constants
- `pkg/report/report_test.go` — existing tests that validate suffix output (ContainSubstring checks)
- `pkg/processor/processor.go` — caller site (~line 939) to confirm no changes needed there
- Reference pattern: `//go:embed filename.md` followed by `var Name string` — one directive per file, one exported variable per template
</context>

<requirements>

## 1. Create the prompts directory and embed file

Create `pkg/report/prompts/prompts.go`:

```go
// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompts

import _ "embed"

//go:embed completion-report.md
var CompletionReport string

//go:embed validation-command.md
var ValidationCommand string

//go:embed validation-prompt.md
var ValidationPrompt string

//go:embed changelog.md
var Changelog string
```

## 2. Create completion-report.md

Create `pkg/report/prompts/completion-report.md` containing the exact text currently returned by `Suffix()` in suffix.go, with two changes:
- Replace the literal `MarkerStart` concatenation (`"` + MarkerStart + `"`) with the actual marker string `<!-- DARK-FACTORY-REPORT`
- Replace the literal `MarkerEnd` concatenation with `DARK-FACTORY-REPORT -->`

The file content must produce **byte-identical** output to the current `Suffix()` function. Copy the text precisely, including leading newlines, the `---` separator, all markdown formatting, the example JSON line, and the trailing newline.

**Important**: The `Suffix()` function currently starts with `\n\n---` and ends with a trailing `\n`. The .md file must contain this exact content. Do NOT add or remove any whitespace.

## 3. Create validation-command.md

Create `pkg/report/prompts/validation-command.md` containing the text from `ValidationSuffix()`, but replacing the `cmd` parameter insertion point with the placeholder `{{.Command}}`.

The current function builds: `"\n\n---\n\n## Project Validation Command..." + cmd + "\n```\n..."`.

In the template, replace the dynamic `cmd` value with `{{.Command}}` so the line reads:
```
{{.Command}}
```

## 4. Create validation-prompt.md

Create `pkg/report/prompts/validation-prompt.md` containing the text from `ValidationPromptSuffix()`, replacing the `criteria` parameter insertion with `{{.Criteria}}`.

The current function inserts `criteria` between `"...against your changes:\n\n"` and `"\n\nFor each criterion..."`. Replace that with `{{.Criteria}}`.

## 5. Create changelog.md

Create `pkg/report/prompts/changelog.md` containing the text from `ChangelogSuffix()`.

The current function concatenates backtick-wrapped strings. The .md file must contain the final rendered text with actual backticks (not escaped), e.g.:
```
Update CHANGELOG.md following `/home/node/.claude/docs/changelog-guide.md`. Create `## Unreleased` if missing, extend it if it already exists.
```

## 6. Refactor suffix.go to use embedded templates

Update `pkg/report/suffix.go`:

a. Add import for the new prompts package:
```go
import (
    "strings"

    "github.com/bborbe/dark-factory/pkg/report/prompts"
)
```

b. Keep `MarkerStart` and `MarkerEnd` constants exactly as they are.

c. Rewrite `Suffix()` to simply return `prompts.CompletionReport`.

d. Rewrite `ValidationSuffix(cmd string)` to use `strings.Replace`:
```go
func ValidationSuffix(cmd string) string {
    return strings.Replace(prompts.ValidationCommand, "{{.Command}}", cmd, 1)
}
```

e. Rewrite `ValidationPromptSuffix(criteria string)` similarly:
```go
func ValidationPromptSuffix(criteria string) string {
    return strings.Replace(prompts.ValidationPrompt, "{{.Criteria}}", criteria, 1)
}
```

f. Rewrite `ChangelogSuffix()` to return `prompts.Changelog`.

g. Do NOT use `text/template` — `strings.Replace` is simpler and sufficient for single-placeholder substitution.

## 7. Verify byte-identical output

After making changes, the existing tests in `pkg/report/report_test.go` and `pkg/processor/processor_test.go` must pass without modification. These tests use `ContainSubstring` and `HaveSuffix` matchers that validate the exact content.

Pay special attention to:
- Leading `\n\n---` sequences
- Trailing newlines
- The `HaveSuffix(report.Suffix())` check in processor_test.go (~line 1288) which requires exact match at the end of the string

</requirements>

<constraints>
- Pure refactor — no behavior change, no new features
- Do NOT modify any test files
- Do NOT modify pkg/processor/processor.go or any caller
- Do NOT move or rename the MarkerStart / MarkerEnd constants
- Do NOT use text/template — use strings.Replace for placeholder substitution
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` in the project root — must pass with zero failures.

Additionally, verify the new files exist:
```
ls pkg/report/prompts/prompts.go
ls pkg/report/prompts/completion-report.md
ls pkg/report/prompts/validation-command.md
ls pkg/report/prompts/validation-prompt.md
ls pkg/report/prompts/changelog.md
```
</verification>
