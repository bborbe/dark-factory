// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package report

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/bborbe/errors"
)

const headBytes = 64 * 1024 // 64 KiB

// criticalFailurePatterns are the lowercase strings that indicate the claude CLI itself failed.
var criticalFailurePatterns = []string{
	"failed to authenticate",
	`"type":"authentication_error"`,
	"api error: 401",
	"api error: 403",
	"api error: 429",
	"api error: 500",
	"api error: 502",
	"api error: 503",
	"api error: 504",
}

// ScanForCriticalFailures reads the log file and returns a non-empty reason when the log contains
// a pattern indicating the claude CLI itself failed (as opposed to the prompt task failing).
// Returns "" when no critical failure is detected.
// The returned error is reserved for I/O errors (open/read); a detected failure is signalled via the string.
func ScanForCriticalFailures(ctx context.Context, logFile string) (string, error) {
	// #nosec G304 -- logFile path is constructed internally by dark-factory, not user input
	f, err := os.Open(logFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", errors.Wrap(ctx, err, "open log file for failure scan")
	}
	defer f.Close()

	// Read up to the first 64 KiB — auth errors appear near the top.
	buf := make([]byte, headBytes)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", errors.Wrap(ctx, err, "read log head for failure scan")
	}
	lower := strings.ToLower(string(buf[:n]))

	for _, pattern := range criticalFailurePatterns {
		if strings.Contains(lower, pattern) {
			return pattern, nil
		}
	}
	return "", nil
}
