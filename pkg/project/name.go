// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
func Resolve(configOverride string) Name {
	// 1. Config override takes precedence
	if configOverride != "" {
		return Name(configOverride)
	}

	// 2. Try git working tree root
	if name := tryGitRoot(); name != "" {
		return Name(name)
	}

	// 3. Try git remote URL
	if name := tryGitRemote(); name != "" {
		return Name(name)
	}

	// 4. Fallback to current working directory
	if wd, err := os.Getwd(); err == nil {
		return Name(filepath.Base(wd))
	}

	// Ultimate fallback (should never happen)
	return Name("dark-factory")
}

// tryGitRoot tries to get the basename of the git working tree root.
func tryGitRoot() string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return ""
	}
	return filepath.Base(root)
}

// tryGitRemote tries to get the repo name from the git remote URL.
func tryGitRemote() string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	url := strings.TrimSpace(string(output))
	if url == "" {
		return ""
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

	return name
}
