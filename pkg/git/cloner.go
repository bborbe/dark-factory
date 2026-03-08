// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/bborbe/errors"
)

// Cloner handles local git clone operations.
//
//counterfeiter:generate -o ../../mocks/cloner.go --fake-name Cloner . Cloner
type Cloner interface {
	Clone(ctx context.Context, srcDir string, destDir string, branch string) error
	Remove(ctx context.Context, path string) error
}

// cloner implements Cloner.
type cloner struct{}

// NewCloner creates a new Cloner.
func NewCloner() Cloner {
	return &cloner{}
}

// Clone creates a local clone of srcDir at destDir and checks out a new branch.
func (c *cloner) Clone(ctx context.Context, srcDir string, destDir string, branch string) error {
	slog.Debug("cloning repo", "src", srcDir, "dest", destDir, "branch", branch)

	// #nosec G204 -- srcDir and destDir are derived from config and prompt filename
	cmd := exec.CommandContext(ctx, "git", "clone", srcDir, destDir)
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "clone repo")
	}

	// Create and switch to feature branch
	// #nosec G204 -- destDir and branch are controlled by the application
	checkoutCmd := exec.CommandContext(ctx, "git", "-C", destDir, "checkout", "-b", branch)
	if err := checkoutCmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "create branch")
	}

	// Get the original remote URL from the source repo
	// #nosec G204 -- srcDir is controlled by the application
	remoteCmd := exec.CommandContext(ctx, "git", "-C", srcDir, "remote", "get-url", "origin")
	remoteOutput, err := remoteCmd.Output()
	if err != nil {
		return errors.Wrap(ctx, err, "get remote url")
	}

	// Set origin in the clone to the real remote (not the local source path)
	// #nosec G204 -- destDir and remoteURL are controlled by the application
	setRemoteCmd := exec.CommandContext(
		ctx,
		"git",
		"-C",
		destDir,
		"remote",
		"set-url",
		"origin",
		strings.TrimSpace(string(remoteOutput)),
	)
	if err := setRemoteCmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "set remote url")
	}

	return nil
}

// Remove removes the cloned directory.
func (c *cloner) Remove(ctx context.Context, path string) error {
	slog.Debug("removing clone", "path", path)
	return os.RemoveAll(path)
}
