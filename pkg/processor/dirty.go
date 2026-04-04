// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"context"
	"os/exec"
	"strings"

	"github.com/bborbe/errors"
)

//counterfeiter:generate -o ../../mocks/dirty-file-checker.go --fake-name DirtyFileChecker . DirtyFileChecker

// DirtyFileChecker counts dirty files in a git working tree.
type DirtyFileChecker interface {
	CountDirtyFiles(ctx context.Context) (int, error)
}

// NewDirtyFileChecker creates a DirtyFileChecker that runs git status on the host.
func NewDirtyFileChecker(repoDir string) DirtyFileChecker {
	return &gitDirtyFileChecker{repoDir: repoDir}
}

type gitDirtyFileChecker struct {
	repoDir string
}

func (c *gitDirtyFileChecker) CountDirtyFiles(ctx context.Context) (int, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--short")
	cmd.Dir = c.repoDir
	output, err := cmd.Output()
	if err != nil {
		return 0, errors.Wrap(ctx, err, "git status --short")
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return 0, nil
	}
	return len(strings.Split(trimmed, "\n")), nil
}
