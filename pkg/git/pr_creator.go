// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"os/exec"
	"strings"

	"github.com/bborbe/errors"
)

// PRCreator handles GitHub pull request creation.
//
//counterfeiter:generate -o ../../mocks/pr_creator.go --fake-name PRCreator . PRCreator
type PRCreator interface {
	Create(ctx context.Context, title string, body string) (string, error)
}

// prCreator implements PRCreator.
type prCreator struct{}

// NewPRCreator creates a new PRCreator.
func NewPRCreator() PRCreator {
	return &prCreator{}
}

// Create creates a pull request and returns the PR URL.
func (p *prCreator) Create(ctx context.Context, title string, body string) (string, error) {
	// #nosec G204 -- title is from prompt frontmatter, body is static text
	cmd := exec.CommandContext(ctx, "gh", "pr", "create", "--title", title, "--body", body)
	output, err := cmd.Output()
	if err != nil {
		return "", errors.Wrap(ctx, err, "create pull request")
	}
	return strings.TrimSpace(string(output)), nil
}
