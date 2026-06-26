// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package project

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/subproc"
)

// Name is a typed alias for the resolved project name. Construct via Resolve.
type Name string

// String returns the underlying string for use at boundaries (logs, exec args, etc.).
func (n Name) String() string { return string(n) }

// Resolve resolves the project name using the fallback chain:
// 1. Config override (if non-empty)
// 2. Git repository root directory name
// 3. Git remote repo name (origin URL → extract repo name)
// 4. Working directory name
// 5. Literal "dark-factory"
//
// Returns an error only when ctx is cancelled or timed out during a git probe.
// A git command merely failing (not in a repo) is not an error — it falls through.
func Resolve(ctx context.Context, runner subproc.Runner, configOverride string) (Name, error) {
	// 1. Config override takes precedence
	if configOverride != "" {
		return Name(configOverride), nil
	}

	// 2. Try git working tree root
	name, err := tryGitRoot(ctx, runner)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", errors.Wrap(ctx, err, "resolve project name via git root")
		}
	}
	if name != "" {
		return Name(name), nil
	}

	// 3. Try git remote URL
	name, err = tryGitRemote(ctx, runner)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", errors.Wrap(ctx, err, "resolve project name via git remote")
		}
	}
	if name != "" {
		return Name(name), nil
	}

	// 4. Fallback to current working directory
	if wd, err := os.Getwd(); err == nil {
		return Name(filepath.Base(wd)), nil
	}

	// 5. Ultimate fallback (should never happen)
	return Name("dark-factory"), nil
}

// tryGitRoot tries to get the basename of the git working tree root.
func tryGitRoot(ctx context.Context, runner subproc.Runner) (string, error) {
	output, err := runner.RunWithWarnAndTimeout(
		ctx,
		"git rev-parse --show-toplevel",
		"git",
		"rev-parse",
		"--show-toplevel",
	)
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", nil
	}
	return filepath.Base(root), nil
}

// tryGitRemote tries to get the repo name from the git remote URL.
func tryGitRemote(ctx context.Context, runner subproc.Runner) (string, error) {
	output, err := runner.RunWithWarnAndTimeout(
		ctx,
		"git remote get-url origin",
		"git",
		"remote",
		"get-url",
		"origin",
	)
	if err != nil {
		return "", err
	}
	url := strings.TrimSpace(string(output))
	if url == "" {
		return "", nil
	}

	// Extract repo name from URL
	// Examples:
	// - https://github.com/user/repo.git → repo
	// - git@github.com:user/repo.git → repo
	// - /path/to/repo.git → repo

	// Get the last path component
	name := filepath.Base(url)

	// Strip .git suffix if present
	name = strings.TrimSuffix(name, ".git")

	return name, nil
}
