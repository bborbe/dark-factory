---
status: completed
spec: [085-auto-inject-hidegit-guidance]
summary: Added hideGit parameter to promptenricher.NewEnricher with guidance fragment prepended when true
container: dark-factory-exec-401-add-hidegit-guidance-to-enricher
dark-factory-version: v0.164.0
created: "2026-05-21T21:45:00Z"
queued: "2026-05-21T21:43:02Z"
started: "2026-05-21T21:57:49Z"
completed: "2026-05-21T22:06:21Z"
branch: dark-factory/auto-inject-hidegit-guidance
---

<summary>
- `pkg/promptenricher.NewEnricher` gains a new `hideGit bool` constructor parameter
- The fragment text is defined once as a constant in the `pkg/promptenricher` package
- When `hideGit=true`, `Enrich` prepends the fragment after `additionalInstructions` and before the prompt body
- When `hideGit=false` (the default), `Enrich` emits prompts byte-identical to today
- The `Enrich` method signature is unchanged — only the constructor changes
- Unit tests cover: fragment prepended after `additionalInstructions`, fragment not present when `hideGit=false`, ordering assertions
</summary>

<objective>
Implement the hideGit guidance fragment in the prompt enricher. When `hideGit=true`, every enriched prompt receives a constant guidance fragment prepended after `additionalInstructions` (if any) explaining that `/workspace/.git` appears as a character device by design, that `GOFLAGS=-buildvcs=false` is typically set, and that the agent should run the project's precommit gate regardless of `.git`'s appearance.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/promptenricher/enricher.go` — the existing Enricher interface and `Enrich` method (lines 20-82), the constructor `NewEnricher` (lines 27-43), and the struct definition (lines 45-52).
Read `pkg/promptenricher/enricher_test.go` — existing Ginkgo/Gomega test patterns for the enricher.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — the `NewEnricher` is a factory function; constructor parameter additions are additive and do not break callers if all call sites are updated.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — test structure, Gomega assertions, mock usage.
Read `pkg/report/report.go` — to understand the existing suffix pattern (e.g., `Suffix()`, `ChangelogSuffix()`) this spec's fragment follows.
</context>

<requirements>
1. In `pkg/promptenricher/enricher.go`:
   - Add `hideGit bool` as the last parameter to `NewEnricher`
   - Add `hideGit bool` field to the `enricher` struct
   - Add a package-level constant `hideGitGuidanceFragment` (type `string`) containing the ~200-word guidance text. The constant MUST contain the literal substrings `character device` AND `hideGit=true active` (these are the spec's grep markers — both substrings must appear verbatim). Content must:
     - Name `/workspace/.git` as a character device that appears when `hideGit=true` is active
     - State the mask is intentional dark-factory behavior, not a broken repo
     - State that `GOFLAGS=-buildvcs=false` is typically already set, and that `go test`, `errcheck`, `gosec`, `golangci-lint`, etc. work without git metadata
     - Instruct the agent to run `make precommit` (or the project's equivalent gate) regardless of `.git`'s appearance
   - Rewrite the existing `additionalInstructions` prepend block at `enricher.go:56-58` to also handle the fragment, producing the order `additionalInstructions → fragment → prompt body → suffixes`. Replace this existing block:
     ```go
     if e.additionalInstructions != "" {
         content = e.additionalInstructions + "\n\n" + content
     }
     ```
     with:
     ```go
     prefix := ""
     if e.additionalInstructions != "" {
         prefix = e.additionalInstructions + "\n\n"
     }
     if e.hideGit {
         prefix = prefix + hideGitGuidanceFragment + "\n\n"
     }
     content = prefix + content
     ```
     CRITICAL: do NOT append the fragment to `content` after the existing prepend block — that would place the fragment AFTER the prompt body, contradicting the spec's required ordering (`additionalInstructions → fragment → PROMPT_BODY`) and breaking the requirement-2 tests. The fragment must be part of the prefix chain, not a separate append.

2. Update the OTHER `promptenricher.NewEnricher` call sites to pass `false` for the new `hideGit` parameter (preserves current behavior; the wire-hidegit-into-factory prompt updates the factory site to the resolved expression):
   - `pkg/factory/factory.go:954` — pass `false` as the last argument here. The sibling `085-wire-hidegit-into-factory-and-test.md` prompt will replace this `false` with the resolved expression `workflow == config.WorkflowWorktree || hideGit`. For now this prompt only adds the parameter to keep compilation green.
   - `pkg/processor/processor_test.go:138` — pass `false` (test default).
   - `pkg/processor/processor_retry_test.go:685` — pass `false` (test default).
   - `pkg/processor/processor_cancel_test.go:89` — pass `false` (test default).
   Without these updates `make precommit` fails at the compile step because the constructor signature changed.

3. In `pkg/promptenricher/enricher_test.go`:
   Add new test cases in the existing `Describe("Enrich", ...)` block:
   - `It("prepends hideGit fragment when hideGit=true and additionalInstructions is set", ...)` — construct enricher with `hideGit=true` and `additionalInstructions="PROJECT_HEADER"`, call `Enrich(ctx, "PROMPT_BODY")`, assert the result contains `PROJECT_HEADER`, the fragment's distinctive marker, and `PROMPT_BODY` in that order using `indexOf` for position comparisons
   - `It("prepends hideGit fragment when hideGit=true and additionalInstructions is empty", ...)` — construct enricher with `hideGit=true` and no additionalInstructions, call `Enrich(ctx, "PROMPT_BODY")`, assert the result contains the fragment marker followed by `PROMPT_BODY`
   - `It("does not prepend hideGit fragment when hideGit=false", ...)` — construct enricher with `hideGit=false`, call `Enrich(ctx, "PROMPT_BODY")`, assert the result does NOT contain the fragment marker
   - `It("preserves suffix ordering with hideGit fragment", ...)` — construct enricher with `hideGit=true`, `additionalInstructions="HEADER"`, and all suffixes active (HasChangelog=true, testCommand="make test", validationCommand="make precommit"), call `Enrich`, assert ordering: HEADER → fragment → PROMPT_BODY → report → changelog → test → validation → criteria

   Update the `newEnricher` helper function to accept `hideGit bool` as last parameter and pass it to `promptenricher.NewEnricher`.
</requirements>

<constraints>
- The `Enrich` method signature (`Enrich(ctx context.Context, content string) string`) does not change
- The fragment text is defined exactly once as a package-level constant in `pkg/promptenricher/`
- Existing `Enrich` behavior (prepending `additionalInstructions`, appending completion/changelog/test/validation/criteria suffixes) is preserved exactly
- The fragment is prepended only when `hideGit=true`; there is no other condition
- All existing tests must still pass — update the `newEnricher` helper but do not remove or modify existing test cases
- Do NOT commit — dark-factory handles git
- Use `errors.Wrapf(ctx, err, "message")` for error wrapping — never `fmt.Errorf`
</constraints>

<verification>
```bash
go test ./pkg/promptenricher/... -v
make precommit
```
All tests must pass including the new ones.
</verification>
