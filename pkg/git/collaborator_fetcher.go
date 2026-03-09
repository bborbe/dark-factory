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
)

//counterfeiter:generate -o ../../mocks/collaborator-fetcher.go --fake-name CollaboratorFetcher . CollaboratorFetcher

// CollaboratorFetcher fetches GitHub repository collaborators.
type CollaboratorFetcher interface {
	Fetch(ctx context.Context) []string
}

type collaboratorFetcher struct {
	ghToken          string
	useCollaborators bool
	allowedReviewers []string
}

// NewCollaboratorFetcher creates a CollaboratorFetcher.
// If useCollaborators is false or allowedReviewers is non-empty, collaborators are not fetched from GitHub.
func NewCollaboratorFetcher(
	ghToken string,
	useCollaborators bool,
	allowedReviewers []string,
) CollaboratorFetcher {
	return &collaboratorFetcher{
		ghToken:          ghToken,
		useCollaborators: useCollaborators,
		allowedReviewers: allowedReviewers,
	}
}

func (f *collaboratorFetcher) Fetch(ctx context.Context) []string {
	if len(f.allowedReviewers) > 0 {
		return f.allowedReviewers
	}
	if !f.useCollaborators {
		return nil
	}

	// Get the repo name with owner
	nameCmd := exec.CommandContext( //nolint:gosec
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
		nameCmd.Env = append(os.Environ(), "GH_TOKEN="+f.ghToken)
	}
	nameOut, err := nameCmd.Output()
	if err != nil {
		slog.Warn("failed to get repo name for collaborators", "error", err)
		return nil
	}
	repoName := strings.TrimSpace(string(nameOut))

	// Fetch collaborator logins
	collabCmd := exec.CommandContext( //nolint:gosec
		ctx,
		"gh",
		"api",
		"repos/"+repoName+"/collaborators",
		"--jq",
		".[].login",
	) // #nosec G204 -- repoName from gh CLI, not user input
	if f.ghToken != "" {
		collabCmd.Env = append(os.Environ(), "GH_TOKEN="+f.ghToken)
	}
	collabOut, err := collabCmd.Output()
	if err != nil {
		slog.Warn("failed to fetch collaborators", "error", err)
		return nil
	}

	var result []string
	for _, line := range strings.Split(strings.TrimSpace(string(collabOut)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			result = append(result, line)
		}
	}
	return result
}
