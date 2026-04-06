// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/bborbe/errors"
)

//counterfeiter:generate -o ../../mocks/pr_creator.go --fake-name PRCreator . PRCreator

// PRCreator handles GitHub pull request creation.
type PRCreator interface {
	Create(ctx context.Context, title string, body string) (string, error)
	// FindOpenPR returns the URL of an open PR for the given branch, or "" if none exists.
	FindOpenPR(ctx context.Context, branch string) (string, error)
}

// CommandOutputFn executes a command and returns its output.
type CommandOutputFn func(cmd *exec.Cmd) ([]byte, error)

// defaultCommandOutput runs the command and returns its output.
func defaultCommandOutput(cmd *exec.Cmd) ([]byte, error) {
	return cmd.Output()
}

// prCreator implements PRCreator.
type prCreator struct {
	ghToken         string
	commandOutputFn CommandOutputFn
}

// NewPRCreator creates a new PRCreator.
func NewPRCreator(ghToken string) PRCreator {
	return &prCreator{
		ghToken:         ghToken,
		commandOutputFn: defaultCommandOutput,
	}
}

// NewPRCreatorWithCommandOutput creates a new PRCreator with a custom command output function.
func NewPRCreatorWithCommandOutput(ghToken string, fn CommandOutputFn) PRCreator {
	return &prCreator{
		ghToken:         ghToken,
		commandOutputFn: fn,
	}
}

// FindOpenPR returns the URL of an open PR for the given branch, or "" if none exists.
func (p *prCreator) FindOpenPR(ctx context.Context, branch string) (string, error) {
	// #nosec G204 -- branch name comes from validated frontmatter
	cmd := exec.CommandContext(
		ctx,
		"gh", "pr", "list",
		"--head", branch,
		"--state", "open",
		"--json", "url",
		"--jq", ".[0].url",
	)
	if p.ghToken != "" {
		cmd.Env = append(os.Environ(), "GH_TOKEN="+p.ghToken)
	}
	output, err := p.commandOutputFn(cmd)
	if err != nil {
		return "", errors.Wrap(ctx, err, "list open PRs")
	}
	return strings.TrimSpace(string(output)), nil
}

// Create creates a pull request and returns the PR URL.
func (p *prCreator) Create(ctx context.Context, title string, body string) (string, error) {
	if err := ValidatePRTitle(ctx, title); err != nil {
		return "", errors.Wrap(ctx, err, "validate PR title")
	}
	// #nosec G204 -- title is from prompt frontmatter, body is static text
	cmd := exec.CommandContext(ctx, "gh", "pr", "create", "--title", title, "--body", body)
	if p.ghToken != "" {
		cmd.Env = append(os.Environ(), "GH_TOKEN="+p.ghToken)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	output, err := p.commandOutputFn(cmd)
	if err != nil {
		return "", errors.Errorf(ctx, "create pull request: %v: %s", err, stderr.String())
	}
	return strings.TrimSpace(string(output)), nil
}
