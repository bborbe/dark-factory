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

// prCreator implements PRCreator.
type prCreator struct {
	ghToken string
}

// NewPRCreator creates a new PRCreator.
func NewPRCreator(ghToken string) PRCreator {
	return &prCreator{
		ghToken: ghToken,
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
	output, err := cmd.Output()
	if err != nil {
		return "", errors.Wrap(ctx, err, "list open PRs")
	}
	return strings.TrimSpace(string(output)), nil
}

// Create creates a pull request and returns the PR URL.
func (p *prCreator) Create(ctx context.Context, title string, body string) (string, error) {
	if strings.HasPrefix(title, "-") {
		return "", errors.Errorf(ctx, "invalid PR title: must not start with a dash")
	}
	// #nosec G204 -- title is from prompt frontmatter, body is static text
	cmd := exec.CommandContext(ctx, "gh", "pr", "create", "--title", title, "--body", body)
	if p.ghToken != "" {
		cmd.Env = append(os.Environ(), "GH_TOKEN="+p.ghToken)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return "", errors.Errorf(ctx, "create pull request: %v: %s", err, stderr.String())
	}
	return strings.TrimSpace(string(output)), nil
}
