// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/subproc"
)

//counterfeiter:generate -o ../../mocks/dirty-file-checker.go --fake-name DirtyFileChecker . DirtyFileChecker

// DirtyFileChecker counts dirty files in a git working tree.
type DirtyFileChecker interface {
	CountDirtyFiles(ctx context.Context) (int, error)
}

// NewDirtyFileChecker creates a DirtyFileChecker that runs git status on the host.
func NewDirtyFileChecker(repoDir string) DirtyFileChecker {
	return &gitDirtyFileChecker{repoDir: repoDir, runner: subproc.NewRunner()}
}

func newDirtyFileCheckerWithRunner(repoDir string, runner subproc.Runner) *gitDirtyFileChecker {
	return &gitDirtyFileChecker{repoDir: repoDir, runner: runner}
}

type gitDirtyFileChecker struct {
	repoDir string
	runner  subproc.Runner
}

func (c *gitDirtyFileChecker) CountDirtyFiles(ctx context.Context) (int, error) {
	output, err := c.runner.RunWithWarnAndTimeoutDir(
		ctx,
		"git status --short",
		c.repoDir,
		"git",
		"status",
		"--short",
	)
	if err != nil {
		return 0, errors.Wrap(ctx, err, "git status --short")
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return 0, nil
	}
	return len(strings.Split(trimmed, "\n")), nil
}
