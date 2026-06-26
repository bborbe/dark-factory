// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import "context"

// ResolveGitRoot returns the absolute path to the root of the current git repository
// by running `git rev-parse --show-toplevel`. Returns an error if not inside a git repo.
func ResolveGitRoot(ctx context.Context) (string, error) {
	return NewHelpers().ResolveGitRoot(ctx)
}
