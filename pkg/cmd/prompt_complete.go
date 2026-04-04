// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/prompt-complete-command.go --fake-name PromptCompleteCommand . PromptCompleteCommand

// PromptCompleteCommand executes the prompt complete subcommand.
type PromptCompleteCommand interface {
	Run(ctx context.Context, args []string) error
}

// promptCompleteCommand implements PromptCompleteCommand.
type promptCompleteCommand struct {
	queueDir              string
	completedDir          string
	promptManager         prompt.Manager
	releaser              git.Releaser
	pr                    bool
	brancher              git.Brancher
	prCreator             git.PRCreator
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// NewPromptCompleteCommand creates a new PromptCompleteCommand.
func NewPromptCompleteCommand(
	queueDir string,
	completedDir string,
	promptManager prompt.Manager,
	releaser git.Releaser,
	pr bool,
	brancher git.Brancher,
	prCreator git.PRCreator,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) PromptCompleteCommand {
	return &promptCompleteCommand{
		queueDir:              queueDir,
		completedDir:          completedDir,
		promptManager:         promptManager,
		releaser:              releaser,
		pr:                    pr,
		brancher:              brancher,
		prCreator:             prCreator,
		currentDateTimeGetter: currentDateTimeGetter,
	}
}

// Run executes the prompt complete command.
func (c *promptCompleteCommand) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.Errorf(ctx, "usage: dark-factory prompt complete <file>")
	}

	path, err := FindPromptFile(ctx, c.queueDir, args[0])
	if err != nil {
		return err
	}

	pf, err := prompt.Load(ctx, path, c.currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	switch prompt.PromptStatus(pf.Frontmatter.Status) {
	case prompt.PendingVerificationPromptStatus,
		prompt.FailedPromptStatus,
		prompt.PermanentlyFailedPromptStatus,
		prompt.InReviewPromptStatus,
		prompt.ExecutingPromptStatus:
		// acceptable states — proceed
	default:
		return errors.Errorf(
			ctx,
			"prompt cannot be completed (current status: %s)",
			pf.Frontmatter.Status,
		)
	}

	title := pf.Title()
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), ".md")
	}

	completedPath := filepath.Join(c.completedDir, filepath.Base(path))

	gitCtx := context.WithoutCancel(ctx)

	if err := c.promptManager.MoveToCompleted(ctx, path); err != nil {
		return errors.Wrap(ctx, err, "move to completed")
	}

	if err := c.releaser.CommitCompletedFile(gitCtx, completedPath); err != nil {
		return errors.Wrap(ctx, err, "commit completed file")
	}

	if !c.pr {
		if err := c.completeDirectWorkflow(gitCtx, ctx, title); err != nil {
			return err
		}
	} else {
		if err := c.completePRWorkflow(gitCtx, ctx, pf, title, completedPath); err != nil {
			return err
		}
	}

	fmt.Printf("completed: %s\n", filepath.Base(path))
	return nil
}

// completeDirectWorkflow handles commit/release for the direct workflow.
func (c *promptCompleteCommand) completeDirectWorkflow(
	gitCtx, ctx context.Context,
	title string,
) error {
	if !c.releaser.HasChangelog(gitCtx) {
		if err := c.releaser.CommitOnly(gitCtx, title); err != nil {
			return errors.Wrap(ctx, err, "commit")
		}
		slog.Info("committed changes")
		return nil
	}

	bump := git.DetermineBumpFromChangelog(ctx, ".")
	if err := c.releaser.CommitAndRelease(gitCtx, bump); err != nil {
		return errors.Wrap(ctx, err, "commit and release")
	}
	slog.Info("committed and released")
	return nil
}

// completePRWorkflow handles commit/push/PR for PR or worktree workflow.
func (c *promptCompleteCommand) completePRWorkflow(
	gitCtx context.Context,
	ctx context.Context,
	pf *prompt.PromptFile,
	title string,
	completedPath string,
) error {
	branch := pf.Branch()
	if branch == "" {
		branch = "dark-factory/" + strings.TrimSuffix(filepath.Base(completedPath), ".md")
	}

	if err := c.releaser.CommitOnly(gitCtx, title); err != nil {
		return errors.Wrap(ctx, err, "commit changes")
	}

	if err := c.brancher.Push(gitCtx, branch); err != nil {
		return errors.Wrap(ctx, err, "push branch")
	}

	prURL, err := c.prCreator.Create(gitCtx, title, "Automated by dark-factory")
	if err != nil {
		return errors.Wrap(ctx, err, "create pull request")
	}
	slog.Info("created PR", "url", prURL)
	return nil
}
