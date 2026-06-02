// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"context"
	"log/slog"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
	"github.com/bborbe/dark-factory/pkg/specnum"
)

func (c *checker) detectDuplicateSpecNumbers(ctx context.Context) ([]Finding, error) {
	specDirs := []string{
		c.deps.SpecsInboxDir,
		c.deps.SpecsInProgressDir,
		c.deps.SpecsCompletedDir,
		c.deps.SpecsRejectedDir,
	}
	paths, err := scanDirsForSpecs(ctx, specDirs)
	if err != nil {
		return nil, err
	}
	groups := scanSpecsByNumberPrefix(paths)

	var findings []Finding
	for _, names := range groups {
		if len(names) <= 1 {
			continue
		}
		sort.Strings(names)

		// Later (lex last) file is the one to renumber.
		idToMove := names[len(names)-1]                     // e.g. "019-feature.md"
		idToMoveStem := strings.TrimSuffix(idToMove, ".md") // e.g. "019-feature"

		// Build detail parts describing all colliding files.
		var detailParts []string
		for _, name := range names {
			stem := strings.TrimSuffix(name, ".md")
			status := "unknown"
			// Find the spec to get status and linked-prompt count.
			for _, d := range specDirs {
				p := d + "/" + name
				if sf, err := spec.Load(ctx, p, c.deps.CurrentDateTimeGetter); err == nil {
					status = sf.Frontmatter.Status
					break
				}
			}
			counter := prompt.NewCounter(
				c.deps.CurrentDateTimeGetter,
				c.deps.PromptsInboxDir,
				c.deps.PromptsInProgressDir,
				c.deps.PromptsCompletedDir,
			)
			_, total, countErr := counter.CountBySpec(ctx, stem)
			if countErr != nil {
				slog.Warn("doctor: CountBySpec failed; reporting linked-prompts as 0",
					"spec", stem,
					"error", countErr.Error())
			}
			detailParts = append(
				detailParts,
				name+" (status: "+status+", linked-prompts: "+strconv.Itoa(total)+")",
			)
		}

		// TargetPaths is the spec to renumber (lex-last loser), NOT the survivors.
		// The fixer's filterRelevantRenames matches reindex's emitted OldPath
		// against filepath.Join(specDir, targetPath); reindex moves the LOSER,
		// so TargetPaths must name that file. Detail still lists all colliding
		// files for operator visibility.
		findings = append(findings, Finding{
			Category:    CategoryDuplicateSpecNumbers,
			TargetPaths: []string{idToMove},
			SpecID:      idToMoveStem,
			Detail:      "duplicate spec number across: " + strings.Join(detailParts, "; "),
			FixCommand:  "dark-factory spec renumber " + idToMoveStem,
		})
	}
	return findings, nil
}

// scanSpecsByNumberPrefix scans spec file paths and groups them by numeric prefix.
func scanSpecsByNumberPrefix(paths []string) map[int][]string {
	result := make(map[int][]string)
	for _, path := range paths {
		name := filepath.Base(path)
		num := specnum.Parse(strings.TrimSuffix(name, ".md"))
		if num < 0 {
			continue
		}
		result[num] = append(result[num], name)
	}
	return result
}
