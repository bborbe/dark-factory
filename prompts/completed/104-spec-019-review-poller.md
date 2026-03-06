---
status: completed
summary: Created ReviewPoller in pkg/review/poller.go with 13 tests covering all verdict/state paths, regenerated mocks including mocks/review_poller.go, and fixed revive lint stutter with nolint comment
container: dark-factory-104-spec-019-review-poller
dark-factory-version: v0.17.29
created: "2026-03-06T14:55:39Z"
queued: "2026-03-06T14:55:39Z"
started: "2026-03-06T14:55:39Z"
completed: "2026-03-06T15:09:57Z"
---
<objective>
Create the `ReviewPoller` that watches all `in_review` prompts, fetches GitHub review state, generates fix prompts on request-changes, triggers merge on approval, and handles retry limits and external close/merge. This is the core loop of spec 018.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/processor/processor.go for how the processor struct is structured and how it uses injected dependencies.
Read pkg/git/review_fetcher.go for ReviewFetcher and ReviewVerdict (just added).
Read pkg/review/fix_prompt_generator.go for FixPromptGenerator and GenerateOpts (just added).
Read pkg/git/pr_merger.go for PRMerger.WaitAndMerge.
Read pkg/prompt/prompt.go for PromptFile, Manager interface, StatusInReview, StatusCompleted, StatusFailed, Branch(), PRURL(), RetryCount(), IncrementRetryCount().
</context>

<requirements>
1. Create `pkg/review/poller.go` with:

   ```go
   //counterfeiter:generate -o ../../mocks/review_poller.go --fake-name ReviewPoller . ReviewPoller
   type ReviewPoller interface {
       Run(ctx context.Context) error
   }
   ```

   Constructor:
   ```go
   func NewReviewPoller(
       queueDir      string,
       inboxDir      string,
       allowedReviewers []string,
       maxRetries    int,
       pollInterval  time.Duration,
       fetcher       git.ReviewFetcher,
       prMerger      git.PRMerger,
       promptManager prompt.Manager,
       generator     FixPromptGenerator,
   ) ReviewPoller
   ```

   `Run` implementation:
   - Loop until ctx is cancelled
   - Scan queueDir for prompts with `status: in_review`
   - For each `in_review` prompt:
     a. Read `PRURL()` — if empty, log warning and skip
     b. Call `fetcher.FetchPRState(ctx, prURL)`:
        - "MERGED" → set status `completed`, move to completedDir, continue
        - "CLOSED" → set status `failed`, move to completedDir, continue
     c. Call `fetcher.FetchLatestReview(ctx, prURL, allowedReviewers)`:
        - `ReviewVerdictNone` → skip (no trusted review yet)
        - `ReviewVerdictApproved` → call `prMerger.WaitAndMerge(ctx, prURL)`, on success set status `completed`
        - `ReviewVerdictChangesRequested`:
          - If `RetryCount() >= maxRetries` → set status `failed`, log "retry limit reached"
          - Otherwise → call `generator.Generate(ctx, GenerateOpts{...})`, call `promptManager.IncrementRetryCount(ctx, path)`
   - After scanning all prompts, sleep `pollInterval`
   - All errors are logged as warnings — never stop the loop for a single prompt failure

2. Create `pkg/review/poller_test.go`:
   - Approved review from trusted reviewer → WaitAndMerge called, status set to completed
   - Changes-requested within retry limit → generator.Generate called, RetryCount incremented
   - Changes-requested at retry limit → status set to failed, generator not called
   - MERGED PR → status set to completed without calling fetcher
   - CLOSED PR → status set to failed
   - No review yet (VerdictNone) → no action taken
   - Error from fetcher → warning logged, loop continues

3. Regenerate mocks with `go generate ./...`.
</requirements>

<constraints>
- Poller errors are non-fatal — log warnings, never stop the loop
- Do NOT call prMerger for CLOSED/MERGED detection — use FetchPRState
- Do NOT commit — dark-factory handles git
- Coverage ≥ 80% for pkg/review/poller.go
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
