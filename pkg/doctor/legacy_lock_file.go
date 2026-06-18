// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"
)

// detectLegacyLockFiles scans all prompt and spec status directories for *.lock
// sidecar files left by the abandoned per-file locking scheme (spec 097).
func (c *checker) detectLegacyLockFiles(ctx context.Context) ([]Finding, error) {
	dirs := []string{
		c.deps.PromptsInboxDir,
		c.deps.PromptsInProgressDir,
		c.deps.PromptsCompletedDir,
		c.deps.PromptsCancelledDir,
		c.deps.SpecsInboxDir,
		c.deps.SpecsInProgressDir,
		c.deps.SpecsCompletedDir,
		c.deps.SpecsRejectedDir,
	}

	var findings []Finding
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, errors.Wrap(ctx, err, "read directory for lock-file scan: "+dir)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if !strings.HasSuffix(e.Name(), ".lock") {
				continue
			}
			findings = append(findings, Finding{
				Category:    CategoryLegacyLockFile,
				TargetPaths: []string{filepath.Join(dir, e.Name())},
				Detail:      "legacy lock-file sidecar from old per-file locking scheme",
				FixCommand:  "dark-factory doctor --fix",
			})
		}
	}
	return findings, nil
}
