// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/dark-factory/pkg/specnum"
)

func (c *checker) detectOrphanInProgressPrompts(ctx context.Context) ([]Finding, error) {
	completedOrRejectedDirs := []string{
		c.deps.SpecsCompletedDir,
		c.deps.SpecsRejectedDir,
	}

	promptPaths, err := scanDirsForPrompts(ctx, []string{c.deps.PromptsInProgressDir})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	specPaths, err := scanDirsForSpecs(ctx, completedOrRejectedDirs)
	if err != nil {
		return nil, err
	}

	var findings []Finding
	for _, path := range promptPaths {
		pf, err := c.deps.PromptManager.Load(ctx, path)
		if err != nil {
			continue
		}

		for _, specRef := range pf.Frontmatter.Specs {
			if c.specExistsInDirs(specRef, specPaths) {
				promptStem := strings.TrimSuffix(filepath.Base(path), ".md")
				findings = append(findings, Finding{
					Category:    CategoryOrphanInProgressPrompt,
					TargetPaths: []string{path},
					SpecID:      specRef,
					Detail:      "prompt links to spec " + specRef + " which is completed or rejected",
					FixCommand:  "dark-factory prompt cancel " + promptStem,
				})
				break // one finding per prompt, not per spec ref
			}
		}
	}
	return findings, nil
}

// specExistsInDirs returns true if a spec with the given reference exists in any of the spec paths.
func (c *checker) specExistsInDirs(specRef string, specPaths []string) bool {
	specNum := specnum.Parse(specRef)
	for _, path := range specPaths {
		stem := strings.TrimSuffix(filepath.Base(path), ".md")
		if specnum.Parse(stem) == specNum {
			return true
		}
	}
	return false
}
