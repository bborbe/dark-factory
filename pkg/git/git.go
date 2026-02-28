// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/bborbe/errors"
)

// CommitAndRelease performs the full git workflow:
// 1. git add -A
// 2. Read CHANGELOG.md and add entry
// 3. Bump version (patch)
// 4. git commit
// 5. git tag
// 6. git push + push tag
func CommitAndRelease(ctx context.Context, changelogEntry string) error {
	// Stage all changes
	if err := gitAddAll(ctx); err != nil {
		return errors.Wrap(ctx, err, "git add")
	}

	// Get next version
	nextVersion, err := getNextVersion(ctx)
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

// gitAddAll stages all changes
func gitAddAll(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "add", "-A")
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "run git add")
	}
	return nil
}

// getNextVersion determines the next version by bumping the patch version
func getNextVersion(ctx context.Context) (string, error) {
	// Get latest tag
	cmd := exec.CommandContext(ctx, "git", "describe", "--tags", "--abbrev=0")
	output, err := cmd.Output()
	if err != nil {
		// No tags exist yet, start with v0.1.0
		return "v0.1.0", nil
	}

	latestTag := strings.TrimSpace(string(output))
	return BumpPatchVersion(latestTag)
}

// BumpPatchVersion increments the patch version of a semver tag
func BumpPatchVersion(tag string) (string, error) {
	// Match vX.Y.Z
	re := regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)
	matches := re.FindStringSubmatch(tag)
	if matches == nil {
		return "", errors.Errorf(context.Background(), "invalid version tag: %s", tag)
	}

	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])

	// Bump patch
	patch++

	return "v" + strconv.Itoa(major) + "." + strconv.Itoa(minor) + "." + strconv.Itoa(patch), nil
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
	// #nosec G204 -- git commit message is controlled by the application
	cmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return errors.Wrapf(ctx, err, "run git commit: %s", stderr.String())
	}
	return nil
}

// gitTag creates a tag
func gitTag(ctx context.Context, tag string) error {
	// #nosec G204 -- git tag is generated by version bumping
	cmd := exec.CommandContext(ctx, "git", "tag", tag)
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "run git tag")
	}
	return nil
}

// gitPush pushes commits to remote
func gitPush(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "push")
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "run git push")
	}
	return nil
}

// gitPushTag pushes a tag to remote
func gitPushTag(ctx context.Context, tag string) error {
	// #nosec G204 -- git tag is generated by version bumping
	cmd := exec.CommandContext(ctx, "git", "push", "origin", tag)
	if err := cmd.Run(); err != nil {
		return errors.Wrap(ctx, err, "run git push tag")
	}
	return nil
}
