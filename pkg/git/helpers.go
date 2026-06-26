// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"log/slog"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/subproc"
)

// Helpers groups git CLI free functions onto a runner-bearing struct so
// production wiring injects a runner once and tests inject a fake.
type Helpers struct {
	runner subproc.Runner
}

// NewHelpers wires a Helpers with the default production runner.
func NewHelpers() *Helpers { return &Helpers{runner: subproc.NewRunner()} }

// NewHelpersWithRunner is the test seam.
func NewHelpersWithRunner(r subproc.Runner) *Helpers { return &Helpers{runner: r} }

// HasDirtyFiles returns true if there are any uncommitted changes in the working tree.
func (h *Helpers) HasDirtyFiles(ctx context.Context) (bool, error) {
	output, err := h.runner.RunWithWarnAndTimeout(
		ctx,
		"git status --porcelain",
		"git",
		"status",
		"--porcelain",
	)
	if err != nil {
		return false, errors.Wrapf(ctx, err, "git status: %s", stderrFromErr(err))
	}
	return len(strings.TrimSpace(string(output))) > 0, nil
}

// CommitCompletedFile stages and commits a completed prompt file.
func (h *Helpers) CommitCompletedFile(ctx context.Context, path string) error {
	// Stage only the specified file
	addOut, err := h.runner.RunWithWarnAndTimeout(ctx, "git add", "git", "add", path)
	if err != nil {
		return errors.Wrapf(ctx, err, "git add: %s", stderrFromErr(err))
	}
	if s := strings.TrimSpace(string(addOut)); s != "" {
		slog.Debug("git output", "op", "add-completed-file", "output", s)
	}

	// Check if there's anything to commit
	statusOut, err := h.runner.RunWithWarnAndTimeout(
		ctx,
		"git status --porcelain",
		"git",
		"status",
		"--porcelain",
	)
	if err != nil {
		return errors.Wrapf(ctx, err, "git status: %s", stderrFromErr(err))
	}
	if len(strings.TrimSpace(string(statusOut))) == 0 {
		return nil
	}

	// Commit
	commitOut, err := h.runner.RunWithWarnAndTimeout(
		ctx,
		"git commit",
		"git",
		"commit",
		"-m",
		"move prompt to completed",
	)
	if err != nil {
		return errors.Wrapf(ctx, err, "git commit: %s", stderrFromErr(err))
	}
	if s := strings.TrimSpace(string(commitOut)); s != "" {
		slog.Debug("git output", "op", "commit-completed-file", "output", s)
	}
	return nil
}

// MoveFile moves a file using git mv to preserve history.
// Falls back to os.Rename if git operations fail or not in a git repo.
func (h *Helpers) MoveFile(ctx context.Context, oldPath string, newPath string) error {
	_, err := h.runner.RunWithWarnAndTimeout(ctx, "git mv", "git", "mv", oldPath, newPath)
	if err != nil {
		slog.Debug("git mv failed, falling back to os.Rename", "stderr", stderrFromErr(err))
		return fallbackRename(ctx, oldPath, newPath)
	}
	return nil
}

// gitAddAll stages all changes.
func (h *Helpers) gitAddAll(ctx context.Context) error {
	out, err := h.runner.RunWithWarnAndTimeout(ctx, "git add -A", "git", "add", "-A")
	if err != nil {
		return errors.Wrapf(ctx, err, "git add all: %s", stderrFromErr(err))
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		slog.Debug("git output", "op", "add-all", "output", s)
	}
	return nil
}

// stageAllAndCheck stages all changes and reports whether anything was staged.
func (h *Helpers) stageAllAndCheck(ctx context.Context) (bool, error) {
	if err := h.gitAddAll(ctx); err != nil {
		return false, errors.Wrap(ctx, err, "git add")
	}
	output, err := h.runner.RunWithWarnAndTimeout(
		ctx,
		"git status --porcelain",
		"git",
		"status",
		"--porcelain",
	)
	if err != nil {
		return false, errors.Wrapf(ctx, err, "git status: %s", stderrFromErr(err))
	}
	return len(strings.TrimSpace(string(output))) > 0, nil
}

// getNextVersion determines the next version based on the bump type.
func (h *Helpers) getNextVersion(ctx context.Context, bump VersionBump) (string, error) {
	out, err := h.runner.RunWithWarnAndTimeout(
		ctx,
		"git tag --list v*",
		"git",
		"tag",
		"--list",
		"v*",
	)
	if err != nil {
		return "", errors.Wrapf(ctx, err, "list git tags: %s", stderrFromErr(err))
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	versions := make([]SemanticVersionNumber, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		version, parseErr := ParseSemanticVersionNumber(ctx, line)
		if parseErr != nil {
			continue
		}
		versions = append(versions, version)
	}

	var maxTagVersion *SemanticVersionNumber
	if len(versions) > 0 {
		mv := versions[0]
		for _, v := range versions[1:] {
			if mv.Less(v) {
				mv = v
			}
		}
		maxTagVersion = &mv
	}

	changelogVersion, _ := latestVersionFromChangelog(ctx)

	var base SemanticVersionNumber
	switch {
	case maxTagVersion == nil && changelogVersion == nil:
		return "v0.1.0", nil
	case maxTagVersion == nil:
		base = *changelogVersion
	case changelogVersion == nil:
		base = *maxTagVersion
	case maxTagVersion.Less(*changelogVersion):
		slog.Warn(
			"changelog has orphan version above highest tag; bumping from changelog to avoid semver regression",
			"orphan_version",
			changelogVersion.String(),
			"highest_tag",
			maxTagVersion.String(),
		)
		base = *changelogVersion
	default:
		base = *maxTagVersion
	}

	var nextVersion SemanticVersionNumber
	switch bump {
	case MinorBump:
		nextVersion = base.BumpMinor()
	case PatchBump:
		nextVersion = base.BumpPatch()
	default:
		nextVersion = base.BumpPatch()
	}
	return nextVersion.String(), nil
}

// gitCommit creates a commit with the given message.
func (h *Helpers) gitCommit(ctx context.Context, message string) error {
	slog.Debug("creating commit", "message", message)
	out, err := h.runner.RunWithWarnAndTimeout(ctx, "git commit", "git", "commit", "-m", message)
	if err != nil {
		return errors.Wrapf(ctx, err, "create commit: %s", stderrFromErr(err))
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		slog.Debug("git output", "op", "commit", "output", s)
	}
	return nil
}

// gitTag creates a tag.
func (h *Helpers) gitTag(ctx context.Context, tag string) error {
	if _, err := ParseSemanticVersionNumber(ctx, tag); err != nil {
		return errors.Wrap(ctx, err, "invalid tag format")
	}
	slog.Debug("creating tag", "tag", tag)
	out, err := h.runner.RunWithWarnAndTimeout(ctx, "git tag", "git", "tag", tag)
	if err != nil {
		return errors.Wrapf(ctx, err, "create tag: %s", stderrFromErr(err))
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		slog.Debug("git output", "op", "tag", "output", s)
	}
	return nil
}

// gitPush pushes commits to remote.
func (h *Helpers) gitPush(ctx context.Context) error {
	slog.Debug("pushing commits to remote")
	out, err := h.runner.RunWithWarnAndTimeout(ctx, "git push", "git", "push")
	if err != nil {
		return errors.Wrapf(ctx, err, "push to remote: %s", stderrFromErr(err))
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		slog.Debug("git output", "op", "push", "output", s)
	}
	return nil
}

// gitPushTag pushes a tag to remote.
func (h *Helpers) gitPushTag(ctx context.Context, tag string) error {
	if _, err := ParseSemanticVersionNumber(ctx, tag); err != nil {
		return errors.Wrap(ctx, err, "invalid tag format")
	}
	slog.Debug("pushing tag to remote", "tag", tag)
	out, err := h.runner.RunWithWarnAndTimeout(ctx, "git push tag", "git", "push", "origin", tag)
	if err != nil {
		return errors.Wrapf(ctx, err, "push tag to remote: %s", stderrFromErr(err))
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		slog.Debug("git output", "op", "push-tag", "output", s)
	}
	return nil
}

// ResolveGitRoot returns the absolute path to the root of the current git repository.
func (h *Helpers) ResolveGitRoot(ctx context.Context) (string, error) {
	out, err := h.runner.RunWithWarnAndTimeout(
		ctx,
		"git rev-parse --show-toplevel",
		"git",
		"rev-parse",
		"--show-toplevel",
	)
	if err != nil {
		return "", errors.New(
			ctx,
			"not inside a git repository (run dark-factory from inside a git repo)",
		)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", errors.New(ctx, "git rev-parse --show-toplevel returned empty output")
	}
	return root, nil
}

// CommitAndRelease performs the full git workflow.
func (h *Helpers) CommitAndRelease(ctx context.Context, bump VersionBump) error {
	has, err := h.stageAllAndCheck(ctx)
	if err != nil {
		return err
	}
	if !has {
		slog.Info("no staged changes — skipping release")
		return nil
	}

	nextVersion, err := h.getNextVersion(ctx, bump)
	if err != nil {
		return errors.Wrap(ctx, err, "get next version")
	}

	if err := updateChangelog(ctx, nextVersion); err != nil {
		return errors.Wrap(ctx, err, "update changelog")
	}

	if err := h.gitAddAll(ctx); err != nil {
		return errors.Wrap(ctx, err, "git add changelog")
	}

	commitMsg := "release " + nextVersion
	if err := h.gitCommit(ctx, commitMsg); err != nil {
		return errors.Wrap(ctx, err, "git commit")
	}

	if err := h.gitTag(ctx, nextVersion); err != nil {
		return errors.Wrap(ctx, err, "git tag")
	}

	if err := h.gitPush(ctx); err != nil {
		return errors.Wrap(ctx, err, "git push")
	}

	if err := h.gitPushTag(ctx, nextVersion); err != nil {
		return errors.Wrap(ctx, err, "git push tag")
	}

	return nil
}

// CommitAll stages all changes and commits with the given message.
func (h *Helpers) CommitAll(ctx context.Context, message string) error {
	has, err := h.stageAllAndCheck(ctx)
	if err != nil {
		return err
	}
	if !has {
		return nil
	}
	return h.gitCommit(ctx, message)
}
