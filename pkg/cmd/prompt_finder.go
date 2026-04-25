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

// FindPromptFileInDirs searches dirs in order and returns the first match.
func FindPromptFileInDirs(ctx context.Context, id string, dirs ...string) (string, error) {
	// Try as a direct path first (absolute or relative with directory component)
	if filepath.IsAbs(id) || strings.ContainsRune(id, '/') {
		if _, err := os.Stat(id); err == nil {
			return id, nil
		}
	}

	for _, dir := range dirs {
		path, err := FindPromptFile(ctx, dir, id)
		if err == nil {
			return path, nil
		}
	}
	return "", errors.Errorf(ctx, "prompt not found: %s", id)
}

// FindPromptFile finds a prompt file by id in dir, supporting:
// - absolute or relative path with directory component (checked directly)
// - exact filename with .md extension
// - filename without .md extension
// - numeric prefix match (e.g. "122" matches "122-some-name.md")
func FindPromptFile(ctx context.Context, dir, id string) (string, error) {
	// Try as a direct path first (absolute or relative with directory component)
	if filepath.IsAbs(id) || strings.ContainsRune(id, '/') {
		if _, err := os.Stat(id); err == nil {
			return id, nil
		}
	}

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

	// Try prefix match (e.g. "122" matches "122-some-name.md")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.Errorf(ctx, "file not found: %s", id)
		}
		return "", errors.Wrap(ctx, err, "read directory")
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if strings.HasPrefix(entry.Name(), id+"-") {
			return filepath.Join(dir, entry.Name()), nil
		}
	}

	return "", errors.Errorf(ctx, "file not found: %s", id)
}
