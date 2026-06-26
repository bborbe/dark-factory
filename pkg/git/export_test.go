// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/subproc"
)

// DecideMergeActionForTest exposes the internal decideMergeAction function for external tests.
func DecideMergeActionForTest(mergeStateStatus string) (shouldMerge bool, err error) {
	return decideMergeAction(mergeStateStatus)
}

// TruncateStderrForTest exposes truncateStderr for external tests.
func TruncateStderrForTest(s string) string {
	return truncateStderr(s)
}

// NewClonerWithRunnerForTest exposes newClonerWithRunner for external tests.
func NewClonerWithRunnerForTest(r subproc.Runner) Cloner { return newClonerWithRunner(r) }

// NewBrancherWithRunnerForTest creates a Brancher with an injected runner for external tests.
func NewBrancherWithRunnerForTest(r subproc.Runner) Brancher {
	return NewBrancher(withBrancherRunner(r))
}

// NewWorktreerWithRunnerForTest exposes newWorktreerWithRunner for external tests.
func NewWorktreerWithRunnerForTest(r subproc.Runner) Worktreer { return newWorktreerWithRunner(r) }

// NewReleaserWithRunnerForTest exposes newReleaserWithRunner for external tests.
func NewReleaserWithRunnerForTest(r subproc.Runner) Releaser { return newReleaserWithRunner(r) }

// NewGHRepoNameFetcherWithRunnerForTest exposes newGHRepoNameFetcherWithRunner for external tests.
func NewGHRepoNameFetcherWithRunnerForTest(ghToken string, r subproc.Runner) RepoNameFetcher {
	return newGHRepoNameFetcherWithRunner(ghToken, r)
}

// NewGHCollaboratorListerWithRunnerForTest exposes newGHCollaboratorListerWithRunner for external tests.
func NewGHCollaboratorListerWithRunnerForTest(ghToken string, r subproc.Runner) CollaboratorLister {
	return newGHCollaboratorListerWithRunner(ghToken, r)
}

// NewPRMergerWithRunnerForTest exposes newPRMergerWithRunner for external tests.
func NewPRMergerWithRunnerForTest(
	ghToken string,
	cdt libtime.CurrentDateTimeGetter,
	r subproc.Runner,
) PRMerger {
	return newPRMergerWithRunner(ghToken, cdt, r)
}
