// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"context"
	"os/exec"
	"strings"

	"github.com/bborbe/errors"
)

// ResolveGitRoot returns the absolute path to the root of the current git repository
// by running `git rev-parse --show-toplevel`. Returns an error if not inside a git repo.
func ResolveGitRoot(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
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
