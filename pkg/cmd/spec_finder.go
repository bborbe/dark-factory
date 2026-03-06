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

// findSpecFile finds a spec by exact filename or numeric prefix match within specsDir.
func findSpecFile(ctx context.Context, specsDir, id string) (string, error) {
	// Try exact match with .md extension
	if strings.HasSuffix(id, ".md") {
		path := filepath.Join(specsDir, id)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	} else {
		// Try as filename without extension
		path := filepath.Join(specsDir, id+".md")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// Try prefix match (e.g. "001" matches "001-my-spec.md")
	entries, err := os.ReadDir(specsDir)
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
			return filepath.Join(specsDir, name), nil
		}
	}

	return "", errors.Errorf(ctx, "spec not found: %s", id)
}
