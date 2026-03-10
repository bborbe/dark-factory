// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/prompt"
)

//counterfeiter:generate -o ../../mocks/prompt-verify-command.go --fake-name PromptVerifyCommand . PromptVerifyCommand

// PromptVerifyCommand executes the prompt verify subcommand.
type PromptVerifyCommand interface {
	Run(ctx context.Context, args []string) error
}

// promptVerifyCommand implements PromptVerifyCommand.
type promptVerifyCommand struct {
	queueDir              string
	completedDir          string
	promptManager         prompt.Manager
	releaser              git.Releaser
	pr                    bool
	brancher              git.Brancher
	prCreator             git.PRCreator
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// NewPromptVerifyCommand creates a new PromptVerifyCommand.
func NewPromptVerifyCommand(
	queueDir string,
	completedDir string,
	promptManager prompt.Manager,
	releaser git.Releaser,
	pr bool,
	brancher git.Brancher,
	prCreator git.PRCreator,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) PromptVerifyCommand {
	return &promptVerifyCommand{
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

// Run executes the prompt verify command.
func (c *promptVerifyCommand) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.Errorf(ctx, "usage: dark-factory prompt verify <file>")
	}

	path, err := FindPromptFile(ctx, c.queueDir, args[0])
	if err != nil {
		return err
	}

	pf, err := prompt.Load(ctx, path, c.currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	if pf.Frontmatter.Status != string(prompt.PendingVerificationPromptStatus) {
		return errors.Errorf(
			ctx,
			"prompt is not in pending verification state (current: %s)",
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

	fmt.Printf("verified: %s\n", filepath.Base(path))
	return nil
}

// completeDirectWorkflow handles commit/release for the direct workflow.
func (c *promptVerifyCommand) completeDirectWorkflow(
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

	bump := determineBumpFromChangelog()
	if err := c.releaser.CommitAndRelease(gitCtx, bump); err != nil {
		return errors.Wrap(ctx, err, "commit and release")
	}
	slog.Info("committed and released")
	return nil
}

// completePRWorkflow handles commit/push/PR for PR or worktree workflow.
func (c *promptVerifyCommand) completePRWorkflow(
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

// determineBumpFromChangelog reads CHANGELOG.md and returns MinorBump if any feat: entry
// is in the Unreleased section, PatchBump otherwise.
func determineBumpFromChangelog() git.VersionBump {
	content, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		return git.PatchBump
	}

	lines := strings.Split(string(content), "\n")
	inUnreleased := false
	for _, line := range lines {
		if strings.HasPrefix(line, "## Unreleased") {
			inUnreleased = true
			continue
		}
		if inUnreleased && strings.HasPrefix(line, "##") {
			break
		}
		if inUnreleased && strings.HasPrefix(strings.TrimSpace(line), "- feat:") {
			return git.MinorBump
		}
	}
	return git.PatchBump
}
