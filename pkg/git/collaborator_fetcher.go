// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/bborbe/errors"
)

//counterfeiter:generate -o ../../mocks/collaborator-fetcher.go --fake-name CollaboratorFetcher . CollaboratorFetcher

// CollaboratorFetcher fetches GitHub repository collaborators.
type CollaboratorFetcher interface {
	Fetch(ctx context.Context) []string
}

//counterfeiter:generate -o ../../mocks/repo-name-fetcher.go --fake-name RepoNameFetcher . RepoNameFetcher

// RepoNameFetcher fetches the current GitHub repository name with owner.
type RepoNameFetcher interface {
	Fetch(ctx context.Context) (string, error)
}

//counterfeiter:generate -o ../../mocks/collaborator-lister.go --fake-name CollaboratorLister . CollaboratorLister

// CollaboratorLister lists collaborator logins for a GitHub repository.
type CollaboratorLister interface {
	List(ctx context.Context, repoName string) ([]string, error)
}

// NewCollaboratorFetcher creates a CollaboratorFetcher.
// If useCollaborators is false or allowedReviewers is non-empty, collaborators are not fetched from GitHub.
func NewCollaboratorFetcher(
	repoNameFetcher RepoNameFetcher,
	collaboratorLister CollaboratorLister,
	useCollaborators bool,
	allowedReviewers []string,
) CollaboratorFetcher {
	return &collaboratorFetcher{
		repoNameFetcher:  repoNameFetcher,
		collaboratorList: collaboratorLister,
		useCollaborators: useCollaborators,
		allowedReviewers: allowedReviewers,
	}
}

type collaboratorFetcher struct {
	repoNameFetcher  RepoNameFetcher
	collaboratorList CollaboratorLister
	useCollaborators bool
	allowedReviewers []string
}

func (f *collaboratorFetcher) Fetch(ctx context.Context) []string {
	if len(f.allowedReviewers) > 0 {
		return f.allowedReviewers
	}
	if !f.useCollaborators {
		return nil
	}

	repoName, err := f.repoNameFetcher.Fetch(ctx)
	if err != nil {
		return nil
	}

	collaborators, err := f.collaboratorList.List(ctx, repoName)
	if err != nil {
		return nil
	}
	return collaborators
}

// NewGHRepoNameFetcher creates a RepoNameFetcher that uses gh CLI.
func NewGHRepoNameFetcher(ghToken string) RepoNameFetcher {
	return &ghRepoNameFetcher{ghToken: ghToken}
}

// ghRepoNameFetcher fetches repo name using gh CLI.
type ghRepoNameFetcher struct {
	ghToken string
}

func (f *ghRepoNameFetcher) Fetch(ctx context.Context) (string, error) {
	cmd := exec.CommandContext( //nolint:gosec
		ctx,
		"gh",
		"repo",
		"view",
		"--json",
		"nameWithOwner",
		"--jq",
		".nameWithOwner",
	) // #nosec G204 -- fixed args, no user input
	if f.ghToken != "" {
		cmd.Env = append(os.Environ(), "GH_TOKEN="+f.ghToken)
	}
	out, err := cmd.Output()
	if err != nil {
		return "", errors.Wrap(ctx, err, "gh repo view")
	}
	return strings.TrimSpace(string(out)), nil
}

// NewGHCollaboratorLister creates a CollaboratorLister that uses gh CLI.
func NewGHCollaboratorLister(ghToken string) CollaboratorLister {
	return &ghCollaboratorLister{ghToken: ghToken}
}

// ghCollaboratorLister fetches collaborators using gh CLI.
type ghCollaboratorLister struct {
	ghToken string
}

func (l *ghCollaboratorLister) List(ctx context.Context, repoName string) ([]string, error) {
	cmd := exec.CommandContext( //nolint:gosec
		ctx,
		"gh",
		"api",
		"repos/"+repoName+"/collaborators",
		"--jq",
		".[].login",
	) // #nosec G204 -- repoName from gh CLI, not user input
	if l.ghToken != "" {
		cmd.Env = append(os.Environ(), "GH_TOKEN="+l.ghToken)
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrap(ctx, err, "gh api collaborators")
	}

	var result []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			result = append(result, line)
		}
	}
	return result, nil
}
