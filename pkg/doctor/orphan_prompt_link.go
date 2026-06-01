// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/bborbe/dark-factory/pkg/specnum"
)

func (c *checker) detectOrphanPromptLinks(ctx context.Context) ([]Finding, error) {
	promptDirs := []string{
		c.deps.PromptsInboxDir,
		c.deps.PromptsInProgressDir,
		c.deps.PromptsCompletedDir,
		c.deps.PromptsCancelledDir,
	}
	specDirs := []string{
		c.deps.SpecsInboxDir,
		c.deps.SpecsInProgressDir,
		c.deps.SpecsCompletedDir,
		c.deps.SpecsRejectedDir,
	}

	promptPaths, err := scanDirsForPrompts(ctx, promptDirs)
	if err != nil {
		return nil, err
	}

	specPaths, err := scanDirsForSpecs(ctx, specDirs)
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
			if !c.specExists(specRef, specPaths) {
				promptStem := strings.TrimSuffix(filepath.Base(path), ".md")
				relink := "dark-factory prompt relink " + promptStem + " <new-spec-id>"
				findings = append(findings, Finding{
					Category:    CategoryOrphanPromptLink,
					TargetPaths: []string{path},
					SpecID:      specRef,
					Detail:      "spec " + specRef + " not found; to fix: " + relink,
					FixCommand:  "dark-factory prompt unlink " + promptStem,
				})
			}
		}
	}
	return findings, nil
}

// specExists returns true if a spec with the given reference exists in any of the spec paths.
func (c *checker) specExists(specRef string, specPaths []string) bool {
	specNum := specnum.Parse(specRef)
	for _, path := range specPaths {
		stem := strings.TrimSuffix(filepath.Base(path), ".md")
		if specnum.Parse(stem) == specNum {
			return true
		}
	}
	return false
}
