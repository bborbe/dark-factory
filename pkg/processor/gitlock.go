// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package processor

import (
	"os"
	"path/filepath"
)

//counterfeiter:generate -o ../../mocks/git-lock-checker.go --fake-name GitLockChecker . GitLockChecker

// GitLockChecker checks whether .git/index.lock exists in the working tree.
type GitLockChecker interface {
	Exists() bool
}

// NewGitLockChecker creates a GitLockChecker for the given repo directory.
func NewGitLockChecker(repoDir string) GitLockChecker {
	return &gitLockChecker{repoDir: repoDir}
}

type gitLockChecker struct {
	repoDir string
}

func (c *gitLockChecker) Exists() bool {
	_, err := os.Stat(filepath.Join(c.repoDir, ".git", "index.lock"))
	return err == nil
}
