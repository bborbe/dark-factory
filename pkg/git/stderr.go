// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package git

import "strings"

// maxStderrBytes is the maximum number of bytes captured from git stderr before
// truncation. 8 KiB is enough to surface the actionable first lines of any git
// error (conflict lists, auth failures, refname warnings) while keeping log
// lines bounded. Operators can always re-run the failed command manually if
// they need the full output.
const maxStderrBytes = 8192

// truncateStderr returns s unchanged if len(s) <= maxStderrBytes.
// If s exceeds the limit, the first maxStderrBytes bytes are returned followed
// by the literal string " (truncated)".
func truncateStderr(s string) string {
	if len(s) <= maxStderrBytes {
		return strings.TrimRight(s, "\n")
	}
	return strings.TrimRight(s[:maxStderrBytes], "\n") + " (truncated)"
}
