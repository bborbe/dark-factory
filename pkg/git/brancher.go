// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/subproc"
)

//counterfeiter:generate -o ../../mocks/brancher.go --fake-name Brancher . Brancher

// Brancher handles git branch operations.
type Brancher interface {
	CreateAndSwitch(ctx context.Context, name string) error
	Push(ctx context.Context, name string) error
	Switch(ctx context.Context, name string) error
	CurrentBranch(ctx context.Context) (string, error)
	Fetch(ctx context.Context) error
	FetchAndVerifyBranch(ctx context.Context, branch string) error
	DefaultBranch(ctx context.Context) (string, error)
	Pull(ctx context.Context) error
	MergeOriginDefault(ctx context.Context) error
	IsClean(ctx context.Context) (bool, error)
	// IsCleanIgnoring returns the dirty paths that are NOT covered by any of
	// the given ignorePrefixes. An empty slice means the tree is effectively
	// clean for the caller's purposes. The prefix comparison is
	// strings.HasPrefix(path, prefix) on the path as reported by
	// git status --porcelain (relative, no leading slash).
	// A non-nil error means the git command itself failed.
	IsCleanIgnoring(ctx context.Context, ignorePrefixes []string) ([]string, error)
	// DiscardUncommittedInPaths restores each path prefix to its HEAD state using
	// git checkout HEAD -- <prefix>. Prefixes not covered by any uncommitted change
	// are silently skipped. An empty prefixes slice is a no-op.
	DiscardUncommittedInPaths(ctx context.Context, prefixes []string) error
	MergeToDefault(ctx context.Context, branch string) error
	CommitsAhead(ctx context.Context, branch string) (int, error)
	// FetchBranch fetches the named branch from origin as a local branch
	// (git fetch origin <branch>:<branch>).
	FetchBranch(ctx context.Context, branch string) error
}

// BrancherOption is a functional option for configuring a brancher.
type BrancherOption func(*brancher)

// WithDefaultBranch sets a configured default branch on the brancher.
func WithDefaultBranch(branch string) BrancherOption {
	return func(b *brancher) {
		if branch != "" {
			b.configuredDefaultBranch = branch
		}
	}
}

// withBrancherRunner is an unexported option for injecting a runner (tests).
func withBrancherRunner(r subproc.Runner) BrancherOption {
	return func(b *brancher) {
		b.runner = r
	}
}

// brancher implements Brancher.
type brancher struct {
	configuredDefaultBranch string
	runner                  subproc.Runner
}

// NewBrancher creates a new Brancher.
func NewBrancher(opts ...BrancherOption) Brancher {
	b := &brancher{runner: subproc.NewRunner()}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// CreateAndSwitch creates a new branch and switches to it.
func (b *brancher) CreateAndSwitch(ctx context.Context, name string) error {
	if err := ValidateBranchName(ctx, name); err != nil {
		return errors.Wrap(ctx, err, "validate branch name")
	}
	slog.Debug("creating and switching to branch", "branch", name)
	out, err := b.runner.RunWithWarnAndTimeout(
		ctx,
		"git checkout -b",
		"git",
		"checkout",
		"-b",
		name,
	)
	if err != nil {
		return errors.Wrapf(
			ctx,
			err,
			"create and switch to branch: %s",
			stderrFromErr(err),
		)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		slog.Debug("git output", "op", "create-and-switch", "output", s)
	}
	return nil
}

// Push pushes a branch to the remote repository.
func (b *brancher) Push(ctx context.Context, name string) error {
	if err := ValidateBranchName(ctx, name); err != nil {
		return errors.Wrap(ctx, err, "validate branch name")
	}
	slog.Debug("pushing branch to remote", "branch", name)
	out, err := b.runner.RunWithWarnAndTimeout(
		ctx,
		"git push -u origin",
		"git",
		"push",
		"-u",
		"origin",
		name,
	)
	if err != nil {
		return errors.Wrapf(
			ctx,
			err,
			"push branch to remote: %s",
			stderrFromErr(err),
		)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		slog.Debug("git output", "op", "push-branch", "output", s)
	}
	return nil
}

// Switch switches to an existing branch.
func (b *brancher) Switch(ctx context.Context, name string) error {
	if err := ValidateBranchName(ctx, name); err != nil {
		return errors.Wrap(ctx, err, "validate branch name")
	}
	slog.Debug("switching to branch", "branch", name)
	out, err := b.runner.RunWithWarnAndTimeout(ctx, "git checkout", "git", "checkout", name)
	if err != nil {
		return errors.Wrapf(ctx, err, "switch to branch: %s", stderrFromErr(err))
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		slog.Debug("git output", "op", "switch-branch", "output", s)
	}
	return nil
}

// CurrentBranch returns the name of the current branch.
func (b *brancher) CurrentBranch(ctx context.Context) (string, error) {
	output, err := b.runner.RunWithWarnAndTimeout(
		ctx,
		"git rev-parse --abbrev-ref HEAD",
		"git",
		"rev-parse",
		"--abbrev-ref",
		"HEAD",
	)
	if err != nil {
		return "", errors.Wrapf(
			ctx,
			err,
			"get current branch: %s",
			stderrFromErr(err),
		)
	}
	branch := strings.TrimSpace(string(output))
	slog.Debug("current branch", "branch", branch)
	return branch, nil
}

// Fetch fetches updates from the remote repository.
func (b *brancher) Fetch(ctx context.Context) error {
	slog.Debug("fetching from origin")
	out, err := b.runner.RunWithWarnAndTimeout(ctx, "git fetch origin", "git", "fetch", "origin")
	if err != nil {
		return errors.Wrapf(ctx, err, "fetch from origin: %s", stderrFromErr(err))
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		slog.Debug("git output", "op", "fetch", "output", s)
	}
	return nil
}

// FetchAndVerifyBranch fetches from origin and verifies the branch exists remotely.
func (b *brancher) FetchAndVerifyBranch(ctx context.Context, branch string) error {
	if err := ValidateBranchName(ctx, branch); err != nil {
		return errors.Wrap(ctx, err, "validate branch name")
	}
	slog.Debug("fetching from origin and verifying branch", "branch", branch)
	out, err := b.runner.RunWithWarnAndTimeout(ctx, "git fetch origin", "git", "fetch", "origin")
	if err != nil {
		return errors.Wrapf(ctx, err, "fetch from origin: %s", stderrFromErr(err))
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		slog.Debug("git output", "op", "fetch-and-verify-branch", "output", s)
	}

	_, err = b.runner.RunWithWarnAndTimeout(
		ctx,
		"git rev-parse --verify",
		"git",
		"rev-parse",
		"--verify",
		"origin/"+branch,
	)
	if err != nil {
		return errors.Errorf(ctx, "branch not found at origin: %s", branch)
	}
	return nil
}

// DefaultBranch returns the repository's default branch name.
func (b *brancher) DefaultBranch(ctx context.Context) (string, error) {
	if b.configuredDefaultBranch != "" {
		slog.Debug("default branch from config", "branch", b.configuredDefaultBranch)
		return b.configuredDefaultBranch, nil
	}
	output, err := b.runner.RunWithWarnAndTimeout(
		ctx,
		"gh repo view --json defaultBranchRef",
		"gh",
		"repo",
		"view",
		"--json",
		"defaultBranchRef",
		"--jq",
		".defaultBranchRef.name",
	)
	if err != nil {
		if branch := b.defaultBranchFromSymbolicRef(ctx); branch != "" {
			return branch, nil
		}
		return "", errors.Wrap(ctx, err, "get default branch")
	}
	branch := strings.TrimSpace(string(output))
	if branch == "" {
		return "", errors.Errorf(ctx, "default branch is empty")
	}
	slog.Debug("default branch", "branch", branch)
	return branch, nil
}

// defaultBranchFromSymbolicRef tries to determine the default branch using
// git symbolic-ref refs/remotes/origin/HEAD.
func (b *brancher) defaultBranchFromSymbolicRef(ctx context.Context) string {
	output, err := b.runner.RunWithWarnAndTimeout(
		ctx,
		"git symbolic-ref",
		"git",
		"symbolic-ref",
		"refs/remotes/origin/HEAD",
	)
	if err != nil {
		return ""
	}
	const prefix = "refs/remotes/origin/"
	ref := strings.TrimSpace(string(output))
	if !strings.HasPrefix(ref, prefix) {
		return ""
	}
	branch := strings.TrimPrefix(ref, prefix)
	if branch == "" {
		return ""
	}
	slog.Debug("default branch from git symbolic-ref", "branch", branch)
	return branch
}

// Pull runs git pull on the current branch.
func (b *brancher) Pull(ctx context.Context) error {
	slog.Debug("pulling current branch")
	out, err := b.runner.RunWithWarnAndTimeout(ctx, "git pull", "git", "pull")
	if err != nil {
		return errors.Wrapf(ctx, err, "pull current branch: %s", stderrFromErr(err))
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		slog.Debug("git output", "op", "pull", "output", s)
	}
	return nil
}

// IsClean returns true if the working tree has no uncommitted changes.
func (b *brancher) IsClean(ctx context.Context) (bool, error) {
	output, err := b.runner.RunWithWarnAndTimeout(
		ctx,
		"git status --porcelain",
		"git",
		"status",
		"--porcelain",
	)
	if err != nil {
		return false, errors.Wrapf(
			ctx,
			err,
			"check working tree status: %s",
			stderrFromErr(err),
		)
	}
	return strings.TrimSpace(string(output)) == "", nil
}

// IsCleanIgnoring returns dirty paths not covered by any ignorePrefixes.
func (b *brancher) IsCleanIgnoring(ctx context.Context, ignorePrefixes []string) ([]string, error) {
	output, err := b.runner.RunWithWarnAndTimeout(
		ctx,
		"git status --porcelain -z",
		"git",
		"status",
		"--porcelain",
		"-z",
	)
	if err != nil {
		return nil, errors.Wrapf(
			ctx,
			err,
			"check working tree status: %s",
			stderrFromErr(err),
		)
	}
	var dirty []string
	// -z output: each entry is "XY <path>\0". Renamed entries (status R or C)
	// append a second NUL-separated record holding the old path; we ignore the
	// old-path follow-up records by only examining entries with a status prefix.
	skipNext := false
	for _, entry := range strings.Split(strings.TrimRight(string(output), "\x00"), "\x00") {
		if skipNext {
			skipNext = false
			continue
		}
		if len(entry) < 4 {
			continue
		}
		statusXY := entry[:2]
		path := entry[3:]
		if statusXY[0] == 'R' || statusXY[0] == 'C' {
			skipNext = true
		}
		ignored := false
		for _, prefix := range ignorePrefixes {
			if prefix != "" && strings.HasPrefix(path, prefix) {
				ignored = true
				break
			}
		}
		if !ignored {
			dirty = append(dirty, path)
		}
	}
	return dirty, nil
}

// DiscardUncommittedInPaths restores each path prefix to HEAD state.
func (b *brancher) DiscardUncommittedInPaths(ctx context.Context, prefixes []string) error {
	for _, prefix := range prefixes {
		if prefix == "" {
			continue
		}
		// Force English locale so the "did not match any file" probe below is locale-stable.
		_, err := b.runner.RunWithWarnAndTimeoutEnv(
			ctx,
			"git checkout HEAD --",
			"",
			[]string{"LC_ALL=C", "LANG=C"},
			"git", "checkout", "HEAD", "--", prefix,
		)
		if err != nil {
			msg := stderrFromErr(err)
			if strings.Contains(msg, "did not match any file") {
				slog.Debug("DiscardUncommittedInPaths: prefix matched no tracked files, skipping",
					"prefix", prefix)
				continue
			}
			return errors.Wrapf(ctx, err, "discard uncommitted changes in %q: %s", prefix, msg)
		}
		slog.Debug("DiscardUncommittedInPaths: discarded bookkeeping dirt", "prefix", prefix)
	}
	return nil
}

// MergeOriginDefault merges the remote default branch into the current branch.
func (b *brancher) MergeOriginDefault(ctx context.Context) error {
	defaultBranch, err := b.DefaultBranch(ctx)
	if err != nil {
		slog.Warn("skipping merge origin default: could not determine default branch", "error", err)
		return nil
	}
	slog.Debug("merging origin default branch", "branch", defaultBranch)
	out, err := b.runner.RunWithWarnAndTimeout(
		ctx,
		"git merge origin/"+defaultBranch,
		"git",
		"merge",
		"origin/"+defaultBranch,
	)
	if err != nil {
		return errors.Wrapf(
			ctx,
			err,
			"merge origin/%s: %s",
			defaultBranch,
			stderrFromErr(err),
		)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		slog.Debug("git output", "op", "merge-origin-default", "output", s)
	}
	return nil
}

// MergeToDefault merges the given feature branch into the default branch.
func (b *brancher) MergeToDefault(ctx context.Context, branch string) error {
	if err := ValidateBranchName(ctx, branch); err != nil {
		return errors.Wrap(ctx, err, "validate branch name")
	}
	defaultBranch, err := b.DefaultBranch(ctx)
	if err != nil {
		return errors.Wrap(ctx, err, "get default branch")
	}
	_, err = b.runner.RunWithWarnAndTimeout(
		ctx,
		"git checkout "+defaultBranch,
		"git",
		"checkout",
		defaultBranch,
	)
	if err != nil {
		return errors.Wrapf(
			ctx,
			err,
			"switch to default branch before merge: %s",
			stderrFromErr(err),
		)
	}
	_, err = b.runner.RunWithWarnAndTimeout(
		ctx,
		"git merge --no-ff "+branch,
		"git",
		"merge",
		"--no-ff",
		branch,
	)
	if err != nil {
		return errors.Wrapf(
			ctx,
			err,
			"merge branch %q to default: %s",
			branch,
			stderrFromErr(err),
		)
	}
	slog.Info("merged feature branch to default", "branch", branch, "default", defaultBranch)
	return nil
}

// FetchBranch fetches the named branch from origin into a local branch of the same name.
func (b *brancher) FetchBranch(ctx context.Context, branch string) error {
	if err := ValidateBranchName(ctx, branch); err != nil {
		return errors.Wrap(ctx, err, "validate branch name")
	}
	slog.Debug("fetching branch from origin as local ref", "branch", branch)
	_, err := b.runner.RunWithWarnAndTimeout(
		ctx,
		"git fetch origin "+branch,
		"git",
		"fetch",
		"origin",
		branch+":"+branch,
	)
	if err != nil {
		msg := stderrFromErr(err)
		if strings.Contains(msg, "couldn't find remote ref") {
			slog.Debug("FetchBranch: branch not on origin yet, skipping", "branch", branch)
			return nil
		}
		return errors.Wrapf(
			ctx,
			err,
			"fetch branch %q from origin: %s",
			branch,
			msg,
		)
	}
	slog.Debug("FetchBranch: fetched branch as local ref", "branch", branch)
	return nil
}

// CommitsAhead returns the number of commits on branch ahead of the default branch.
func (b *brancher) CommitsAhead(ctx context.Context, branch string) (int, error) {
	if err := ValidateBranchName(ctx, branch); err != nil {
		return 0, errors.Wrap(ctx, err, "validate branch name")
	}
	defaultBranch, err := b.DefaultBranch(ctx)
	if err != nil {
		return 0, errors.Wrap(ctx, err, "get default branch for commit count")
	}
	output, err := b.runner.RunWithWarnAndTimeout(
		ctx,
		"git rev-list --count",
		"git",
		"rev-list",
		"--count",
		"origin/"+defaultBranch+".."+branch,
	)
	if err != nil {
		return 0, errors.Wrapf(
			ctx,
			err,
			"count commits ahead: %s",
			stderrFromErr(err),
		)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, errors.Wrap(ctx, err, "parse commit count")
	}
	return count, nil
}
