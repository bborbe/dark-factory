---
status: completed
spec: [019-review-fix-loop]
summary: Created FixPromptGenerator in pkg/review with interface, implementation, tests, and counterfeiter mock
container: dark-factory-103-spec-019-fix-prompt-generator
dark-factory-version: v0.17.29
created: "2026-03-06T14:49:47Z"
queued: "2026-03-06T14:49:47Z"
started: "2026-03-06T14:49:47Z"
completed: "2026-03-06T14:55:35Z"
---
<objective>
Create a `FixPromptGenerator` that writes a fix prompt to the inbox when a PR receives a `request-changes` review. The generated prompt targets the existing branch and PR (using spec 017 fields). Depends on spec-018-retry-count-frontmatter and spec-018-review-fetcher.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/prompt/prompt.go for PromptFile, Branch(), PRURL(), frontmatter structure.
Read prompts/completed/ for examples of real prompt files to understand the expected format.
The fix prompt lands in inboxDir — NOT queueDir. Human approves by moving to queue.
</context>

<requirements>
1. Create `pkg/review/fix_prompt_generator.go` with:

   ```go
   //counterfeiter:generate -o ../../mocks/fix_prompt_generator.go --fake-name FixPromptGenerator . FixPromptGenerator
   type FixPromptGenerator interface {
       Generate(ctx context.Context, opts GenerateOpts) error
   }

   type GenerateOpts struct {
       InboxDir    string
       OriginalPromptName string // filename without path, used to derive fix prompt name
       Branch      string
       PRURL       string
       RetryCount  int
       ReviewBody  string
   }
   ```

   Implementation:
   - Derive fix prompt filename: `fix-<originalPromptName>-retry-<retryCount>.md` (no number prefix — dark-factory assigns)
   - Write to `filepath.Join(opts.InboxDir, filename)`
   - If file already exists → return nil (idempotent, don't overwrite)
   - File content (no frontmatter — dark-factory adds it):

   ```
   <objective>
   Fix the issues raised in the code review for PR <prURL>.
   </objective>

   <context>
   Read CLAUDE.md for project conventions.
   This is a follow-up fix for branch <branch>.
   </context>

   <requirements>
   Fix all issues raised in the review feedback below. Do not change unrelated code.
   </requirements>

   <review_feedback>
   <reviewBody>
   </review_feedback>

   <constraints>
   - Do NOT commit — dark-factory handles git
   - Run make precommit to verify
   </constraints>

   <verification>
   Run `make precommit` — must pass.
   </verification>
   ```

   Note: The generated file has no YAML frontmatter. Do NOT add `branch` or `pr-url` here — those will be injected by the review poller after generation (next prompt).

2. Create `pkg/review/fix_prompt_generator_test.go` and `pkg/review/review_suite_test.go`:
   - Generate creates file with correct content in inboxDir
   - Generate is idempotent (second call does not overwrite existing file)
   - Generated filename contains originalPromptName and retryCount
</requirements>

<constraints>
- Fix prompt lands in inboxDir only — never queueDir
- Idempotent: skip if file already exists
- Do NOT modify existing files
- Do NOT commit — dark-factory handles git
- Coverage ≥ 80% for pkg/review
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
