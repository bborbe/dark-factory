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

//counterfeiter:generate -o ../../mocks/cloner.go --fake-name Cloner . Cloner

// Cloner handles local git clone operations.
type Cloner interface {
	Clone(ctx context.Context, srcDir string, destDir string, branch string) error
	Remove(ctx context.Context, path string) error
}

// NewCloner creates a new Cloner.
func NewCloner() Cloner {
	return &cloner{}
}

// cloner implements Cloner.
type cloner struct{}

// Clone creates a local clone of srcDir at destDir and checks out the branch.
// If the branch already exists on the remote, it is tracked; otherwise a new branch is created.
// If destDir already exists (e.g. from a previous crashed run), it is removed first.
func (c *cloner) Clone(ctx context.Context, srcDir string, destDir string, branch string) error {
	slog.Info("cloning repo", "src", srcDir, "dest", destDir, "branch", branch)

	if err := c.removeStale(ctx, destDir); err != nil {
		return errors.Wrap(ctx, err, "remove stale clone")
	}
	if err := c.gitClone(ctx, srcDir, destDir); err != nil {
		return errors.Wrap(ctx, err, "git clone")
	}
	if err := c.setRealRemote(ctx, srcDir, destDir); err != nil {
		return errors.Wrap(ctx, err, "set real remote")
	}
	return c.checkoutBranch(ctx, destDir, branch)
}

// removeStale removes destDir if it already exists (stale clone from a previous run).
func (c *cloner) removeStale(ctx context.Context, destDir string) error {
	if _, err := os.Stat(destDir); err == nil {
		slog.Warn("removing stale clone directory", "path", destDir)
		if err := os.RemoveAll(destDir); err != nil {
			return errors.Wrapf(ctx, err, "remove stale clone at %s", destDir)
		}
	}
	return nil
}

// gitClone runs git clone srcDir destDir.
func (c *cloner) gitClone(ctx context.Context, srcDir string, destDir string) error {
	// #nosec G204 -- srcDir and destDir are derived from config and prompt filename
	cmd := exec.CommandContext(ctx, "git", "clone", srcDir, destDir)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(
			ctx,
			err,
			"clone repo (src=%s dest=%s): %s",
			srcDir,
			destDir,
			stderr.String(),
		)
	}
	return nil
}

// setRealRemote updates the clone's origin to the real remote URL from srcDir.
// This must happen before fetch so we fetch from the real remote, not the local path.
func (c *cloner) setRealRemote(ctx context.Context, srcDir string, destDir string) error {
	// #nosec G204 -- srcDir is controlled by the application
	remoteCmd := exec.CommandContext(ctx, "git", "-C", srcDir, "remote", "get-url", "origin")
	remoteOutput, err := remoteCmd.Output()
	if err != nil {
		return errors.Wrap(ctx, err, "get remote url")
	}
	// #nosec G204 -- destDir and remoteURL are controlled by the application
	setRemoteCmd := exec.CommandContext(
		ctx, "git", "-C", destDir, "remote", "set-url", "origin",
		strings.TrimSpace(string(remoteOutput)),
	)
	if err := setRemoteCmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "set remote url")
	}
	return nil
}

// checkoutBranch fetches from origin and checks out branch, tracking it if it exists remotely.
func (c *cloner) checkoutBranch(ctx context.Context, destDir string, branch string) error {
	// Fetch from real remote to detect existing branches (best-effort)
	// #nosec G204 -- destDir is controlled by the application
	fetchCmd := exec.CommandContext(ctx, "git", "-C", destDir, "fetch", "origin")
	if err := fetchCmd.Run(); err != nil {
		slog.Warn("fetch from origin failed in clone, will create fresh branch", "error", err)
	}

	// Check if branch already exists on origin
	// #nosec G204 -- destDir and branch are controlled by the application
	verifyCmd := exec.CommandContext(
		ctx,
		"git",
		"-C",
		destDir,
		"rev-parse",
		"--verify",
		"origin/"+branch,
	)
	if verifyCmd.Run() == nil {
		return c.checkoutTrack(ctx, destDir, branch)
	}
	return c.checkoutNew(ctx, destDir, branch)
}

// checkoutTrack checks out an existing remote branch and tracks it.
func (c *cloner) checkoutTrack(ctx context.Context, destDir string, branch string) error {
	// #nosec G204 -- destDir and branch are controlled by the application
	cmd := exec.CommandContext(ctx, "git", "-C", destDir, "checkout", "--track", "origin/"+branch)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(
			ctx,
			err,
			"track existing branch (dest=%s branch=%s): %s",
			destDir,
			branch,
			stderr.String(),
		)
	}
	slog.Info("tracking existing remote branch", "branch", branch)
	return nil
}

// checkoutNew creates a new local branch.
func (c *cloner) checkoutNew(ctx context.Context, destDir string, branch string) error {
	// #nosec G204 -- destDir and branch are controlled by the application
	cmd := exec.CommandContext(ctx, "git", "-C", destDir, "checkout", "-b", branch)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return errors.Wrapf(
			ctx,
			err,
			"create branch (dest=%s branch=%s): %s",
			destDir,
			branch,
			stderr.String(),
		)
	}
	return nil
}

// Remove removes the cloned directory.
func (c *cloner) Remove(ctx context.Context, path string) error {
	slog.Debug("removing clone", "path", path)
	return os.RemoveAll(path)
}
