// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runner

import (
	"context"
	stderrors "errors"
	"os"

	"github.com/bborbe/errors"
)

// ErrWorktreeOrSubmodule is returned when dark-factory is started from a git worktree or
// submodule CWD (where .git is a regular file pointing to the parent repo) without hideGit=true.
var ErrWorktreeOrSubmodule = stderrors.New(
	"worktree CWD detected: .git is a file (worktree or submodule); " +
		"dark-factory cannot run from a worktree unless hideGit=true. " +
		"To proceed: --set hideGit=true or add 'hideGit: true' to .dark-factory.yaml. " +
		"See docs/troubleshooting.md and the 'PR via Pre-Created Worktree' runbook for details",
)

// DetectWorktreeOrSubmodule checks whether the current working directory is a git worktree or
// submodule. In both cases, .git is a regular file (not a directory) that points into the
// parent repository's .git/worktrees or .git/modules directory.
func DetectWorktreeOrSubmodule(ctx context.Context) error {
	info, err := os.Lstat(".git")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrapf(ctx, err, "lstat .git failed")
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	return ErrWorktreeOrSubmodule
}
