// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompt

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"
)

//counterfeiter:generate -o ../../mocks/prompt-counter.go --fake-name PromptCounter . Counter

// Counter counts prompts linked to specs.
type Counter interface {
	CountBySpec(ctx context.Context, specID string) (completed, total int, err error)
}

// promptCounter implements Counter by scanning multiple directories.
type promptCounter struct {
	currentDateTimeGetter libtime.CurrentDateTimeGetter
	dirs                  []string
}

// NewCounter creates a Counter that scans the given directories.
func NewCounter(currentDateTimeGetter libtime.CurrentDateTimeGetter, dirs ...string) Counter {
	return &promptCounter{currentDateTimeGetter: currentDateTimeGetter, dirs: dirs}
}

// CountBySpec counts prompts matching specID across all configured directories.
// Returns (completed, total, error).
func (pc *promptCounter) CountBySpec(ctx context.Context, specID string) (int, int, error) {
	completed := 0
	total := 0
	for _, dir := range pc.dirs {
		c, t, err := countInDir(ctx, dir, specID, pc.currentDateTimeGetter)
		if err != nil {
			return 0, 0, errors.Wrap(ctx, err, "count in dir")
		}
		completed += c
		total += t
	}
	return completed, total, nil
}

// countInDir scans a single directory for prompts matching specID.
func countInDir(
	ctx context.Context,
	dir, specID string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (int, int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, errors.Wrap(ctx, err, "read directory")
	}
	completed := 0
	total := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		pf, err := Load(ctx, path, currentDateTimeGetter)
		if err != nil {
			slog.Warn("skipping prompt during count", "file", entry.Name(), "error", err)
			continue
		}
		if !pf.Frontmatter.HasSpec(specID) {
			continue
		}
		total++
		if pf.Frontmatter.Status == string(CompletedPromptStatus) {
			completed++
		}
	}
	return completed, total, nil
}
