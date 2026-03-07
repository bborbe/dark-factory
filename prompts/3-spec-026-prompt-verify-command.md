---
spec: ["026"]
status: created
created: "2026-03-07T22:30:00Z"
---
<summary>
- Adds `dark-factory prompt verify <file>` CLI command
- Command finds the prompt by ID in queueDir, validates it is in `pending_verification` state
- On validation success: moves prompt to completed/, commits the file, then runs the workflow-specific git flow (direct: tag+push; PR/worktree: push branch + create PR)
- Returns a clear error if status is not `pending_verification`
- Wired into `pkg/factory/factory.go` as `CreatePromptVerifyCommand`
- Wired into `main.go` `runPromptCommand` as `"verify"` case and added to `printHelp`
- Fully tested with Ginkgo/counterfeiter mocks
</summary>

<objective>
Add the `dark-factory prompt verify <file>` command that completes the human verification gate. After the human has run host-side checks on a `pending_verification` prompt, this command moves the prompt to `completed/`, commits the file, and runs the same post-completion git flow the processor would have run (commit, tag, push for direct workflow; commit, push, PR for PR/worktree workflow).
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `pkg/cmd/approve.go` — structure of a command with promptManager dependency; follow this pattern for the new command.
Read `pkg/cmd/requeue.go` — FindPromptFile usage, how to load and modify a prompt file.
Read `pkg/cmd/spec_complete.go` — example of a command that transitions a state and calls a git operation.
Read `pkg/cmd/prompt_finder.go` — `FindPromptFile` function signature and behavior.
Read `pkg/prompt/prompt.go` — `PendingVerificationPromptStatus`, `MarkPendingVerification()`, `MarkCompleted()`, Manager interface (`MoveToCompleted`, `Load`).
Read `pkg/processor/processor.go` — `moveToCompletedAndCommit`, `handleDirectWorkflow`, `handlePRWorkflow` to understand what the verify command must replicate.
Read `pkg/git/` — Releaser interface (CommitCompletedFile, CommitAndRelease, CommitOnly, HasChangelog), Brancher interface (Push), PRCreator interface (Create).
Read `pkg/config/config.go` — `Workflow`, `WorkflowDirect`, `WorkflowPR`, `WorkflowWorktree` constants.
Read `pkg/factory/factory.go` — `CreateApproveCommand` as the factory pattern to follow; `CreateProcessor` as reference for injecting git services.
Read `main.go` — `runPromptCommand` switch and `printHelp`.
Read `/home/node/.claude/docs/go-patterns.md` and `/home/node/.claude/docs/go-testing.md`.
</context>

<requirements>
1. Create `pkg/cmd/prompt_verify.go`:

   Interface with counterfeiter annotation:
   ```go
   //counterfeiter:generate -o ../../mocks/prompt-verify-command.go --fake-name PromptVerifyCommand . PromptVerifyCommand
   type PromptVerifyCommand interface {
       Run(ctx context.Context, args []string) error
   }
   ```

   Private struct:
   ```go
   type promptVerifyCommand struct {
       queueDir     string
       completedDir string
       promptManager prompt.Manager
       releaser      git.Releaser
       workflow      config.Workflow
       brancher      git.Brancher
       prCreator     git.PRCreator
   }
   ```

   Constructor:
   ```go
   func NewPromptVerifyCommand(
       queueDir string,
       completedDir string,
       promptManager prompt.Manager,
       releaser git.Releaser,
       workflow config.Workflow,
       brancher git.Brancher,
       prCreator git.PRCreator,
   ) PromptVerifyCommand
   ```

   `Run` method:
   - Requires exactly one arg; return `errors.Errorf(ctx, "usage: dark-factory prompt verify <file>")` if missing.
   - Call `FindPromptFile(ctx, c.queueDir, args[0])` to locate the prompt. Return the error if not found.
   - Load the prompt: `pf, err := prompt.Load(ctx, path)`.
   - Validate status: if `pf.Frontmatter.Status != string(prompt.PendingVerificationPromptStatus)`, return:
     ```go
     errors.Errorf(ctx, "prompt is not in pending verification state (current: %s)", pf.Frontmatter.Status)
     ```
   - Extract title for git commit message: `title := pf.Title()` (fall back to base filename if empty).
   - Derive `completedPath := filepath.Join(c.completedDir, filepath.Base(path))`.
   - Use a non-cancellable context for git ops: `gitCtx := context.WithoutCancel(ctx)`.
   - Call `c.promptManager.MoveToCompleted(ctx, path)` to move the file and mark it completed.
   - Call `c.releaser.CommitCompletedFile(gitCtx, completedPath)` to commit the moved file.
   - Then branch on `c.workflow`:
     - `config.WorkflowDirect`: call `c.completeDirectWorkflow(gitCtx, ctx, title)`.
     - `config.WorkflowPR` or `config.WorkflowWorktree`: call `c.completePRWorkflow(gitCtx, ctx, pf, title, completedPath)`.
   - Print `fmt.Printf("verified: %s\n", filepath.Base(path))`.
   - Return nil.

2. Add `completeDirectWorkflow` private method:
   ```go
   func (c *promptVerifyCommand) completeDirectWorkflow(gitCtx, ctx context.Context, title string) error
   ```
   Implementation mirrors `processor.handleDirectWorkflow`:
   - If `!c.releaser.HasChangelog(gitCtx)`: call `c.releaser.CommitOnly(gitCtx, title)`, log "committed changes", return nil.
   - Else: call `c.releaser.CommitAndRelease(gitCtx, determineBump())`, log "committed and released", return nil.
   - `determineBump()` is a package-level function in `pkg/processor/processor.go`. Since `prompt_verify.go` is in package `cmd`, replicate the bump-determination logic locally as a private unexported function `determineBumpFromChangelog() git.VersionBump`:
     - Read `CHANGELOG.md`; if any line in the `## Unreleased` section starts with `"- feat:"`, return `git.MinorBump`; else return `git.PatchBump`.
     - If CHANGELOG.md does not exist or cannot be read, return `git.PatchBump`.

3. Add `completePRWorkflow` private method:
   ```go
   func (c *promptVerifyCommand) completePRWorkflow(
       gitCtx context.Context,
       ctx context.Context,
       pf *prompt.PromptFile,
       title string,
       completedPath string,
   ) error
   ```
   Implementation:
   - Determine the branch name: `branch := pf.Branch()`. If empty, derive it from the prompt filename: `branch = "dark-factory/" + strings.TrimSuffix(filepath.Base(completedPath), ".md")`.
   - Call `c.releaser.CommitOnly(gitCtx, title)` to commit code changes on the branch.
   - Call `c.brancher.Push(gitCtx, branch)` to push the branch.
   - Call `c.prCreator.Create(gitCtx, title, "Automated by dark-factory")` to create the PR.
   - Log: `slog.Info("created PR", "url", prURL)`.
   - Return nil.
   Note: This method intentionally does NOT handle auto-merge — the human can merge via GitHub after verifying. Auto-merge is a separate feature.

4. Run `make generate` to create `mocks/prompt-verify-command.go`.

5. Create `pkg/cmd/prompt_verify_test.go` (package `cmd_test`, Ginkgo):
   Test cases:
   - No args → error containing "usage: dark-factory prompt verify".
   - Prompt not found → error propagated from FindPromptFile.
   - Prompt status is `approved` → error containing "not in pending verification state".
   - Prompt status is `failed` → error containing "not in pending verification state".
   - Prompt status is `pending_verification`, workflow `direct`, no CHANGELOG:
     → `MoveToCompleted` called, `CommitCompletedFile` called, `CommitOnly` called, prints "verified: <file>".
   - Prompt status is `pending_verification`, workflow `direct`, CHANGELOG with `- feat:` in Unreleased:
     → `CommitAndRelease` called with `git.MinorBump`.
   - Prompt status is `pending_verification`, workflow `pr`:
     → `MoveToCompleted`, `CommitCompletedFile`, `CommitOnly`, `Push`, `Create` (PR) all called.
   - Prompt with no branch in frontmatter (workflow `pr`) → branch name derived as `"dark-factory/" + slug`.
   Use counterfeiter mocks for `prompt.Manager`, `git.Releaser`, `git.Brancher`, `git.PRCreator`. These mocks already exist in `mocks/` — check before generating.

6. In `pkg/factory/factory.go`, add:
   ```go
   // CreatePromptVerifyCommand creates a PromptVerifyCommand.
   func CreatePromptVerifyCommand(cfg config.Config) cmd.PromptVerifyCommand {
       promptManager, releaser := createPromptManager(
           cfg.Prompts.InboxDir,
           cfg.Prompts.InProgressDir,
           cfg.Prompts.CompletedDir,
       )
       ghToken := cfg.ResolvedGitHubToken()
       return cmd.NewPromptVerifyCommand(
           cfg.Prompts.InProgressDir,
           cfg.Prompts.CompletedDir,
           promptManager,
           releaser,
           cfg.Workflow,
           git.NewBrancher(),
           git.NewPRCreator(ghToken),
       )
   }
   ```
   Place it after `CreateRequeueCommand`.

7. In `main.go`, in `runPromptCommand`, add:
   ```go
   case "verify":
       return factory.CreatePromptVerifyCommand(cfg).Run(ctx, args)
   ```
   Place it after the `"retry"` case.

8. In `main.go`, in `printHelp`, add the new subcommand in the prompt section:
   ```
   "  prompt verify <id>     Verify a pending-verification prompt (triggers commit/push)\n"+
   ```
   Place it after `"  prompt retry           Shorthand for prompt requeue --failed\n"`.

9. Remove any imports that become unused.
</requirements>

<constraints>
- The verify command must return a clear error when the prompt is not in `pending_verification` — never silently succeed on wrong state
- `prompt verify` on a non-existent file must return an error (propagated from FindPromptFile)
- The command searches ONLY the queueDir (in-progress) for the prompt — prompts are in queueDir when pending_verification
- Do NOT implement auto-merge in the verify command — human merges the PR manually
- Do NOT modify the processor — all processor changes were done in prompt 2
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `make precommit` must pass
</constraints>

<verification>
Run `make precommit` — must pass.

Check coverage:
```bash
go test -cover ./pkg/cmd/... ./pkg/factory/...
```
Coverage must be ≥80% for `pkg/cmd`.

Smoke test (manual, if binary available):
```bash
# Create a fake pending-verification prompt in in-progress/
echo -e "---\nstatus: pending_verification\n---\n# Test verify\n\n<verification>\nmake test\n</verification>" \
  > prompts/in-progress/999-test-verify.md
dark-factory prompt verify 999-test-verify
# Should print: verified: 999-test-verify.md
# (will fail at git step if not in a real repo with changes, but status transition works)

# Wrong state
echo -e "---\nstatus: approved\n---\n# Test" > prompts/in-progress/998-test-wrong.md
dark-factory prompt verify 998-test-wrong
# Should print error: prompt is not in pending verification state (current: approved)

# Cleanup
rm -f prompts/in-progress/999-test-verify.md prompts/in-progress/998-test-wrong.md
```
</verification>
