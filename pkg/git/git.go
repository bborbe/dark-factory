// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"bufio"
	"context"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/bborbe/errors"
	"github.com/bborbe/run"

	"github.com/bborbe/dark-factory/pkg/subproc"
)

// DefaultCommitBackoff defines the default retry backoff for git commit operations.
// 3 retries with exponential backoff: ~2s, ~4s, ~8s.
var DefaultCommitBackoff = run.Backoff{
	Delay:   2 * time.Second,
	Factor:  1.0,
	Retries: 3,
}

// CommitWithRetry runs fn with retry logic using the given backoff configuration.
// Logs WARN on each retry attempt. Pass DefaultCommitBackoff for production use;
// tests can pass a Backoff with Delay: 0 and small Retries.
func CommitWithRetry(
	ctx context.Context,
	backoff run.Backoff,
	fn func(context.Context) error,
) error {
	return run.Retry(backoff, func(ctx context.Context) error {
		err := fn(ctx)
		if err != nil {
			if _, lockErr := os.Stat(".git/index.lock"); lockErr == nil {
				slog.Warn("retrying git commit, index.lock held", "error", err)
			} else {
				slog.Warn("retrying git commit after failure", "error", err)
			}
		}
		return err
	})(ctx)
}

// HasDirtyFiles returns true if there are any uncommitted changes in the working tree.
func HasDirtyFiles(ctx context.Context) (bool, error) {
	return NewHelpers().HasDirtyFiles(ctx)
}

// CommitAll stages all changes and commits with the given message.
// Used during committing recovery to commit work files left from a prior run.
func CommitAll(ctx context.Context, message string) error {
	return NewHelpers().CommitAll(ctx, message)
}

// VersionBump specifies the type of version bump to perform.
type VersionBump int

const (
	// PatchBump increments the patch version (vX.Y.Z -> vX.Y.Z+1)
	PatchBump VersionBump = iota
	// MinorBump increments the minor version (vX.Y.Z -> vX.Y+1.0)
	MinorBump
)

//counterfeiter:generate -o ../../mocks/releaser.go --fake-name Releaser . Releaser

// Releaser handles git commit, tag, and push operations.
type Releaser interface {
	GetNextVersion(ctx context.Context, bump VersionBump) (string, error)
	CommitAndRelease(ctx context.Context, bump VersionBump) error
	CommitCompletedFile(ctx context.Context, path string) error
	CommitOnly(ctx context.Context, message string) error
	HasChangelog(ctx context.Context) bool
	MoveFile(ctx context.Context, oldPath string, newPath string) error
	PushBranch(ctx context.Context) error
	// DetermineBump inspects CHANGELOG.md in the current directory and returns
	// the appropriate version bump. PatchBump if missing or no `- feat:` entry.
	DetermineBump(ctx context.Context) VersionBump
	// CommitWithRetry runs fn with the default retry backoff and lock-aware
	// logging. Application-layer code uses this seam instead of the package-
	// level git.CommitWithRetry so processor stays mockable.
	CommitWithRetry(ctx context.Context, fn func(context.Context) error) error
}

// releaser implements Releaser.
type releaser struct {
	helpers *Helpers
}

// NewReleaser creates a new Releaser.
func NewReleaser() Releaser {
	return &releaser{helpers: NewHelpers()}
}

// newReleaserWithRunner creates a Releaser with an injected runner (for tests).
func newReleaserWithRunner(r subproc.Runner) Releaser {
	return &releaser{helpers: NewHelpersWithRunner(r)}
}

// GetNextVersion determines the next version based on the bump type.
func (r *releaser) GetNextVersion(ctx context.Context, bump VersionBump) (string, error) {
	return r.helpers.getNextVersion(ctx, bump)
}

// CommitAndRelease performs the full git workflow.
func (r *releaser) CommitAndRelease(ctx context.Context, bump VersionBump) error {
	return r.helpers.CommitAndRelease(ctx, bump)
}

// CommitCompletedFile commits a completed prompt file to git.
func (r *releaser) CommitCompletedFile(ctx context.Context, path string) error {
	return r.helpers.CommitCompletedFile(ctx, path)
}

// HasChangelog checks if CHANGELOG.md exists in the current directory.
func (r *releaser) HasChangelog(ctx context.Context) bool {
	_, err := os.Stat("CHANGELOG.md")
	return err == nil
}

// DetermineBump inspects CHANGELOG.md in the current directory.
func (r *releaser) DetermineBump(ctx context.Context) VersionBump {
	return DetermineBumpFromChangelog(ctx, ".")
}

// CommitWithRetry runs fn with the default retry backoff.
func (r *releaser) CommitWithRetry(ctx context.Context, fn func(context.Context) error) error {
	return CommitWithRetry(ctx, DefaultCommitBackoff, fn)
}

// CommitOnly performs a simple commit without versioning, tagging, or pushing.
func (r *releaser) CommitOnly(ctx context.Context, message string) error {
	has, err := r.helpers.stageAllAndCheck(ctx)
	if err != nil {
		return err
	}
	if !has {
		slog.Info("no staged changes — skipping commit")
		return nil
	}
	if err := r.helpers.gitCommit(ctx, message); err != nil {
		return errors.Wrap(ctx, err, "git commit")
	}
	return nil
}

// MoveFile moves a file using git mv to preserve history.
func (r *releaser) MoveFile(ctx context.Context, oldPath string, newPath string) error {
	return r.helpers.MoveFile(ctx, oldPath, newPath)
}

// PushBranch pushes the current branch's commits to the remote.
func (r *releaser) PushBranch(ctx context.Context) error {
	return r.helpers.gitPush(ctx)
}

// CommitAndRelease performs the full git workflow (package-level wrapper for tests and scripts).
func CommitAndRelease(ctx context.Context, bump VersionBump) error {
	return NewHelpers().CommitAndRelease(ctx, bump)
}

// CommitCompletedFile stages and commits a completed prompt file (package-level wrapper).
func CommitCompletedFile(ctx context.Context, path string) error {
	return NewHelpers().CommitCompletedFile(ctx, path)
}

// MoveFile moves a file using git mv (package-level wrapper).
func MoveFile(ctx context.Context, oldPath string, newPath string) error {
	return NewHelpers().MoveFile(ctx, oldPath, newPath)
}

// GetNextVersion determines the next version (package-level wrapper).
func GetNextVersion(ctx context.Context, bump VersionBump) (string, error) {
	return NewHelpers().getNextVersion(ctx, bump)
}

// fallbackRename performs os.Rename when git operations are not available.
func fallbackRename(ctx context.Context, oldPath string, newPath string) error {
	if err := os.Rename(oldPath, newPath); err != nil {
		return errors.Wrap(ctx, err, "rename file")
	}
	return nil
}

// latestVersionFromChangelog parses CHANGELOG.md in the current working directory
// for lines matching "## vX.Y.Z" and returns the maximum version found.
// Returns nil if no valid version entries are found or the file does not exist.
func latestVersionFromChangelog(ctx context.Context) (*SemanticVersionNumber, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(ctx, err, "get working directory")
	}
	changelogPath := cwd + "/CHANGELOG.md"

	// #nosec G304 -- changelogPath is constructed from os.Getwd() + known filename, not user input
	f, err := os.Open(changelogPath)
	if err != nil {
		return nil, nil //nolint:nilerr // file not found is expected
	}
	defer f.Close()

	re := regexp.MustCompile(`^## (v\d+\.\d+\.\d+)`)
	var maxVersion *SemanticVersionNumber
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		matches := re.FindStringSubmatch(scanner.Text())
		if matches == nil {
			continue
		}
		version, parseErr := ParseSemanticVersionNumber(ctx, matches[1])
		if parseErr != nil {
			continue
		}
		if maxVersion == nil || maxVersion.Less(version) {
			v := version
			maxVersion = &v
		}
	}
	return maxVersion, nil
}

// updateChangelog renames ## Unreleased to version in CHANGELOG.md.
func updateChangelog(ctx context.Context, version string) error {
	changelogPath := "CHANGELOG.md"

	content, err := os.ReadFile(changelogPath)
	if err != nil {
		return errors.Wrap(ctx, err, "read changelog")
	}

	lines := strings.Split(string(content), "\n")
	result, unreleasedFound := processUnreleasedSection(lines, version)

	if !unreleasedFound {
		return errors.New(
			ctx,
			"CHANGELOG.md has no ## Unreleased section; YOLO must write changelog entries before release",
		)
	}

	output := strings.Join(result, "\n")
	if err := os.WriteFile(changelogPath, []byte(output), 0600); err != nil {
		return errors.Wrap(ctx, err, "write changelog")
	}

	return nil
}

// processUnreleasedSection renames ## Unreleased to ## version, preserving all content.
func processUnreleasedSection(lines []string, version string) ([]string, bool) {
	result := make([]string, 0, len(lines))
	unreleasedFound := false

	for _, line := range lines {
		if strings.HasPrefix(line, "## Unreleased") {
			unreleasedFound = true
			result = append(result, "## "+version)
			continue
		}

		result = append(result, line)
	}

	return result, unreleasedFound
}
