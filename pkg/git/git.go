// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/bborbe/errors"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
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
	CommitAndRelease(ctx context.Context, title string, bump VersionBump) error
	CommitCompletedFile(ctx context.Context, path string) error
	CommitOnly(ctx context.Context, message string) error
	HasChangelog(ctx context.Context) bool
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
func (r *releaser) CommitAndRelease(ctx context.Context, title string, bump VersionBump) error {
	return CommitAndRelease(ctx, title, bump)
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

// CommitAndRelease performs the full git workflow:
// 1. git add -A
// 2. Read CHANGELOG.md and add entry
// 3. Bump version (patch or minor)
// 4. git commit
// 5. git tag
// 6. git push + push tag
func CommitAndRelease(ctx context.Context, changelogEntry string, bump VersionBump) error {
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
	if err := updateChangelog(ctx, changelogEntry, nextVersion); err != nil {
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
	repo, err := gogit.PlainOpen(".")
	if err != nil {
		return errors.Wrap(ctx, err, "open git repository")
	}

	wt, err := repo.Worktree()
	if err != nil {
		return errors.Wrap(ctx, err, "get worktree")
	}

	// Check if there's anything to commit before staging
	statusBefore, err := wt.Status()
	if err != nil {
		return errors.Wrap(ctx, err, "get status before add")
	}

	// Stage all changes (equivalent to git add -A)
	// This handles file moves properly by staging both the deletion
	// of the old path and the addition of the new path
	if err := wt.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
		return errors.Wrap(ctx, err, "add all changes")
	}

	// Check if there's anything to commit after staging
	statusAfter, err := wt.Status()
	if err != nil {
		return errors.Wrap(ctx, err, "get status after add")
	}

	// Nothing to commit - file already staged or committed
	if statusBefore.IsClean() && statusAfter.IsClean() {
		return nil
	}

	// Commit the completed file
	return gitCommit(ctx, "move prompt to completed")
}

// gitAddAll stages all changes
func gitAddAll(ctx context.Context) error {
	repo, err := gogit.PlainOpen(".")
	if err != nil {
		return errors.Wrap(ctx, err, "open git repository")
	}

	wt, err := repo.Worktree()
	if err != nil {
		return errors.Wrap(ctx, err, "get worktree")
	}

	// Add all files (equivalent to git add -A)
	if err := wt.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
		return errors.Wrap(ctx, err, "add all files")
	}

	return nil
}

// GetNextVersion determines the next version based on the bump type.
func GetNextVersion(ctx context.Context, bump VersionBump) (string, error) {
	return getNextVersion(ctx, bump)
}

// getNextVersion determines the next version based on the bump type
func getNextVersion(ctx context.Context, bump VersionBump) (string, error) {
	repo, err := gogit.PlainOpen(".")
	if err != nil {
		return "", errors.Wrap(ctx, err, "open git repository")
	}

	// Get all tags
	tags, err := repo.Tags()
	if err != nil {
		return "", errors.Wrap(ctx, err, "get tags")
	}

	// Collect and parse all valid semver tags
	var versions []SemanticVersionNumber
	err = tags.ForEach(func(ref *plumbing.Reference) error {
		tagName := ref.Name().Short()
		version, parseErr := ParseSemanticVersionNumber(ctx, tagName)
		if parseErr != nil {
			// Skip invalid semver tags
			return nil
		}
		versions = append(versions, version)
		return nil
	})
	if err != nil {
		return "", errors.Wrap(ctx, err, "iterate tags")
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

// updateChangelog adds an entry to CHANGELOG.md and renames ## Unreleased to version
//
//nolint:gocognit
func updateChangelog(ctx context.Context, entry string, version string) error {
	changelogPath := "CHANGELOG.md"

	content, err := os.ReadFile(changelogPath)
	if err != nil {
		return errors.Wrap(ctx, err, "read changelog")
	}

	lines := strings.Split(string(content), "\n")
	result, unreleasedFound := processUnreleasedSection(lines, entry, version)

	if !unreleasedFound {
		result = insertNewVersionSection(lines, entry, version)
	}

	output := strings.Join(result, "\n")
	if err := os.WriteFile(changelogPath, []byte(output), 0600); err != nil {
		return errors.Wrap(ctx, err, "write changelog")
	}

	return nil
}

// processUnreleasedSection processes lines and replaces ## Unreleased with version
func processUnreleasedSection(lines []string, entry string, version string) ([]string, bool) {
	result := make([]string, 0, len(lines)+3)
	unreleasedFound := false
	skip := false

	for i, line := range lines {
		if skip {
			skip = false
			continue
		}

		if strings.HasPrefix(line, "## Unreleased") {
			unreleasedFound = true
			result = append(result, "## "+version)

			if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "###") {
				result = append(result, lines[i+1])
				result = append(result, "- "+entry)
				skip = true
				continue
			}

			result = append(result, "")
			result = append(result, "### Added")
			result = append(result, "- "+entry)
			continue
		}

		result = append(result, line)
	}

	return result, unreleasedFound
}

// insertNewVersionSection inserts a new version section when no Unreleased exists
func insertNewVersionSection(lines []string, entry string, version string) []string {
	insertIndex := findInsertIndex(lines)
	if insertIndex == -1 {
		return lines
	}

	newSection := []string{
		"## " + version,
		"",
		"### Added",
		"- " + entry,
		"",
	}

	result := make([]string, 0, len(lines)+len(newSection))
	result = append(result, lines[:insertIndex]...)
	result = append(result, newSection...)
	result = append(result, lines[insertIndex:]...)

	return result
}

// findInsertIndex finds where to insert a new version section
func findInsertIndex(lines []string) int {
	for i, line := range lines {
		if line == "" && i > 0 &&
			(strings.HasPrefix(lines[i-1], "#") || strings.Contains(lines[i-1], "Semantic Versioning")) {
			return i + 1
		}
	}

	for i, line := range lines {
		if line == "" {
			return i + 1
		}
	}

	return -1
}

// gitCommit creates a commit with the given message
func gitCommit(ctx context.Context, message string) error {
	repo, err := gogit.PlainOpen(".")
	if err != nil {
		return errors.Wrap(ctx, err, "open git repository")
	}

	wt, err := repo.Worktree()
	if err != nil {
		return errors.Wrap(ctx, err, "get worktree")
	}

	// Get git config for author information
	cfg, err := repo.Config()
	if err != nil {
		return errors.Wrap(ctx, err, "get git config")
	}

	author := &object.Signature{
		Name:  cfg.User.Name,
		Email: cfg.User.Email,
		When:  time.Now(),
	}

	commitOpts := &gogit.CommitOptions{
		Author: author,
	}

	_, err = wt.Commit(message, commitOpts)
	if err != nil {
		return errors.Wrap(ctx, err, "create commit")
	}

	return nil
}

// gitTag creates a tag
func gitTag(ctx context.Context, tag string) error {
	repo, err := gogit.PlainOpen(".")
	if err != nil {
		return errors.Wrap(ctx, err, "open git repository")
	}

	head, err := repo.Head()
	if err != nil {
		return errors.Wrap(ctx, err, "get HEAD")
	}

	_, err = repo.CreateTag(tag, head.Hash(), nil)
	if err != nil {
		return errors.Wrap(ctx, err, "create tag")
	}

	return nil
}

// gitPush pushes commits to remote
func gitPush(ctx context.Context) error {
	repo, err := gogit.PlainOpen(".")
	if err != nil {
		return errors.Wrap(ctx, err, "open git repository")
	}

	err = repo.Push(&gogit.PushOptions{})
	if err != nil && err != gogit.NoErrAlreadyUpToDate {
		return errors.Wrap(ctx, err, "push to remote")
	}

	return nil
}

// gitPushTag pushes a tag to remote
func gitPushTag(ctx context.Context, tag string) error {
	repo, err := gogit.PlainOpen(".")
	if err != nil {
		return errors.Wrap(ctx, err, "open git repository")
	}

	// Push the specific tag
	refSpec := config.RefSpec("refs/tags/" + tag + ":refs/tags/" + tag)
	err = repo.Push(&gogit.PushOptions{
		RefSpecs: []config.RefSpec{refSpec},
	})
	if err != nil && err != gogit.NoErrAlreadyUpToDate {
		return errors.Wrap(ctx, err, "push tag to remote")
	}

	return nil
}
