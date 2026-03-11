// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"os/exec"
	"regexp"
	"strings"

	"github.com/bborbe/errors"
)

// BitbucketRemoteCoords holds the project key and repo slug parsed from a Bitbucket Server remote URL.
type BitbucketRemoteCoords struct {
	// Project is the Bitbucket project key (uppercased, e.g. "BRO").
	Project string
	// Repo is the repository slug (lowercased, e.g. "sentinel").
	Repo string
}

// ParseBitbucketRemoteURL parses a Bitbucket Server git remote URL and returns project key and repo slug.
//
// Supported formats:
//   - SSH:   ssh://bitbucket.example.com:7999/bro/sentinel.git
//   - HTTPS: https://bitbucket.example.com/scm/bro/sentinel.git
//
// The project key is uppercased and the repo slug is lowercased (matching Bitbucket Server conventions).
func ParseBitbucketRemoteURL(
	ctx context.Context,
	remoteURL string,
) (*BitbucketRemoteCoords, error) {
	// SSH format: ssh://host:port/project/repo.git
	sshRe := regexp.MustCompile(`^ssh://[^/]+/([^/]+)/([^/]+?)(?:\.git)?$`)
	if m := sshRe.FindStringSubmatch(remoteURL); m != nil {
		return &BitbucketRemoteCoords{
			Project: strings.ToUpper(m[1]),
			Repo:    strings.ToLower(strings.TrimSuffix(m[2], ".git")),
		}, nil
	}

	// HTTPS format: https://host/scm/project/repo.git
	httpsRe := regexp.MustCompile(`^https?://[^/]+/scm/([^/]+)/([^/]+?)(?:\.git)?$`)
	if m := httpsRe.FindStringSubmatch(remoteURL); m != nil {
		return &BitbucketRemoteCoords{
			Project: strings.ToUpper(m[1]),
			Repo:    strings.ToLower(strings.TrimSuffix(m[2], ".git")),
		}, nil
	}

	return nil, errors.Errorf(
		ctx,
		"unrecognized Bitbucket Server remote URL format: %q (expected ssh://host:port/project/repo.git or https://host/scm/project/repo.git)",
		remoteURL,
	)
}

// ParseBitbucketRemoteFromGit reads the git remote URL for the given remote name and parses it.
// Uses `git remote get-url <remoteName>` to fetch the URL.
func ParseBitbucketRemoteFromGit(
	ctx context.Context,
	remoteName string,
) (*BitbucketRemoteCoords, error) {
	// #nosec G204 -- remoteName is a fixed string ("origin"), not user input
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", remoteName)
	output, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrap(ctx, err, "get git remote url")
	}
	remoteURL := strings.TrimSpace(string(output))
	return ParseBitbucketRemoteURL(ctx, remoteURL)
}
