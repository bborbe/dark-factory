// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
)

// FindPromptFileInDirs resolves an <id> argument against one or more prompt directories.
// Accepts four formats: padded number, unpadded number, full basename, full basename with .md.
// For numeric IDs, collects all matches across all dirs and returns an ambiguity error if
// more than one file matches the same numeric prefix.
func FindPromptFileInDirs(ctx context.Context, id string, dirs ...string) (string, error) {
	return findFilesInDirs(ctx, id, "prompt", "read directory", dirs)
}

// FindPromptFile finds a prompt file by id in a single directory.
// Accepts four formats: padded number, unpadded number, full basename, full basename with .md.
// Detects and reports ambiguous matches within the single directory.
func FindPromptFile(ctx context.Context, dir, id string) (string, error) {
	return findFilesInDirs(ctx, id, "prompt", "read directory", []string{dir})
}
