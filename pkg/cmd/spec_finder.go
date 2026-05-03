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

	"github.com/bborbe/dark-factory/pkg/specnum"
)

// FindSpecFile finds a spec by absolute/relative path, exact filename, or numeric prefix match within specsDir.
func FindSpecFile(ctx context.Context, specsDir, id string) (string, error) {
	return FindSpecFileInDirs(ctx, id, specsDir)
}

// FindSpecFileInDirs resolves an <id> argument against one or more spec directories.
// Accepts four formats:
//   - padded number:              "063"
//   - unpadded number:            "63"
//   - full basename (no ext):     "063-bug-foo"
//   - full basename (with .md):   "063-bug-foo.md"
//
// Resolution order:
//  1. Direct path (absolute or containing a directory separator) — checked as-is.
//  2. Exact basename match — strip .md suffix, append .md, stat in each dir in order.
//  3. Numeric match — parse cleanID as integer via specnum.Parse; scan ALL dirs and
//     collect files whose numeric prefix equals idNum; return unique or ambiguity error.
func FindSpecFileInDirs(ctx context.Context, id string, dirs ...string) (string, error) {
	return findFilesInDirs(ctx, id, "spec", "read specs directory", dirs)
}

// findFilesInDirs resolves an <id> argument against one or more directories using
// integer-value numeric prefix matching and ambiguity detection.
// kind is used in error messages ("spec", "prompt", etc.).
func findFilesInDirs(
	ctx context.Context,
	id, kind, readErrMsg string,
	dirs []string,
) (string, error) {
	if filepath.IsAbs(id) || strings.ContainsRune(id, '/') {
		if _, err := os.Stat(id); err == nil {
			return id, nil
		}
	}
	cleanID := strings.TrimSuffix(id, ".md")
	for _, dir := range dirs {
		path := filepath.Join(dir, cleanID+".md")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	idNum := specnum.Parse(cleanID)
	if idNum < 0 {
		return "", errors.Errorf(ctx, "%s not found: %s", kind, id)
	}
	var matches []string
	for _, dir := range dirs {
		dirMatches, err := collectNumericMatches(ctx, dir, idNum, readErrMsg)
		if err != nil {
			return "", err
		}
		matches = append(matches, dirMatches...)
	}
	switch len(matches) {
	case 0:
		return "", errors.Errorf(ctx, "%s not found: %s", kind, id)
	case 1:
		return matches[0], nil
	default:
		return "", errors.Errorf(
			ctx,
			"ambiguous %s id %s: matches %s",
			kind,
			id,
			strings.Join(matches, ", "),
		)
	}
}

// collectNumericMatches scans dir for .md files whose numeric prefix equals idNum.
func collectNumericMatches(
	ctx context.Context,
	dir string,
	idNum int,
	readErrMsg string,
) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrap(ctx, err, readErrMsg)
	}
	var matches []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if specnum.Parse(entry.Name()) == idNum {
			matches = append(matches, filepath.Join(dir, entry.Name()))
		}
	}
	return matches, nil
}
