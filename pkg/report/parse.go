// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package report

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"io"
	"os"
	"strings"

	"github.com/bborbe/errors"
)

const tailBytes = 4096

// ErrStartWithoutEnd is returned by ParseFromLog when a start marker is found in the tail
// but no matching end marker follows it. This signals an actionable parse failure —
// the agent began a report but the tail window was cut before the closing marker.
var ErrStartWithoutEnd = stderrors.New("found start marker but no valid end marker")

// ParseFromLog reads the last N bytes of a log file and extracts the CompletionReport.
// Returns nil if no report found (graceful — old prompts won't have one).
func ParseFromLog(ctx context.Context, logFile string) (*CompletionReport, error) {

	// Open the file
	// #nosec G304 -- logFile path is constructed internally by dark-factory, not user input
	f, err := os.Open(logFile)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "open log file")
	}
	defer f.Close()

	// Get file size
	stat, err := f.Stat()
	if err != nil {
		return nil, errors.Wrap(ctx, err, "stat log file")
	}

	// Read last 4096 bytes (or entire file if smaller)
	var offset int64
	readSize := tailBytes
	if stat.Size() < int64(tailBytes) {
		offset = 0
		readSize = int(stat.Size())
	} else {
		offset = stat.Size() - int64(tailBytes)
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, errors.Wrap(ctx, err, "seek to tail")
	}

	buf := make([]byte, readSize)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return nil, errors.Wrap(ctx, err, "read log tail")
	}
	tail := string(buf[:n])

	// Find the last start marker in the tail.
	startIdx := strings.LastIndex(tail, MarkerStart)
	if startIdx == -1 {
		// No report found — not an error, old prompts won't have one.
		return nil, nil //nolint:nilnil
	}

	// Search for an end marker only in the text that follows the last start marker.
	remaining := tail[startIdx+len(MarkerStart):]
	relEndIdx := strings.Index(remaining, MarkerEnd)
	if relEndIdx == -1 {
		return nil, errors.Wrap(ctx, ErrStartWithoutEnd, "parse report tail boundary")
	}

	// Extract JSON between the markers.
	jsonStr := strings.TrimSpace(remaining[:relEndIdx])

	// Unmarshal.
	var report CompletionReport
	if err := json.Unmarshal([]byte(jsonStr), &report); err != nil {
		return nil, errors.Wrap(ctx, err, "unmarshal completion report JSON")
	}
	return &report, nil
}
