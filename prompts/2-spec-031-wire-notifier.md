---
status: created
spec: ["031"]
created: "2026-03-11T22:30:00Z"
---
<summary>
- A `Notifier` is injected into the processor so prompt failures and partial completions fire notifications
- The spec auto-completer fires a notification when a spec transitions to `verifying`
- The review poller fires a notification when the retry limit is exhausted
- The runner fires a notification for each prompt stuck in `executing` state at startup
- The factory creates a `Notifier` from config (Telegram + Discord, or no-op) and injects it everywhere
- All notification calls are non-blocking: a 5-second timeout is set inside each channel implementation
- No behavioral change to processing — notifications are additive side effects only
- When `notifications:` is absent from config, a no-op notifier is used — zero overhead, zero config required
- All five trigger points are covered by tests using the counterfeiter `FakeNotifier`
</summary>

<objective>
Wire the `pkg/notifier.Notifier` (created in the previous prompt) into all five trigger points: processor (prompt failed, prompt partial), spec.AutoCompleter (spec→verifying), review.ReviewPoller (retry limit), and runner (stuck container at startup). Update `pkg/factory/factory.go` to construct the notifier from config and inject it everywhere.
</objective>

<context>
**Prerequisite**: Prompt 1 (`1-spec-031-notifier-package.md`) must be completed before executing this prompt. It creates `pkg/notifier/`, `pkg/config/NotificationsConfig`, and `mocks/notifier.go`.

Read CLAUDE.md for project conventions.
Read `pkg/notifier/notifier.go` — the Notifier interface and Event struct (created by prompt 1). Key types: `Notifier` interface with `Notify(ctx, Event) error`, `Event` struct with fields `ProjectName`, `EventType`, `PromptName`, `PRURL` (all strings).
Read `pkg/processor/processor.go` — add Notifier field; fire in `handlePromptFailure` and in `validateCompletionReport` (partial status).
Read `pkg/spec/spec.go` — add Notifier to `autoCompleter`; fire after `sf.MarkVerifying()` in `CheckAndComplete`.
Read `pkg/review/poller.go` — add Notifier to `reviewPoller`; fire in `handleChangesRequested` when `retryCount >= p.maxRetries`.
Read `pkg/runner/runner.go` — fire notification for each prompt reset by `ResetExecuting` at startup. The runner needs a Notifier to call after `promptManager.ResetExecuting`.
Read `pkg/factory/factory.go` — add `CreateNotifier(cfg config.Config) notifier.Notifier` factory function, inject into all four constructors.
Read `/home/node/.claude/docs/go-patterns.md` for constructor patterns.
Read `/home/node/.claude/docs/go-testing.md` for Ginkgo/Gomega and counterfeiter usage.
</context>

<requirements>
1. **`pkg/processor/processor.go`** — inject `Notifier`:

   Add `notifier notifier.Notifier` field to `processor` struct.
   Add `notifier notifier.Notifier` parameter to `NewProcessor` (append to existing params).

   In `handlePromptFailure`, after logging the error, call:
   ```go
   p.notifier.Notify(ctx, notifier.Event{
       ProjectName: p.projectName,
       EventType:   "prompt_failed",
       PromptName:  filepath.Base(path),
   })
   ```
   (ignore the returned error — it is already handled inside the multi-notifier)

   For partial completion: `validateCompletionReport` is a package-level function (not a method) that returns an error for any non-success status. Add a helper method on `processor` to check for partial status and notify:

   ```go
   func (p *processor) notifyFromReport(ctx context.Context, logFile string, promptPath string) {
       completionReport, err := report.ParseFromLog(ctx, logFile)
       if err != nil || completionReport == nil {
           return
       }
       if completionReport.Status == "partial" {
           p.notifier.Notify(ctx, notifier.Event{
               ProjectName: p.projectName,
               EventType:   "prompt_partial",
               PromptName:  filepath.Base(promptPath),
           })
       }
   }
   ```

   Call `p.notifyFromReport(ctx, logFile, promptPath)` in `handlePostExecution` immediately after `validateCompletionReport` returns an error:
   ```go
   summary, err := validateCompletionReport(ctx, logFile)
   if err != nil {
       p.notifyFromReport(ctx, logFile, promptPath)
       return err
   }
   ```

2. **`pkg/spec/spec.go`** — inject `Notifier` into `autoCompleter`:

   Add `notifier notifier.Notifier` field to `autoCompleter` struct.
   Add `notifier notifier.Notifier` parameter to `NewAutoCompleter` (append to existing params).

   Add `projectName string` and `notifier notifier.Notifier` fields to `autoCompleter` struct and `NewAutoCompleter` params:
   ```go
   func NewAutoCompleter(
       queueDir, completedDir, specsInboxDir, specsInProgressDir, specsCompletedDir string,
       currentDateTimeGetter libtime.CurrentDateTimeGetter,
       projectName string,
       notifier notifier.Notifier,
   ) AutoCompleter
   ```

   In `CheckAndComplete`, **after** `sf.Save()` succeeds (not before — only notify if the state was persisted):
   ```go
   a.notifier.Notify(ctx, notifier.Event{
       ProjectName: a.projectName,
       EventType:   "spec_verifying",
       PromptName:  sf.Name,
   })
   ```

3. **`pkg/review/poller.go`** — inject `Notifier`:

   Add `notifier notifier.Notifier` field to `reviewPoller` struct.
   Add `notifier notifier.Notifier` parameter to `NewReviewPoller` (append to existing params).

   In `handleChangesRequested`, after setting `FailedPromptStatus` when `retryCount >= p.maxRetries`:
   ```go
   p.notifier.Notify(ctx, notifier.Event{
       ProjectName: "",         // reviewPoller does not have projectName — pass it as a new field
       EventType:   "review_limit",
       PromptName:  filepath.Base(path),
       PRURL:       prURL,
   })
   ```

   Add `projectName string` field and constructor param to `reviewPoller` as well.

   Updated `NewReviewPoller` signature (append `projectName string, notifier notifier.Notifier`):
   ```go
   func NewReviewPoller(
       queueDir string,
       inboxDir string,
       allowedReviewers []string,
       maxRetries int,
       pollInterval time.Duration,
       fetcher git.ReviewFetcher,
       prMerger git.PRMerger,
       promptManager prompt.Manager,
       generator FixPromptGenerator,
       projectName string,
       notifier notifier.Notifier,
   ) ReviewPoller
   ```

4. **`pkg/runner/runner.go`** — fire notification for stuck containers:

   The runner calls `r.promptManager.ResetExecuting(ctx)` which resets prompts stuck in "executing" state from a previous crash. To detect which prompts were stuck, we need to know how many were reset.

   Change the `prompt.Manager.ResetExecuting` signature to return `[]string` (paths of reset prompts) OR add a separate step: scan for executing prompts BEFORE calling ResetExecuting, then fire one notification per stuck prompt.

   **Preferred approach** (minimal change to Manager interface): before calling `r.promptManager.ResetExecuting(ctx)`, scan `r.inProgressDir` for prompts with `status: executing`, collect their names, call `ResetExecuting`, then for each stuck prompt fire:
   ```go
   r.notifier.Notify(ctx, notifier.Event{
       ProjectName: r.projectName,
       EventType:   "stuck_container",
       PromptName:  entry.Name(),
   })
   ```

   Add `projectName string` and `notifier notifier.Notifier` fields and params to `NewRunner`.

   Add a helper method to runner:
   ```go
   func (r *runner) notifyStuckContainers(ctx context.Context) {
       entries, err := os.ReadDir(r.inProgressDir)
       if err != nil {
           return
       }
       for _, entry := range entries {
           if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
               continue
           }
           path := filepath.Join(r.inProgressDir, entry.Name())
           fm, err := r.promptManager.ReadFrontmatter(ctx, path)
           if err != nil {
               continue
           }
           if prompt.PromptStatus(fm.Status) == prompt.ExecutingPromptStatus {
               r.notifier.Notify(ctx, notifier.Event{
                   ProjectName: r.projectName,
                   EventType:   "stuck_container",
                   PromptName:  entry.Name(),
               })
           }
       }
   }
   ```

   Call `r.notifyStuckContainers(ctx)` immediately before `r.promptManager.ResetExecuting(ctx)` in `Run`.

5. **`pkg/factory/factory.go`** — add `CreateNotifier` and wire everywhere:

   ```go
   // CreateNotifier creates a Notifier from config, or a no-op if no channels are configured.
   func CreateNotifier(cfg config.Config) notifier.Notifier {
       var notifiers []notifier.Notifier
       if token := cfg.ResolvedTelegramBotToken(); token != "" {
           chatID := cfg.ResolvedTelegramChatID()
           if chatID != "" {
               notifiers = append(notifiers, notifier.NewTelegramNotifier(token, chatID))
           }
       }
       if webhook := cfg.ResolvedDiscordWebhook(); webhook != "" {
           notifiers = append(notifiers, notifier.NewDiscordNotifier(webhook))
       }
       return notifier.NewMultiNotifier(notifiers...)
   }
   ```

   In `CreateRunner`, create `n := CreateNotifier(cfg)` and pass it to:
   - `CreateProcessor(...)` — add `n` as last param
   - `CreateReviewPoller(...)` (inside `createOptionalReviewPoller`) — need to thread it through
   - `NewRunner(...)` — add `projectName, n` as new params

   Update `createOptionalReviewPoller` to accept `cfg config.Config, promptManager prompt.Manager, projectName string, n notifier.Notifier`.

   Update `CreateProcessor` to accept `n notifier.Notifier` as last param and pass it to `processor.NewProcessor`.

   Update `spec.NewAutoCompleter(...)` calls in `CreateProcessor` to pass `projectName, n` as new params.

   Update `CreateOneShotRunner` similarly.

   Update `CreateReviewPoller` to accept `projectName string, n notifier.Notifier` and pass both to `review.NewReviewPoller`.

6. **Tests** — update all affected tests to pass a `FakeNotifier` (from `mocks/notifier.go`) where a `Notifier` param is needed:
   - `pkg/processor/processor_internal_test.go` or `pkg/processor/processor_test.go` — verify `FakeNotifier.NotifyCallCount()` is 1 on prompt failure and on partial completion
   - `pkg/spec/spec_test.go` — verify `FakeNotifier.NotifyCallCount()` is 1 when `CheckAndComplete` transitions to verifying
   - `pkg/review/poller_test.go` — verify `FakeNotifier.NotifyCallCount()` is 1 when retry limit is exhausted
   - `pkg/runner/runner_test.go` — verify `notifyStuckContainers` fires when a prompt has `executing` status

7. Run `make generate` to regenerate mocks after any interface changes.
</requirements>

<constraints>
- Config shape is a frozen contract: Telegram uses `botTokenEnv` + `chatIDEnv`, Discord uses `webhookEnv` (env var names, not secrets)
- Notification delivery must not block the main processing loop — short timeout (5s) is inside each channel impl (already done in prompt 1)
- Failed notification delivery logs a warning but does not fail the prompt/spec lifecycle
- Bot tokens and webhook URLs must NEVER appear in log output
- Webhook URLs must use HTTPS only (validated in prompt 1; no re-validation needed here)
- Notifications are purely additive — no behavioral changes to existing processing
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
</constraints>

<verification>
Run `make precommit` — must pass.
</verification>
