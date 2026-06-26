// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"os/exec"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/subproc"
)

//counterfeiter:generate -o ../../mocks/pr_creator.go --fake-name PRCreator . PRCreator

// PRCreator handles GitHub pull request creation.
type PRCreator interface {
	// Create creates a pull request on the given branch and returns the PR URL.
	Create(ctx context.Context, title string, body string, branch string) (string, error)
	// FindOpenPR returns the URL of an open PR for the given branch, or "" if none exists.
	FindOpenPR(ctx context.Context, branch string) (string, error)
}

// CommandOutputFn executes a command and returns its output.
// Kept for backward compatibility; new code should use the runner-based constructors.
type CommandOutputFn func(cmd *exec.Cmd) ([]byte, error)

// prCreator implements PRCreator via subproc.Runner.
type prCreator struct {
	ghToken string
	runner  subproc.Runner
}

// NewPRCreator creates a new PRCreator.
func NewPRCreator(ghToken string) PRCreator {
	return &prCreator{
		ghToken: ghToken,
		runner:  subproc.NewRunner(),
	}
}

// NewPRCreatorWithCommandOutput creates a PRCreator. The CommandOutputFn is accepted for
// backward compatibility but the spawn goes through the runner.
func NewPRCreatorWithCommandOutput(ghToken string, _ CommandOutputFn) PRCreator {
	return &prCreator{
		ghToken: ghToken,
		runner:  subproc.NewRunner(),
	}
}

// NewPRCreatorWithRunner creates a PRCreator with an injected runner (for tests).
func NewPRCreatorWithRunner(ghToken string, r subproc.Runner) PRCreator {
	return &prCreator{ghToken: ghToken, runner: r}
}

// FindOpenPR returns the URL of an open PR for the given branch, or "" if none exists.
func (p *prCreator) FindOpenPR(ctx context.Context, branch string) (string, error) {
	var extraEnv []string
	if p.ghToken != "" {
		extraEnv = []string{"GH_TOKEN=" + p.ghToken}
	}
	output, err := p.runner.RunWithWarnAndTimeoutEnv(
		ctx,
		"gh pr list",
		"",
		extraEnv,
		"gh", "pr", "list",
		"--head", branch,
		"--state", "open",
		"--json", "url",
		"--jq", ".[0].url",
	)
	if err != nil {
		return "", errors.Wrap(ctx, err, "list open PRs")
	}
	return strings.TrimSpace(string(output)), nil
}

// Create creates a pull request and returns the PR URL.
func (p *prCreator) Create(
	ctx context.Context,
	title string,
	body string,
	branch string,
) (string, error) {
	if err := ValidatePRTitle(ctx, title); err != nil {
		return "", errors.Wrap(ctx, err, "validate PR title")
	}
	var extraEnv []string
	if p.ghToken != "" {
		extraEnv = []string{"GH_TOKEN=" + p.ghToken}
	}
	output, err := p.runner.RunWithWarnAndTimeoutEnv(
		ctx,
		"gh pr create",
		"",
		extraEnv,
		"gh", "pr", "create",
		"--head", branch,
		"--title", title,
		"--body", body,
	)
	if err != nil {
		return "", errors.Errorf(ctx, "create pull request: %v: %s", err, stderrFromErr(err))
	}
	return strings.TrimSpace(string(output)), nil
}
