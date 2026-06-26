// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import (
	"os/exec"
	"strings"

	"github.com/bborbe/errors"
)

// maxStderrBytes is the maximum number of bytes captured from git stderr before
// truncation. 8 KiB is enough to surface the actionable first lines of any git
// error (conflict lists, auth failures, refname warnings) while keeping log
// lines bounded. Operators can always re-run the failed command manually if
// they need the full output.
const maxStderrBytes = 8192

// TruncateStderr returns s unchanged if len(s) <= maxStderrBytes.
// If s exceeds the limit, the first maxStderrBytes bytes are returned followed
// by the literal string " (truncated)". Exported so sibling git-provider
// packages (e.g. pkg/gitprovider/bitbucket) can use it without duplicating.
func TruncateStderr(s string) string {
	if len(s) <= maxStderrBytes {
		return strings.TrimRight(s, "\n")
	}
	return strings.TrimRight(s[:maxStderrBytes], "\n") + " (truncated)"
}

// truncateStderr is the unexported alias preserved for backward-compat with
// pkg/git internal callers; new callers should use TruncateStderr.
func truncateStderr(s string) string { return TruncateStderr(s) }

// stderrFromErr extracts and truncates the child stderr captured in a wrapped
// *exec.ExitError (populated by cmd.Output() inside subproc.Runner). Returns ""
// when err carries no *exec.ExitError.
func stderrFromErr(err error) string {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return TruncateStderr(string(exitErr.Stderr))
	}
	return ""
}

// StderrFromError extracts and truncates the child stderr captured in a wrapped
// *exec.ExitError. Exported so sibling packages (e.g. pkg/gitprovider/bitbucket)
// can use it without duplicating the logic.
func StderrFromError(err error) string { return stderrFromErr(err) }
