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

	"github.com/bborbe/dark-factory/pkg/config"
	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/report"
	"github.com/bborbe/dark-factory/pkg/version"
)

// Processor processes queued prompts.
//
//counterfeiter:generate -o ../../mocks/processor.go --fake-name Processor . Processor
type Processor interface {
	Process(ctx context.Context) error
}

// processor implements Processor.
type processor struct {
	queueDir      string
	completedDir  string
	logDir        string
	projectName   string
	executor      executor.Executor
	promptManager prompt.Manager
	releaser      git.Releaser
	versionGetter version.Getter
	ready         <-chan struct{}
	workflow      config.Workflow
	brancher      git.Brancher
	prCreator     git.PRCreator
	worktree      git.Worktree
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
	workflow config.Workflow,
	brancher git.Brancher,
	prCreator git.PRCreator,
	worktree git.Worktree,
) Processor {
	return &processor{
		queueDir:      queueDir,
		completedDir:  completedDir,
		logDir:        logDir,
		projectName:   projectName,
		executor:      exec,
		promptManager: promptManager,
		releaser:      releaser,
		versionGetter: versionGetter,
		ready:         ready,
		workflow:      workflow,
		brancher:      brancher,
		prCreator:     prCreator,
		worktree:      worktree,
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

	// Process any existing queued prompts first
	if err := p.processExistingQueued(ctx); err != nil {
		return errors.Wrap(ctx, err, "process existing queued prompts")
	}

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

// processExistingQueued scans for and processes any existing queued prompts.
func (p *processor) processExistingQueued(ctx context.Context) error {
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

		// Validate prompt before execution
		if err := pr.ValidateForExecution(ctx); err != nil {
			slog.Debug("skipping prompt", "file", filepath.Base(pr.Path), "reason", err.Error())
			continue
		}

		// Check ordering - all previous prompts must be completed
		if !p.promptManager.AllPreviousCompleted(ctx, pr.Number()) {
			slog.Debug(
				"skipping prompt",
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
			slog.Error("prompt failed", "file", filepath.Base(pr.Path), "error", err)
			// Mark as failed — file may have been moved to completed/ before the error.
			if pf, loadErr := p.promptManager.Load(ctx, pr.Path); loadErr == nil {
				pf.MarkFailed()
				if saveErr := pf.Save(); saveErr != nil {
					slog.Error("failed to set failed status", "error", saveErr)
				}
			}
			return nil // failed — wait for watcher signal or periodic scan
		}

		slog.Info("watching for queued prompts", "dir", p.queueDir)

		// Loop again to process next prompt
	}
}

// processPrompt executes a single prompt and commits the result.
func (p *processor) processPrompt(ctx context.Context, pr prompt.Prompt) error {
	// Sync with remote before execution
	slog.Info("syncing with origin/master")
	if err := p.brancher.Fetch(ctx); err != nil {
		return errors.Wrap(ctx, err, "git fetch origin")
	}
	if err := p.brancher.MergeOriginMaster(ctx); err != nil {
		return errors.Wrap(ctx, err, "git merge origin/master")
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

	slog.Info("executing prompt", "title", title)

	// Setup workflow (branch or worktree) before execution
	workflowState, err := p.setupWorkflow(ctx, baseName)
	if err != nil {
		return err
	}

	// Derive log file path: {logDir}/{basename}.log
	logFile := filepath.Join(p.logDir, baseName+".log")

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
	branchName     string
	originalBranch string
	worktreePath   string
	originalDir    string
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
		if err := pf.Save(); err != nil {
			return errors.Wrap(ctx, err, "save summary")
		}
	}

	// Move to completed/ before commit so it's included in the release
	if err := p.promptManager.MoveToCompleted(ctx, promptPath); err != nil {
		return errors.Wrap(ctx, err, "move to completed")
	}

	slog.Info("moved to completed", "file", filepath.Base(promptPath))

	// Use a non-cancellable context for git ops so they aren't interrupted by shutdown.
	gitCtx := context.WithoutCancel(ctx)

	// Commit the completed file separately (YOLO may have already committed code changes)
	completedPath := filepath.Join(p.completedDir, filepath.Base(promptPath))
	if err := p.releaser.CommitCompletedFile(gitCtx, completedPath); err != nil {
		return errors.Wrap(ctx, err, "commit completed file")
	}

	if p.workflow == config.WorkflowPR {
		return p.handlePRWorkflow(gitCtx, ctx, title, summary, state.branchName, state.originalBranch)
	}

	if p.workflow == config.WorkflowWorktree {
		return p.handleWorktreeWorkflow(
			gitCtx,
			ctx,
			title,
			summary,
			state.branchName,
			state.worktreePath,
			state.originalDir,
		)
	}

	return p.handleDirectWorkflow(gitCtx, ctx, title, summary)
}

// setupWorkflow sets up the appropriate workflow (PR or worktree) before execution.
func (p *processor) setupWorkflow(ctx context.Context, baseName string) (*workflowState, error) {
	state := &workflowState{}

	if p.workflow == config.WorkflowPR {
		var err error
		state.originalBranch, err = p.brancher.CurrentBranch(ctx)
		if err != nil {
			return nil, errors.Wrap(ctx, err, "get current branch")
		}
		state.branchName = "dark-factory/" + baseName
		if err := p.brancher.CreateAndSwitch(ctx, state.branchName); err != nil {
			return nil, errors.Wrap(ctx, err, "create feature branch")
		}
		return state, nil
	}

	if p.workflow == config.WorkflowWorktree {
		state.branchName = "dark-factory/" + baseName
		state.worktreePath = filepath.Join("..", p.projectName+"-"+baseName)
		var err error
		state.originalDir, err = p.setupWorktreeForExecution(
			ctx,
			state.worktreePath,
			state.branchName,
		)
		if err != nil {
			return nil, err
		}
		return state, nil
	}

	return state, nil
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

// handlePRWorkflow handles the PR-based workflow: commit, push, create PR, switch back.
func (p *processor) handlePRWorkflow(
	gitCtx context.Context,
	ctx context.Context,
	title string,
	summary string,
	branchName string,
	originalBranch string,
) error {
	// Commit changes
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

	// Switch back to original branch for next prompt
	if err := p.brancher.Switch(gitCtx, originalBranch); err != nil {
		return errors.Wrap(ctx, err, "switch back to "+originalBranch)
	}

	return nil
}

// handleWorktreeWorkflow handles the worktree-based workflow: commit, push, create PR, remove worktree.
func (p *processor) handleWorktreeWorkflow(
	gitCtx context.Context,
	ctx context.Context,
	title string,
	summary string,
	branchName string,
	worktreePath string,
	originalDir string,
) error {
	// Commit changes (we're already in the worktree directory)
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

	// Switch back to original directory
	if err := os.Chdir(originalDir); err != nil {
		return errors.Wrap(ctx, err, "chdir back to original directory")
	}

	// Remove worktree (best-effort cleanup)
	if err := p.worktree.Remove(gitCtx, worktreePath); err != nil {
		slog.Warn("failed to remove worktree", "path", worktreePath, "error", err)
		// Non-fatal — worktree cleanup is best-effort
	}

	return nil
}

// setupWorktreeForExecution creates a worktree, switches to it, and sets up cleanup.
// Returns the original directory path for later restoration.
func (p *processor) setupWorktreeForExecution(
	ctx context.Context,
	worktreePath string,
	branchName string,
) (string, error) {
	originalDir, err := os.Getwd()
	if err != nil {
		return "", errors.Wrap(ctx, err, "get current directory")
	}

	if err := p.worktree.Add(ctx, worktreePath, branchName); err != nil {
		return "", errors.Wrap(ctx, err, "add worktree")
	}

	// Ensure cleanup happens even if execution fails
	defer func() {
		if originalDir != "" {
			if err := os.Chdir(originalDir); err != nil {
				slog.Warn("failed to chdir back to original directory", "error", err)
			}
		}
		if worktreePath != "" {
			if err := p.worktree.Remove(ctx, worktreePath); err != nil {
				slog.Warn(
					"failed to remove worktree on defer",
					"path",
					worktreePath,
					"error",
					err,
				)
			}
		}
	}()

	// Switch to worktree directory
	if err := os.Chdir(worktreePath); err != nil {
		return "", errors.Wrap(ctx, err, "chdir to worktree")
	}

	return originalDir, nil
}

// handleDirectWorkflow handles the direct commit workflow: commit, tag, push.
func (p *processor) handleDirectWorkflow(
	gitCtx context.Context,
	ctx context.Context,
	title string,
	summary string,
) error {
	// Without CHANGELOG: simple commit only (no tag, no push)
	if !p.releaser.HasChangelog(gitCtx) {
		if err := p.releaser.CommitOnly(gitCtx, title); err != nil {
			return errors.Wrap(ctx, err, "commit")
		}
		slog.Info("committed changes")
		return nil
	}

	// With CHANGELOG: update changelog, bump version, tag, push
	// Use summary for changelog entry if available, otherwise fall back to title
	changelogEntry := summary
	if changelogEntry == "" {
		changelogEntry = title
	}

	bump := determineBump(title)
	nextVersion, err := p.releaser.GetNextVersion(gitCtx, bump)
	if err != nil {
		return errors.Wrap(ctx, err, "get next version")
	}

	if err := p.releaser.CommitAndRelease(gitCtx, changelogEntry, bump); err != nil {
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
	if err := pf.Save(); err != nil {
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
	completionReport, err := report.ParseFromLog(logFile)
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

// sanitizeContainerName ensures the name only contains Docker-safe characters [a-zA-Z0-9_-]
func sanitizeContainerName(name string) string {
	// Replace any character that is not alphanumeric, underscore, or hyphen with hyphen
	re := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	return re.ReplaceAllString(name, "-")
}

// determineBump determines the version bump type based on the title.
// Returns MinorBump for new features, PatchBump for everything else.
func determineBump(title string) git.VersionBump {
	lower := strings.ToLower(title)
	for _, kw := range []string{"add", "implement", "new", "support", "feature"} {
		if strings.Contains(lower, kw) {
			return git.MinorBump
		}
	}
	return git.PatchBump
}
