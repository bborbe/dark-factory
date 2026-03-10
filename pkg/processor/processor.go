// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"
	stderrors "errors"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/report"
	"github.com/bborbe/dark-factory/pkg/spec"
	"github.com/bborbe/dark-factory/pkg/version"
)

var sanitizeContainerNameRegexp = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

//counterfeiter:generate -o ../../mocks/processor.go --fake-name Processor . Processor

// Processor processes queued prompts.
type Processor interface {
	Process(ctx context.Context) error
	ProcessQueue(ctx context.Context) error
}

// processor implements Processor.
type processor struct {
	queueDir          string
	completedDir      string
	logDir            string
	projectName       string
	executor          executor.Executor
	promptManager     prompt.Manager
	releaser          git.Releaser
	versionGetter     version.Getter
	ready             <-chan struct{}
	pr                bool
	brancher          git.Brancher
	prCreator         git.PRCreator
	cloner            git.Cloner
	autoMerge         bool
	autoRelease       bool
	autoReview        bool
	prMerger          git.PRMerger
	autoCompleter     spec.AutoCompleter
	specLister        spec.Lister
	validationCommand string
	verificationGate  bool
	skippedPrompts    map[string]time.Time // filename → mod time when skipped
}

// NewProcessor creates a new Processor.
func NewProcessor(
	queueDir string,
	completedDir string,
	logDir string,
	projectName string,
	exec executor.Executor,
	promptManager prompt.Manager,
	releaser git.Releaser,
	versionGetter version.Getter,
	ready <-chan struct{},
	pr bool,
	brancher git.Brancher,
	prCreator git.PRCreator,
	cloner git.Cloner,
	prMerger git.PRMerger,
	autoMerge bool,
	autoRelease bool,
	autoReview bool,
	autoCompleter spec.AutoCompleter,
	specLister spec.Lister,
	validationCommand string,
	verificationGate bool,
) Processor {
	return &processor{
		queueDir:          queueDir,
		completedDir:      completedDir,
		logDir:            logDir,
		projectName:       projectName,
		executor:          exec,
		promptManager:     promptManager,
		releaser:          releaser,
		versionGetter:     versionGetter,
		ready:             ready,
		pr:                pr,
		brancher:          brancher,
		prCreator:         prCreator,
		cloner:            cloner,
		autoMerge:         autoMerge,
		autoRelease:       autoRelease,
		autoReview:        autoReview,
		prMerger:          prMerger,
		autoCompleter:     autoCompleter,
		specLister:        specLister,
		validationCommand: validationCommand,
		verificationGate:  verificationGate,
		skippedPrompts:    make(map[string]time.Time),
	}
}

// Process starts processing queued prompts.
// It processes existing queued prompts on startup, then listens for signals from the watcher.
func (p *processor) Process(ctx context.Context) error {
	slog.Info("processor started")

	// Reset failed prompts to queued on startup
	if err := p.promptManager.ResetFailed(ctx); err != nil {
		return errors.Wrap(ctx, err, "reset failed prompts")
	}

	// Transition prompted specs with all prompts completed to verifying
	if err := p.checkPromptedSpecs(ctx); err != nil {
		return errors.Wrap(ctx, err, "check prompted specs on startup")
	}

	// Process any existing queued prompts first
	if err := p.processExistingQueued(ctx); err != nil {
		return errors.Wrap(ctx, err, "process existing queued prompts")
	}

	slog.Info("waiting for changes")

	// Listen for ready signals from watcher
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("processor shutting down")
			return nil

		case <-p.ready:
			// Watcher normalized files, check for new queued prompts
			// Clear skipped prompts so all files get re-evaluated after fsnotify event
			p.skippedPrompts = make(map[string]time.Time)
			if err := p.processExistingQueued(ctx); err != nil {
				return errors.Wrap(ctx, err, "process queued prompts")
			}

		case <-ticker.C:
			// Periodic scan for queued prompts (in case we missed a signal)
			if err := p.processExistingQueued(ctx); err != nil {
				return errors.Wrap(ctx, err, "periodic scan")
			}
		}
	}
}

// ProcessQueue runs the startup sequence and drains all queued prompts, then returns.
// Unlike Process, it does not enter the event loop — suitable for one-shot / CI usage.
func (p *processor) ProcessQueue(ctx context.Context) error {
	slog.Info("processor started (one-shot)")

	// Reset failed prompts to queued on startup
	if err := p.promptManager.ResetFailed(ctx); err != nil {
		return errors.Wrap(ctx, err, "reset failed prompts")
	}

	// Transition prompted specs with all prompts completed to verifying
	if err := p.checkPromptedSpecs(ctx); err != nil {
		return errors.Wrap(ctx, err, "check prompted specs on startup")
	}

	// Process all existing queued prompts and return
	if err := p.processExistingQueued(ctx); err != nil {
		return errors.Wrap(ctx, err, "process existing queued prompts")
	}

	// Log once when the queue is empty (one-shot mode only)
	queued, err := p.promptManager.ListQueued(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "list queued prompts")
	}
	if len(queued) == 0 {
		slog.Info("no queued prompts")
	}

	return nil
}

// processExistingQueued scans for and processes any existing queued prompts.
func (p *processor) processExistingQueued(ctx context.Context) error {
	// Block if any prompt is pending human verification
	if p.hasPendingVerification(ctx) {
		slog.Info("queue blocked: prompt pending verification")
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		// Scan for queued prompts
		queued, err := p.promptManager.ListQueued(ctx)
		if err != nil {
			return errors.Wrap(ctx, err, "list queued prompts")
		}

		// No more queued prompts - done
		if len(queued) == 0 {
			slog.Debug("queue scan complete", "queuedCount", 0)
			return nil
		}

		slog.Debug("queue scan complete", "queuedCount", len(queued))

		// Pick first prompt (already sorted alphabetically)
		pr := queued[0]

		// Auto-set status to queued if empty or created (folder location is source of truth)
		if err := p.autoSetQueuedStatus(ctx, &pr); err != nil {
			return err
		}

		// Check if prompt should be skipped (validation or previously failed)
		if p.shouldSkipPrompt(ctx, pr) {
			continue
		}

		// Check ordering - all previous prompts must be completed
		if !p.promptManager.AllPreviousCompleted(ctx, pr.Number()) {
			slog.Info(
				"prompt blocked",
				"file",
				filepath.Base(pr.Path),
				"reason",
				"previous prompt not completed",
			)
			return nil // blocked — wait for watcher signal or periodic scan
		}

		slog.Info("found queued prompt", "file", filepath.Base(pr.Path))

		// Process the prompt (includes moving to completed/ and committing)
		if err := p.processPrompt(ctx, pr); err != nil {
			p.handlePromptFailure(ctx, pr.Path, err)
			return nil // failed — wait for watcher signal or periodic scan
		}

		slog.Info("watching for queued prompts", "dir", p.queueDir)

		// Loop again to process next prompt
	}
}

// shouldSkipPrompt checks if a prompt should be skipped due to validation failure.
// Returns true if the prompt should be skipped, false if it's ready to process.
// Handles both previously-failed prompts (silent skip) and new validation failures (logged).
func (p *processor) shouldSkipPrompt(ctx context.Context, pr prompt.Prompt) bool {
	// Check if this prompt was previously skipped and hasn't been modified
	fileInfo, err := os.Stat(pr.Path)
	if err == nil {
		if lastSkipped, wasSkipped := p.skippedPrompts[pr.Path]; wasSkipped {
			if fileInfo.ModTime().Equal(lastSkipped) {
				// File hasn't changed since we last skipped it - skip silently
				slog.Debug(
					"skipping previously-failed prompt (unchanged)",
					"file",
					filepath.Base(pr.Path),
				)
				return true
			}
			// File was modified - remove from skipped list and re-validate
			delete(p.skippedPrompts, pr.Path)
		}
	}

	// Validate prompt before execution
	if err := pr.ValidateForExecution(ctx); err != nil {
		slog.Warn("skipping prompt", "file", filepath.Base(pr.Path), "reason", err.Error())
		// Record this prompt as skipped so we don't spam logs on next cycle
		if fileInfo != nil {
			p.skippedPrompts[pr.Path] = fileInfo.ModTime()
		}
		return true
	}

	return false
}

// autoSetQueuedStatus sets status to "queued" for any non-terminal status.
// This makes the folder location the source of truth - if a file is in queue/, it should be queued.
func (p *processor) autoSetQueuedStatus(ctx context.Context, pr *prompt.Prompt) error {
	switch pr.Status {
	case prompt.ApprovedPromptStatus,
		prompt.ExecutingPromptStatus,
		prompt.CompletedPromptStatus,
		prompt.FailedPromptStatus,
		prompt.PendingVerificationPromptStatus:
		// Already in a valid processing state — don't override
		return nil
	}
	// Any other status (empty, "created", "draft", etc.) → auto-set to approved
	baseName := filepath.Base(pr.Path)
	previousStatus := pr.Status
	slog.Info(
		"auto-setting status to approved",
		"file",
		baseName,
		"previousStatus",
		previousStatus,
	)
	if err := p.promptManager.SetStatus(ctx, pr.Path, string(prompt.ApprovedPromptStatus)); err != nil {
		return errors.Wrap(ctx, err, "set status to approved")
	}
	// Update local status so ValidateForExecution passes
	pr.Status = prompt.ApprovedPromptStatus
	return nil
}

// handlePromptFailure marks a prompt as failed after execution error.
func (p *processor) handlePromptFailure(ctx context.Context, path string, err error) {
	slog.Error("prompt failed", "file", filepath.Base(path), "error", err)
	// Mark as failed — file may have been moved to completed/ before the error.
	if pf, loadErr := p.promptManager.Load(ctx, path); loadErr == nil {
		pf.MarkFailed()
		if saveErr := pf.Save(ctx); saveErr != nil {
			slog.Error("failed to set failed status", "error", saveErr)
		}
	}
}

// checkPromptedSpecs scans all specs and calls CheckAndComplete for any in "prompted" status.
// This catches specs that were stuck in prompted state across daemon restarts.
func (p *processor) checkPromptedSpecs(ctx context.Context) error {
	specs, err := p.specLister.List(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "list specs")
	}

	for _, sf := range specs {
		if sf.Frontmatter.Status != string(spec.StatusPrompted) {
			continue
		}
		slog.Info("startup: checking prompted spec", "spec", sf.Name)
		if err := p.autoCompleter.CheckAndComplete(ctx, sf.Name); err != nil {
			return errors.Wrap(ctx, err, "check and complete spec")
		}
	}

	return nil
}

// processPrompt executes a single prompt and commits the result.
func (p *processor) processPrompt(ctx context.Context, pr prompt.Prompt) error {
	// Sync with remote before execution
	slog.Info("syncing with remote default branch")
	if err := p.brancher.Fetch(ctx); err != nil {
		return errors.Wrap(ctx, err, "git fetch origin")
	}
	if err := p.brancher.MergeOriginDefault(ctx); err != nil {
		return errors.Wrap(ctx, err, "git merge origin default branch")
	}

	// Load prompt file once
	pf, err := p.promptManager.Load(ctx, pr.Path)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}
	// Check if empty
	content, err := pf.Content()
	if err != nil {
		return p.handleEmptyPrompt(ctx, pr.Path, err)
	}

	// Prepare prompt for execution
	baseName, containerName, title, err := preparePromptForExecution(
		ctx,
		pf,
		pr.Path,
		p.versionGetter.Get(),
		p.projectName,
	)
	if err != nil {
		return err
	}
	// Append completion report suffix to make output machine-parseable
	content = content + report.Suffix()
	// Append changelog instructions when the project has a CHANGELOG.md
	if p.releaser.HasChangelog(ctx) {
		content = content + report.ChangelogSuffix()
	}
	// Inject project-level validation command (overrides prompt-level <verification>)
	if p.validationCommand != "" {
		content = content + report.ValidationSuffix(p.validationCommand)
	}

	slog.Info("executing prompt", "title", title)

	// Derive log file path before setupWorkflow, which may os.Chdir to clone dir.
	logFile, err := filepath.Abs(filepath.Join(p.logDir, baseName+".log"))
	if err != nil {
		return errors.Wrap(ctx, err, "resolve log file path")
	}

	// Setup workflow (branch or clone) before execution
	workflowState, err := p.setupWorkflow(ctx, baseName, pf)
	if err != nil {
		return err
	}

	// Ensure clone cleanup on error (success path cleanup is in handleCloneWorkflow)
	if p.pr && workflowState.clonePath != "" {
		defer p.cleanupCloneOnError(ctx, workflowState)
	}

	// Execute via executor
	if err := p.executor.Execute(ctx, content, logFile, containerName); err != nil {
		slog.Info("docker container exited with error", "error", err)
		return errors.Wrap(ctx, err, "execute prompt")
	}

	slog.Info("docker container exited", "exitCode", 0)

	return p.handlePostExecution(ctx, pf, pr.Path, title, logFile, workflowState)
}

// workflowState holds state needed for workflow cleanup and completion.
type workflowState struct {
	branchName  string
	clonePath   string
	originalDir string
	cleanedUp   bool
}

// cleanupCloneOnError restores the original directory and removes the clone
// when processPrompt exits with an error (success path is handled by handleCloneWorkflow).
func (p *processor) cleanupCloneOnError(ctx context.Context, state *workflowState) {
	if state.cleanedUp {
		return // Already cleaned up by handleCloneWorkflow
	}
	if state.originalDir != "" {
		if err := os.Chdir(state.originalDir); err != nil {
			slog.Warn("failed to chdir back to original directory on error", "error", err)
		}
	}
	if err := p.cloner.Remove(ctx, state.clonePath); err != nil {
		slog.Warn("failed to remove clone on error", "path", state.clonePath, "error", err)
	}
}

// handlePostExecution handles validation, moving to completed, and workflow completion.
func (p *processor) handlePostExecution(
	ctx context.Context,
	pf *prompt.PromptFile,
	promptPath string,
	title string,
	logFile string,
	state *workflowState,
) error {
	// Validate completion report from log
	summary, err := validateCompletionReport(ctx, logFile)
	if err != nil {
		return err
	}

	// Store summary in frontmatter before moving to completed
	if summary != "" {
		pf.SetSummary(summary)
		if err := pf.Save(ctx); err != nil {
			return errors.Wrap(ctx, err, "save summary")
		}
	}

	// Use a non-cancellable context for git ops so they aren't interrupted by shutdown.
	gitCtx := context.WithoutCancel(ctx)

	// Verification gate: pause before git operations if enabled
	if p.verificationGate {
		return p.enterPendingVerification(ctx, pf, promptPath)
	}

	completedPath := filepath.Join(p.completedDir, filepath.Base(promptPath))

	if p.pr {
		// PR workflow: commit only code changes in clone, then manage prompt in original repo.
		return p.handleCloneWorkflow(gitCtx, ctx, pf, title, promptPath, completedPath, state)
	}

	// Direct workflow: move prompt to completed and commit in the same repo.
	if err := p.moveToCompletedAndCommit(ctx, gitCtx, pf, promptPath, completedPath); err != nil {
		return err
	}

	return p.handleDirectWorkflow(gitCtx, ctx, title)
}

// moveToCompletedAndCommit moves the prompt to completed/, triggers spec auto-complete, and commits the file.
func (p *processor) moveToCompletedAndCommit(
	ctx context.Context,
	gitCtx context.Context,
	pf *prompt.PromptFile,
	promptPath string,
	completedPath string,
) error {
	// Move to completed/ before commit so it's included in the release
	if err := p.promptManager.MoveToCompleted(ctx, promptPath); err != nil {
		return errors.Wrap(ctx, err, "move to completed")
	}

	slog.Info("moved to completed", "file", filepath.Base(promptPath))

	// Auto-complete linked specs if all their prompts are now done
	for _, specID := range pf.Specs() {
		if err := p.autoCompleter.CheckAndComplete(ctx, specID); err != nil {
			slog.Warn("spec auto-complete failed", "spec", specID, "error", err)
		}
	}

	// Commit the completed file separately (YOLO may have already committed code changes)
	if err := p.releaser.CommitCompletedFile(gitCtx, completedPath); err != nil {
		return errors.Wrap(ctx, err, "commit completed file")
	}
	return nil
}

// setupWorkflow sets up the appropriate workflow before execution.
func (p *processor) setupWorkflow(
	ctx context.Context,
	baseName string,
	pf *prompt.PromptFile,
) (*workflowState, error) {
	state := &workflowState{}
	if p.pr {
		return p.setupCloneWorkflowState(ctx, baseName, pf, state)
	}
	return state, nil
}

// setupCloneWorkflowState configures state for the clone workflow.
func (p *processor) setupCloneWorkflowState(
	ctx context.Context,
	baseName string,
	pf *prompt.PromptFile,
	state *workflowState,
) (*workflowState, error) {
	if branch := pf.Branch(); branch != "" {
		state.branchName = branch
	} else {
		state.branchName = "dark-factory/" + baseName
	}
	state.clonePath = filepath.Join(os.TempDir(), "dark-factory", p.projectName+"-"+baseName)
	var err error
	state.originalDir, err = p.setupCloneForExecution(ctx, state.clonePath, state.branchName)
	if err != nil {
		return nil, err
	}
	return state, nil
}

// hasPendingVerification returns true if any prompt in queueDir has pending_verification status.
func (p *processor) hasPendingVerification(ctx context.Context) bool {
	entries, err := os.ReadDir(p.queueDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		pf, err := p.promptManager.Load(ctx, filepath.Join(p.queueDir, entry.Name()))
		if err != nil {
			continue
		}
		if pf.Frontmatter.Status == string(prompt.PendingVerificationPromptStatus) {
			return true
		}
	}
	return false
}

// enterPendingVerification transitions a prompt to pending_verification state and logs the verification hint.
func (p *processor) enterPendingVerification(
	ctx context.Context,
	pf *prompt.PromptFile,
	promptPath string,
) error {
	pf.MarkPendingVerification()
	if err := pf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save pending verification status")
	}
	hint := pf.VerificationSection()
	if hint != "" {
		slog.Info(
			"prompt pending verification — run the following checks, then: dark-factory prompt verify <file>",
			"file",
			filepath.Base(promptPath),
			"verification",
			hint,
		)
	} else {
		slog.Info("prompt pending verification",
			"file", filepath.Base(promptPath),
			"hint", "run: dark-factory prompt verify <file> when ready",
		)
	}
	return nil
}

// handleEmptyPrompt handles empty prompts by moving them to completed without execution.
func (p *processor) handleEmptyPrompt(
	ctx context.Context,
	promptPath string,
	contentErr error,
) error {
	// If prompt is empty, move to completed and skip execution
	if stderrors.Is(contentErr, prompt.ErrEmptyPrompt) {
		slog.Debug(
			"skipping empty prompt",
			"file",
			filepath.Base(promptPath),
			"reason",
			"file may still be in progress",
		)
		// Move empty prompts to completed/ (but don't commit)
		if err := p.promptManager.MoveToCompleted(ctx, promptPath); err != nil {
			return errors.Wrap(ctx, err, "move empty prompt to completed")
		}
		return nil
	}
	return errors.Wrap(ctx, contentErr, "get prompt content")
}

// postMergeActions performs post-merge actions: switch to default branch, pull, and optionally release.
func (p *processor) postMergeActions(
	gitCtx context.Context,
	ctx context.Context,
	title string,
) error {
	// PR merged successfully — switch to default branch
	defaultBranch, err := p.brancher.DefaultBranch(gitCtx)
	if err != nil {
		return errors.Wrap(ctx, err, "get default branch")
	}

	if err := p.brancher.Switch(gitCtx, defaultBranch); err != nil {
		return errors.Wrap(ctx, err, "switch to default branch")
	}

	if err := p.brancher.Pull(gitCtx); err != nil {
		return errors.Wrap(ctx, err, "pull default branch")
	}

	slog.Info("merged PR and updated default branch", "branch", defaultBranch)

	// If autoRelease enabled and has changelog, create release
	if p.autoRelease && p.releaser.HasChangelog(gitCtx) {
		if err := p.handleDirectWorkflow(gitCtx, ctx, title); err != nil {
			return errors.Wrap(ctx, err, "auto-release after merge")
		}
	}

	return nil
}

// handleCloneWorkflow handles the clone-based workflow: commit code in clone, push, create PR,
// then manage prompt lifecycle in the original repo.
func (p *processor) handleCloneWorkflow(
	gitCtx context.Context,
	ctx context.Context,
	pf *prompt.PromptFile,
	title string,
	promptPath string,
	completedPath string,
	state *workflowState,
) error {
	branchName := state.branchName
	clonePath := state.clonePath
	originalDir := state.originalDir

	// Commit only code changes in the clone (no prompt files)
	if err := p.releaser.CommitOnly(gitCtx, title); err != nil {
		return errors.Wrap(ctx, err, "commit changes")
	}

	// Push branch
	if err := p.brancher.Push(gitCtx, branchName); err != nil {
		return errors.Wrap(ctx, err, "push branch")
	}

	// Create PR
	prURL, err := p.prCreator.Create(gitCtx, title, "Automated by dark-factory")
	if err != nil {
		return errors.Wrap(ctx, err, "create pull request")
	}
	slog.Info("created PR", "url", prURL)

	// Switch back to original directory before managing prompt
	if err := os.Chdir(originalDir); err != nil {
		return errors.Wrap(ctx, err, "chdir back to original directory")
	}

	// Remove clone (best-effort cleanup)
	if err := p.cloner.Remove(gitCtx, clonePath); err != nil {
		slog.Warn("failed to remove clone", "path", clonePath, "error", err)
	}
	state.cleanedUp = true

	// --- From here, we're back in the original repo ---

	// Handle auto-merge if enabled
	if p.autoMerge {
		if err := p.prMerger.WaitAndMerge(gitCtx, prURL); err != nil {
			return errors.Wrap(ctx, err, "wait and merge PR")
		}
		// After merge, move prompt to completed in original repo
		if err := p.moveToCompletedAndCommit(ctx, gitCtx, pf, promptPath, completedPath); err != nil {
			return err
		}
		return p.postMergeActions(gitCtx, ctx, title)
	}

	// autoReview mode: keep prompt in queueDir with in_review status + PR URL
	if p.autoReview {
		p.savePRURLToFrontmatter(gitCtx, promptPath, prURL)
		if err := p.promptManager.SetStatus(ctx, promptPath, string(prompt.InReviewPromptStatus)); err != nil {
			return errors.Wrap(ctx, err, "set in_review status")
		}
		slog.Info("PR created, waiting for review", "url", prURL)
		return nil
	}

	// Default: move prompt to completed in original repo with PR URL
	if err := p.moveToCompletedAndCommit(ctx, gitCtx, pf, promptPath, completedPath); err != nil {
		return err
	}
	p.savePRURLToFrontmatter(gitCtx, completedPath, prURL)

	return nil
}

// setupCloneForExecution creates a clone, switches to it, and sets up cleanup.
// Returns the original directory path for later restoration.
func (p *processor) setupCloneForExecution(
	ctx context.Context,
	clonePath string,
	branchName string,
) (string, error) {
	originalDir, err := os.Getwd()
	if err != nil {
		return "", errors.Wrap(ctx, err, "get current directory")
	}

	if err := p.cloner.Clone(ctx, originalDir, clonePath, branchName); err != nil {
		return "", errors.Wrap(ctx, err, "clone repo")
	}

	// Switch to clone directory
	if err := os.Chdir(clonePath); err != nil {
		return "", errors.Wrap(ctx, err, "chdir to clone")
	}

	return originalDir, nil
}

// handleDirectWorkflow handles the direct commit workflow: commit, tag, push.
func (p *processor) handleDirectWorkflow(
	gitCtx context.Context,
	ctx context.Context,
	title string,
) error {
	// Without CHANGELOG: simple commit only (no tag, no push)
	if !p.releaser.HasChangelog(gitCtx) {
		if err := p.releaser.CommitOnly(gitCtx, title); err != nil {
			return errors.Wrap(ctx, err, "commit")
		}
		slog.Info("committed changes")
		return nil
	}

	// With CHANGELOG: rename ## Unreleased to version, bump version, tag, push
	bump := determineBump()
	nextVersion, err := p.releaser.GetNextVersion(gitCtx, bump)
	if err != nil {
		return errors.Wrap(ctx, err, "get next version")
	}

	if err := p.releaser.CommitAndRelease(gitCtx, bump); err != nil {
		return errors.Wrap(ctx, err, "commit and release")
	}

	slog.Info("committed and tagged", "version", nextVersion)

	return nil
}

// preparePromptForExecution sets up the prompt metadata and returns execution parameters.
func preparePromptForExecution(
	ctx context.Context,
	pf *prompt.PromptFile,
	promptPath string,
	version string,
	projectName string,
) (baseName string, containerName string, title string, err error) {
	baseName = strings.TrimSuffix(filepath.Base(promptPath), ".md")
	baseName = sanitizeContainerName(baseName)
	containerName = projectName + "-" + baseName

	pf.PrepareForExecution(containerName, version)
	if err := pf.Save(ctx); err != nil {
		return "", "", "", errors.Wrap(ctx, err, "save prompt metadata")
	}

	title = pf.Title()
	if title == "" {
		// Fallback to filename if no title found
		title = strings.TrimSuffix(filepath.Base(promptPath), ".md")
	}

	return baseName, containerName, title, nil
}

// validateCompletionReport parses and validates the completion report from the log file.
// Returns the summary and an error if the report indicates failure.
// Returns ("", nil) if no report found (backwards compatible) or parse error.
// Returns (summary, nil) if report indicates success.
func validateCompletionReport(ctx context.Context, logFile string) (string, error) {
	completionReport, err := report.ParseFromLog(ctx, logFile)
	if err != nil {
		slog.Debug("failed to parse completion report", "error", err)
		// Continue — don't fail the prompt just because report parsing failed
		return "", nil
	}
	if completionReport == nil {
		// No report found — backwards compatible
		return "", nil
	}

	slog.Info(
		"completion report",
		"status",
		completionReport.Status,
		"summary",
		completionReport.Summary,
	)

	// Validate consistency between status and verification results
	correctedStatus, overridden := completionReport.ValidateConsistency()
	if overridden {
		slog.Warn(
			"overriding self-reported status",
			"reported", completionReport.Status,
			"corrected", correctedStatus,
			"verificationCommand", completionReport.Verification.Command,
			"verificationExitCode", completionReport.Verification.ExitCode,
		)
		completionReport.Status = correctedStatus
	}

	if completionReport.Status != "success" {
		// Report says not success — treat as failure
		slog.Info("completion report indicates failure", "status", completionReport.Status)
		if len(completionReport.Blockers) > 0 {
			slog.Info("blockers reported", "blockers", completionReport.Blockers)
		}
		return "", errors.Errorf(ctx, "completion report status: %s", completionReport.Status)
	}

	return completionReport.Summary, nil
}

// savePRURLToFrontmatter saves the PR URL to the prompt frontmatter.
// This is best-effort and non-fatal — all failures are logged as warnings.
func (p *processor) savePRURLToFrontmatter(
	ctx context.Context,
	completedPath string,
	prURL string,
) {
	// Preserve existing pr-url for follow-up prompts
	if existingPF, err := p.promptManager.Load(ctx, completedPath); err == nil &&
		existingPF.PRURL() != "" {
		slog.Debug("pr-url already set, preserving existing value")
		return
	}
	if err := p.promptManager.SetPRURL(ctx, completedPath, prURL); err != nil {
		slog.Warn("failed to save PR URL to frontmatter", "error", err)
	}
}

// sanitizeContainerName ensures the name only contains Docker-safe characters [a-zA-Z0-9_-]
func sanitizeContainerName(name string) string {
	// Replace any character that is not alphanumeric, underscore, or hyphen with hyphen
	return sanitizeContainerNameRegexp.ReplaceAllString(name, "-")
}

// determineBump determines the version bump type by analyzing CHANGELOG.md content.
// Returns MinorBump if any ## Unreleased entry starts with "- feat:", PatchBump otherwise.
func determineBump() git.VersionBump {
	content, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		return git.PatchBump
	}

	unreleasedContent := extractUnreleasedSection(string(content))
	if unreleasedContent == "" {
		return git.PatchBump
	}

	for _, line := range strings.Split(unreleasedContent, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- feat:") {
			return git.MinorBump
		}
	}
	return git.PatchBump
}

// extractUnreleasedSection extracts content between ## Unreleased and the next ## section
func extractUnreleasedSection(content string) string {
	lines := strings.Split(content, "\n")
	inUnreleased := false
	var unreleasedLines []string

	for _, line := range lines {
		if strings.HasPrefix(line, "## Unreleased") {
			inUnreleased = true
			continue
		}
		if inUnreleased && strings.HasPrefix(line, "##") {
			// Hit next version section, stop
			break
		}
		if inUnreleased {
			unreleasedLines = append(unreleasedLines, line)
		}
	}

	return strings.Join(unreleasedLines, "\n")
}
