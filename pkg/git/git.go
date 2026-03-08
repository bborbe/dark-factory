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

// VersionBump specifies the type of version bump to perform.
type VersionBump int

const (
	// PatchBump increments the patch version (vX.Y.Z -> vX.Y.Z+1)
	PatchBump VersionBump = iota
	// MinorBump increments the minor version (vX.Y.Z -> vX.Y+1.0)
	MinorBump
)

// Releaser handles git commit, tag, and push operations.
//
//counterfeiter:generate -o ../../mocks/releaser.go --fake-name Releaser . Releaser
type Releaser interface {
	GetNextVersion(ctx context.Context, bump VersionBump) (string, error)
	CommitAndRelease(ctx context.Context, bump VersionBump) error
	CommitCompletedFile(ctx context.Context, path string) error
	CommitOnly(ctx context.Context, message string) error
	HasChangelog(ctx context.Context) bool
	MoveFile(ctx context.Context, oldPath string, newPath string) error
}

// releaser implements Releaser.
type releaser struct{}

// NewReleaser creates a new Releaser.
func NewReleaser() Releaser {
	return &releaser{}
}

// GetNextVersion determines the next version based on the bump type.
func (r *releaser) GetNextVersion(ctx context.Context, bump VersionBump) (string, error) {
	return getNextVersion(ctx, bump)
}

// CommitAndRelease performs the full git workflow.
func (r *releaser) CommitAndRelease(ctx context.Context, bump VersionBump) error {
	return CommitAndRelease(ctx, bump)
}

// CommitCompletedFile commits a completed prompt file to git.
func (r *releaser) CommitCompletedFile(ctx context.Context, path string) error {
	return CommitCompletedFile(ctx, path)
}

// HasChangelog checks if CHANGELOG.md exists in the current directory.
func (r *releaser) HasChangelog(ctx context.Context) bool {
	_, err := os.Stat("CHANGELOG.md")
	return err == nil
}

// CommitOnly performs a simple commit without versioning, tagging, or pushing.
// This is used for projects without a CHANGELOG.md.
func (r *releaser) CommitOnly(ctx context.Context, message string) error {
	// Stage all changes
	if err := gitAddAll(ctx); err != nil {
		return errors.Wrap(ctx, err, "git add")
	}

	// Commit
	if err := gitCommit(ctx, message); err != nil {
		return errors.Wrap(ctx, err, "git commit")
	}

	return nil
}

// MoveFile moves a file using git mv to preserve history.
// Falls back to os.Rename if git operations fail or not in a git repo.
func (r *releaser) MoveFile(ctx context.Context, oldPath string, newPath string) error {
	return MoveFile(ctx, oldPath, newPath)
}

// CommitAndRelease performs the full git workflow:
// 1. git add -A
// 2. Read CHANGELOG.md and rename ## Unreleased to version
// 3. Bump version (patch or minor)
// 4. git commit
// 5. git tag
// 6. git push + push tag
func CommitAndRelease(ctx context.Context, bump VersionBump) error {
	// Stage all changes
	if err := gitAddAll(ctx); err != nil {
		return errors.Wrap(ctx, err, "git add")
	}

	// Get next version
	nextVersion, err := getNextVersion(ctx, bump)
	if err != nil {
		return errors.Wrap(ctx, err, "get next version")
	}

	// Update CHANGELOG
	if err := updateChangelog(ctx, nextVersion); err != nil {
		return errors.Wrap(ctx, err, "update changelog")
	}

	// Stage CHANGELOG changes
	if err := gitAddAll(ctx); err != nil {
		return errors.Wrap(ctx, err, "git add changelog")
	}

	// Commit
	commitMsg := "release " + nextVersion
	if err := gitCommit(ctx, commitMsg); err != nil {
		return errors.Wrap(ctx, err, "git commit")
	}

	// Tag
	if err := gitTag(ctx, nextVersion); err != nil {
		return errors.Wrap(ctx, err, "git tag")
	}

	// Push
	if err := gitPush(ctx); err != nil {
		return errors.Wrap(ctx, err, "git push")
	}

	// Push tag
	if err := gitPushTag(ctx, nextVersion); err != nil {
		return errors.Wrap(ctx, err, "git push tag")
	}

	return nil
}

// CommitCompletedFile stages and commits a completed prompt file.
// This is called after MoveToCompleted() to ensure the moved file is committed.
// Does nothing if the file is already staged or committed.
func CommitCompletedFile(ctx context.Context, path string) error {
	// Stage only the specified file
	// #nosec G204 -- path is from completed prompt file, controlled by application
	cmd := exec.CommandContext(ctx, "git", "add", path)
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "git add")
	}

	// Check if there's anything to commit
	statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	output, err := statusCmd.Output()
	if err != nil {
		return errors.Wrap(ctx, err, "git status")
	}

	// Nothing to commit
	if len(strings.TrimSpace(string(output))) == 0 {
		return nil
	}

	// Commit
	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", "move prompt to completed")
	if err := commitCmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "git commit")
	}
	return nil
}

// MoveFile moves a file using git mv to preserve history.
// Falls back to os.Rename if git operations fail or not in a git repo.
func MoveFile(ctx context.Context, oldPath string, newPath string) error {
	// #nosec G204 -- paths are controlled by the application, not user input
	cmd := exec.CommandContext(ctx, "git", "mv", oldPath, newPath)
	if err := cmd.Run(); err != nil {
		// git mv failed (not a repo, file not tracked, etc.) — fallback to os.Rename
		return fallbackRename(ctx, oldPath, newPath)
	}
	return nil
}

// fallbackRename performs os.Rename when git operations are not available.
func fallbackRename(ctx context.Context, oldPath string, newPath string) error {
	if err := os.Rename(oldPath, newPath); err != nil {
		return errors.Wrap(ctx, err, "rename file")
	}
	return nil
}

// gitAddAll stages all changes
func gitAddAll(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "add", "-A")
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "git add all")
	}
	return nil
}

// GetNextVersion determines the next version based on the bump type.
func GetNextVersion(ctx context.Context, bump VersionBump) (string, error) {
	return getNextVersion(ctx, bump)
}

// getNextVersion determines the next version based on the bump type
func getNextVersion(ctx context.Context, bump VersionBump) (string, error) {
	// #nosec G204 -- arguments are static, not user input
	cmd := exec.CommandContext(ctx, "git", "tag", "--list", "v*")
	out, err := cmd.Output()
	if err != nil {
		return "", errors.Wrap(ctx, err, "list git tags")
	}

	// Collect and parse all valid semver tags
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	versions := make([]SemanticVersionNumber, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		version, parseErr := ParseSemanticVersionNumber(ctx, line)
		if parseErr != nil {
			continue // Skip invalid semver tags
		}
		versions = append(versions, version)
	}

	// If no valid semver tags exist, start with v0.1.0
	if len(versions) == 0 {
		return "v0.1.0", nil
	}

	// Find the maximum version using proper semver comparison
	maxVersion := versions[0]
	for _, v := range versions[1:] {
		if maxVersion.Less(v) {
			maxVersion = v
		}
	}

	// Apply the appropriate bump
	var nextVersion SemanticVersionNumber
	switch bump {
	case MinorBump:
		nextVersion = maxVersion.BumpMinor()
	case PatchBump:
		nextVersion = maxVersion.BumpPatch()
	default:
		nextVersion = maxVersion.BumpPatch()
	}

	return nextVersion.String(), nil
}

// updateChangelog renames ## Unreleased to version in CHANGELOG.md.
// Returns an error if no ## Unreleased section is found — YOLO is expected to have written it.
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

// processUnreleasedSection renames ## Unreleased to ## version, preserving all content
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

// gitCommit creates a commit with the given message
func gitCommit(ctx context.Context, message string) error {
	slog.Debug("creating commit", "message", message)

	// #nosec G204 -- message is passed as argument to git commit, not executed
	cmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "create commit")
	}
	return nil
}

// gitTag creates a tag
func gitTag(ctx context.Context, tag string) error {
	if _, err := ParseSemanticVersionNumber(ctx, tag); err != nil {
		return errors.Wrap(ctx, err, "invalid tag format")
	}

	slog.Debug("creating tag", "tag", tag)

	// #nosec G204 -- tag is validated as semantic version above
	cmd := exec.CommandContext(ctx, "git", "tag", tag)
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "create tag")
	}
	return nil
}

// gitPush pushes commits to remote using subprocess
func gitPush(ctx context.Context) error {
	slog.Debug("pushing commits to remote")

	cmd := exec.CommandContext(ctx, "git", "push")
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "push to remote")
	}

	return nil
}

// gitPushTag pushes a tag to remote using subprocess
func gitPushTag(ctx context.Context, tag string) error {
	// Validate tag format to prevent command injection
	if _, err := ParseSemanticVersionNumber(ctx, tag); err != nil {
		return errors.Wrap(ctx, err, "invalid tag format")
	}

	slog.Debug("pushing tag to remote", "tag", tag)

	// #nosec G204 -- tag is validated as semantic version above
	cmd := exec.CommandContext(ctx, "git", "push", "origin", tag)
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "push tag to remote")
	}

	return nil
}
