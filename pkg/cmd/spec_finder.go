// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"
)

// FindSpecFile finds a spec by absolute/relative path, exact filename, or numeric prefix match within specsDir.
func FindSpecFile(ctx context.Context, specsDir, id string) (string, error) {
	return FindSpecFileInDirs(ctx, id, specsDir)
}

// FindSpecFileInDirs searches dirs in order and returns the first match.
// Falls back to the existing FindSpecFile logic for each dir.
func FindSpecFileInDirs(ctx context.Context, id string, dirs ...string) (string, error) {
	// Try as a direct path first (absolute or relative with directory component)
	if filepath.IsAbs(id) || strings.ContainsRune(id, '/') {
		if _, err := os.Stat(id); err == nil {
			return id, nil
		}
	}

	for _, dir := range dirs {
		path, err := findInDir(ctx, dir, id)
		if err == nil {
			return path, nil
		}
	}
	return "", errors.Errorf(ctx, "spec not found: %s", id)
}

// findInDir searches for a spec file matching id within a single directory.
func findInDir(ctx context.Context, dir, id string) (string, error) {
	// Try exact match with .md extension
	if strings.HasSuffix(id, ".md") {
		path := filepath.Join(dir, id)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	} else {
		// Try as filename without extension
		path := filepath.Join(dir, id+".md")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// Try prefix match (e.g. "001" matches "001-my-spec.md")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.Errorf(ctx, "spec not found: %s", id)
		}
		return "", errors.Wrap(ctx, err, "read specs directory")
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, id+"-") ||
			strings.HasPrefix(name, id) && strings.HasSuffix(name, ".md") {
			return filepath.Join(dir, name), nil
		}
	}

	return "", errors.Errorf(ctx, "spec not found: %s", id)
}
