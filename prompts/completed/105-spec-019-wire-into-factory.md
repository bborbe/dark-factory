---
status: completed
container: dark-factory-105-spec-019-wire-into-factory
dark-factory-version: v0.17.29
created: "2026-03-06T15:10:01Z"
queued: "2026-03-06T15:10:01Z"
started: "2026-03-06T15:10:01Z"
completed: "2026-03-06T15:26:19Z"
spec: ["019"]
---
<objective>
Wire the review-fix loop into the factory and runner: transition prompts to `in_review` after PR creation (when autoReview is enabled), and add the ReviewPoller as a third goroutine alongside watcher and processor. This completes spec 018.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read pkg/factory/factory.go — specifically CreateRunner and CreateProcessor.
Read pkg/runner/runner.go — specifically the Run method and run.CancelOnFirstError call.
Read pkg/processor/processor.go — specifically handlePRWorkflow and handleWorktreeWorkflow where PR URL is saved.
Read pkg/review/poller.go for ReviewPoller interface (just added).
Read pkg/git/review_fetcher.go for ReviewFetcher (just added).
Read pkg/review/fix_prompt_generator.go for FixPromptGenerator (just added).
Read pkg/config/config.go for AutoReview, MaxReviewRetries, AllowedReviewers, UseCollaborators, PollIntervalSec.
</context>

<requirements>
1. In `pkg/processor/processor.go`, after PR creation succeeds and `autoMerge` is disabled:
   - If `p.autoReview` is true, set prompt status to `in_review` instead of completing the prompt
   - The prompt stays in queueDir with status `in_review` — do NOT move it to completedDir
   - Log: `slog.Info("PR created, waiting for review", "url", prURL)`
   - Add `autoReview bool` field to processor struct and parameter to NewProcessor

2. In `pkg/factory/factory.go`:
   - Add `CreateReviewPoller(cfg config.Config, promptManager prompt.Manager) review.ReviewPoller`
   - Resolve allowed reviewers: use `cfg.AllowedReviewers` if non-empty, otherwise fetch collaborators via `gh api repos/{owner}/{repo}/collaborators --jq '.[].login'` (add a `CollaboratorFetcher` or inline in factory)
   - Pass `cfg.AutoReview` to `CreateProcessor`
   - Update `CreateRunner` to accept and store a `ReviewPoller`

3. In `pkg/runner/runner.go`:
   - Add `reviewPoller review.ReviewPoller` field (optional — nil if autoReview disabled)
   - In `Run`, if `reviewPoller != nil`, add it to the `runners` slice passed to `run.CancelOnFirstError`
   - Update `NewRunner` to accept `reviewPoller review.ReviewPoller`

4. In `pkg/factory/factory.go`, update `CreateRunner`:
   - If `cfg.AutoReview`, create and pass `ReviewPoller` to runner; otherwise pass nil

5. Add tests to `pkg/processor/processor_test.go`:
   - WorkflowPR + autoReview=true: after PR creation, status set to `in_review`, prompt NOT moved to completedDir
   - WorkflowPR + autoReview=false: existing behavior unchanged (prompt completed normally)

6. Add tests to `pkg/runner/runner_test.go`:
   - Runner with non-nil reviewPoller includes it in run.CancelOnFirstError
   - Runner with nil reviewPoller does not include it

7. Regenerate mocks with `go generate ./...`.
</requirements>

<constraints>
- ReviewPoller is optional — nil is valid when autoReview=false
- Do NOT change processor behavior when autoReview=false
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
