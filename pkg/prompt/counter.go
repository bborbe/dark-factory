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
)

// Counter counts prompts linked to specs.
//
//counterfeiter:generate -o ../../mocks/prompt-counter.go --fake-name PromptCounter . Counter
type Counter interface {
	CountBySpec(ctx context.Context, specID string) (completed, total int, err error)
}

// promptCounter implements Counter by scanning multiple directories.
type promptCounter struct {
	dirs []string
}

// NewCounter creates a Counter that scans the given directories.
func NewCounter(dirs ...string) Counter {
	return &promptCounter{dirs: dirs}
}

// CountBySpec counts prompts matching specID across all configured directories.
// Returns (completed, total, error).
func (pc *promptCounter) CountBySpec(ctx context.Context, specID string) (int, int, error) {
	completed := 0
	total := 0
	for _, dir := range pc.dirs {
		c, t, err := countInDir(ctx, dir, specID)
		if err != nil {
			return 0, 0, errors.Wrap(ctx, err, "count in dir")
		}
		completed += c
		total += t
	}
	return completed, total, nil
}

// countInDir scans a single directory for prompts matching specID.
func countInDir(ctx context.Context, dir, specID string) (int, int, error) {
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
		pf, err := Load(ctx, path)
		if err != nil {
			slog.Warn("skipping prompt during count", "file", entry.Name(), "error", err)
			continue
		}
		if pf.Frontmatter.Spec != specID {
			continue
		}
		total++
		if pf.Frontmatter.Status == string(StatusCompleted) {
			completed++
		}
	}
	return completed, total, nil
}
