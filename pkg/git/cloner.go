// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/subproc"
)

//counterfeiter:generate -o ../../mocks/cloner.go --fake-name Cloner . Cloner

// Cloner handles local git clone operations.
type Cloner interface {
	Clone(ctx context.Context, srcDir string, destDir string, branch string) error
	Remove(ctx context.Context, path string) error
}

// NewCloner creates a new Cloner.
func NewCloner() Cloner {
	return &cloner{runner: subproc.NewRunner()}
}

// newClonerWithRunner creates a Cloner with an injected runner (for tests).
func newClonerWithRunner(r subproc.Runner) *cloner {
	return &cloner{runner: r}
}

// cloner implements Cloner.
type cloner struct {
	runner subproc.Runner
}

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
	_, err := c.runner.RunWithWarnAndTimeout(ctx, "git clone", "git", "clone", srcDir, destDir)
	if err != nil {
		return errors.Wrapf(
			ctx,
			err,
			"clone repo (src=%s dest=%s): %s",
			srcDir,
			destDir,
			stderrFromErr(err),
		)
	}
	return nil
}

// setRealRemote updates the clone's origin to the real remote URL from srcDir.
func (c *cloner) setRealRemote(ctx context.Context, srcDir string, destDir string) error {
	remoteOutput, err := c.runner.RunWithWarnAndTimeoutDir(
		ctx,
		"git remote get-url",
		srcDir,
		"git",
		"remote",
		"get-url",
		"origin",
	)
	if err != nil {
		return errors.Wrapf(ctx, err, "get remote url: %s", stderrFromErr(err))
	}
	_, err = c.runner.RunWithWarnAndTimeoutDir(
		ctx,
		"git remote set-url",
		destDir,
		"git",
		"remote",
		"set-url",
		"origin",
		strings.TrimSpace(string(remoteOutput)),
	)
	if err != nil {
		return errors.Wrap(ctx, err, "set remote url")
	}
	return nil
}

// checkoutBranch fetches from origin and checks out branch, tracking it if it exists remotely.
func (c *cloner) checkoutBranch(ctx context.Context, destDir string, branch string) error {
	// Fetch from real remote to detect existing branches (best-effort)
	if _, err := c.runner.RunWithWarnAndTimeoutDir(ctx, "git fetch origin", destDir, "git", "fetch", "origin"); err != nil {
		slog.Warn("fetch from origin failed in clone, will create fresh branch", "error", err)
	}

	// Check if branch already exists on origin
	_, verifyErr := c.runner.RunWithWarnAndTimeoutDir(
		ctx,
		"git rev-parse --verify",
		destDir,
		"git",
		"rev-parse",
		"--verify",
		"origin/"+branch,
	)
	if verifyErr == nil {
		return c.checkoutTrack(ctx, destDir, branch)
	}
	return c.checkoutNew(ctx, destDir, branch)
}

// checkoutTrack checks out an existing remote branch and tracks it.
func (c *cloner) checkoutTrack(ctx context.Context, destDir string, branch string) error {
	_, err := c.runner.RunWithWarnAndTimeoutDir(
		ctx,
		"git checkout --track",
		destDir,
		"git",
		"checkout",
		"--track",
		"origin/"+branch,
	)
	if err != nil {
		return errors.Wrapf(
			ctx,
			err,
			"track existing branch (dest=%s branch=%s): %s",
			destDir,
			branch,
			stderrFromErr(err),
		)
	}
	slog.Info("tracking existing remote branch", "branch", branch)
	return nil
}

// checkoutNew creates a new local branch.
func (c *cloner) checkoutNew(ctx context.Context, destDir string, branch string) error {
	_, err := c.runner.RunWithWarnAndTimeoutDir(
		ctx,
		"git checkout -b",
		destDir,
		"git",
		"checkout",
		"-b",
		branch,
	)
	if err != nil {
		return errors.Wrapf(
			ctx,
			err,
			"create branch (dest=%s branch=%s): %s",
			destDir,
			branch,
			stderrFromErr(err),
		)
	}
	return nil
}

// Remove removes the cloned directory.
func (c *cloner) Remove(ctx context.Context, path string) error {
	slog.Debug("removing clone", "path", path)
	return os.RemoveAll(path)
}
