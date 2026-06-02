// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"context"
	"strconv"
	"time"

	"github.com/bborbe/dark-factory/pkg/spec"
)

const defaultVerifyingStaleHours = 24

func (c *checker) detectVerifyingStale(ctx context.Context) ([]Finding, error) {
	staleHours := c.deps.VerifyingStaleHours
	if staleHours <= 0 {
		staleHours = defaultVerifyingStaleHours
	}
	threshold := time.Duration(staleHours) * time.Hour

	specDirs := []string{
		c.deps.SpecsInboxDir,
		c.deps.SpecsInProgressDir,
		c.deps.SpecsCompletedDir,
	}

	specPaths, err := scanDirsForSpecs(ctx, specDirs)
	if err != nil {
		return nil, err
	}

	var findings []Finding
	for _, path := range specPaths {
		sf, err := spec.Load(ctx, path, c.deps.CurrentDateTimeGetter)
		if err != nil {
			continue
		}
		if sf.Frontmatter.Status != string(spec.StatusVerifying) {
			continue
		}

		verifyingStr := sf.Frontmatter.Verifying
		if verifyingStr == "" {
			findings = append(findings, Finding{
				Category:    CategoryVerifyingStale,
				TargetPaths: []string{sf.Path},
				SpecID:      sf.Name,
				Detail:      "spec is in verifying status but Verifying timestamp is empty",
				FixCommand:  "/dark-factory:verify-spec " + sf.Name,
			})
			continue
		}

		ts, err := time.Parse(time.RFC3339, verifyingStr)
		if err != nil {
			findings = append(findings, Finding{
				Category:    CategoryVerifyingStale,
				TargetPaths: []string{sf.Path},
				SpecID:      sf.Name,
				Detail:      "Verifying timestamp unparseable: " + verifyingStr,
				FixCommand:  "/dark-factory:verify-spec " + sf.Name,
			})
			continue
		}

		if time.Since(ts) > threshold {
			findings = append(findings, Finding{
				Category:    CategoryVerifyingStale,
				TargetPaths: []string{sf.Path},
				SpecID:      sf.Name,
				Detail: "spec has been in verifying status for more than " + strconv.Itoa(
					staleHours,
				) + " hours (since " + verifyingStr + ")",
				FixCommand: "/dark-factory:verify-spec " + sf.Name,
			})
		}
	}
	return findings, nil
}
