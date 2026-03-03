// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"log/slog"
	"os/exec"

	"github.com/bborbe/errors"
)

// Worktree handles git worktree operations.
//
//counterfeiter:generate -o ../../mocks/worktree.go --fake-name Worktree . Worktree
type Worktree interface {
	Add(ctx context.Context, path string, branch string) error
	Remove(ctx context.Context, path string) error
}

// worktree implements Worktree.
type worktree struct{}

// NewWorktree creates a new Worktree.
func NewWorktree() Worktree {
	return &worktree{}
}

// Add creates a new worktree at the specified path with a new branch.
func (w *worktree) Add(ctx context.Context, path string, branch string) error {
	slog.Debug("adding worktree", "path", path, "branch", branch)

	// #nosec G204 -- path and branch are derived from prompt filename and sanitized
	cmd := exec.CommandContext(ctx, "git", "worktree", "add", path, "-b", branch)
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "add worktree")
	}
	return nil
}

// Remove removes a worktree at the specified path.
func (w *worktree) Remove(ctx context.Context, path string) error {
	slog.Debug("removing worktree", "path", path)

	// #nosec G204 -- path is controlled by the application
	cmd := exec.CommandContext(ctx, "git", "worktree", "remove", path)
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "remove worktree")
	}
	return nil
}
