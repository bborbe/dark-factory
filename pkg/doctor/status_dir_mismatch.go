// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
)

var specInProgressAllowed = []spec.Status{
	spec.StatusIdea,
	spec.StatusDraft,
	spec.StatusApproved,
	spec.StatusGenerating,
	spec.StatusPrompted,
	spec.StatusVerifying,
}

func (c *checker) detectStatusDirMismatches(ctx context.Context) ([]Finding, error) {
	var findings []Finding

	// Spec checks.
	specFindings, err := c.checkSpecDirStatuses(ctx)
	if err != nil {
		return nil, err
	}
	findings = append(findings, specFindings...)

	// Prompt checks.
	promptFindings, err := c.checkPromptDirStatuses(ctx)
	if err != nil {
		return nil, err
	}
	findings = append(findings, promptFindings...)

	return findings, nil
}

func (c *checker) checkSpecDirStatuses(ctx context.Context) ([]Finding, error) {
	var findings []Finding

	// specs/in-progress/ must have allowed statuses.
	inProgressPaths, err := scanDirsForSpecs(ctx, []string{c.deps.SpecsInProgressDir})
	if err != nil {
		return nil, err
	}
	for _, path := range inProgressPaths {
		sf, err := spec.Load(ctx, path, c.deps.CurrentDateTimeGetter)
		if err != nil {
			continue
		}
		if !isAllowedSpecStatus(sf.Frontmatter.Status, specInProgressAllowed) {
			findings = append(findings, Finding{
				Category:    CategoryStatusDirMismatch,
				TargetPaths: []string{sf.Path},
				SpecID:      sf.Name,
				Detail:      "spec in specs/in-progress/ has status " + sf.Frontmatter.Status + " but only statuses {idea, draft, approved, generating, prompted, verifying} are allowed in that directory",
				FixCommand:  "dark-factory spec move " + sf.Name,
			})
		}
	}

	// specs/completed/ must have status "completed".
	completedPaths, err := scanDirsForSpecs(ctx, []string{c.deps.SpecsCompletedDir})
	if err != nil {
		return nil, err
	}
	for _, path := range completedPaths {
		sf, err := spec.Load(ctx, path, c.deps.CurrentDateTimeGetter)
		if err != nil {
			continue
		}
		if sf.Frontmatter.Status != string(spec.StatusCompleted) {
			findings = append(findings, Finding{
				Category:    CategoryStatusDirMismatch,
				TargetPaths: []string{sf.Path},
				SpecID:      sf.Name,
				Detail:      "spec in specs/completed/ has status " + sf.Frontmatter.Status + " but only status completed is allowed in that directory",
				FixCommand:  "dark-factory spec move " + sf.Name,
			})
		}
	}

	// specs/rejected/ must have status "rejected".
	rejectedPaths, err := scanDirsForSpecs(ctx, []string{c.deps.SpecsRejectedDir})
	if err != nil {
		return nil, err
	}
	for _, path := range rejectedPaths {
		sf, err := spec.Load(ctx, path, c.deps.CurrentDateTimeGetter)
		if err != nil {
			continue
		}
		if sf.Frontmatter.Status != string(spec.StatusRejected) {
			findings = append(findings, Finding{
				Category:    CategoryStatusDirMismatch,
				TargetPaths: []string{sf.Path},
				SpecID:      sf.Name,
				Detail:      "spec in specs/rejected/ has status " + sf.Frontmatter.Status + " but only status rejected is allowed in that directory",
				FixCommand:  "dark-factory spec move " + sf.Name,
			})
		}
	}

	return findings, nil
}

func (c *checker) checkPromptDirStatuses(ctx context.Context) ([]Finding, error) {
	var findings []Finding

	// prompts/in-progress/ allowed statuses.
	inProgressAllowed := []prompt.PromptStatus{
		prompt.IdeaPromptStatus,
		prompt.DraftPromptStatus,
		prompt.ApprovedPromptStatus,
		prompt.ExecutingPromptStatus,
		prompt.FailedPromptStatus,
		prompt.InReviewPromptStatus,
		prompt.PendingVerificationPromptStatus,
		prompt.CommittingPromptStatus,
	}

	inProgressPaths, err := scanDirsForPrompts(ctx, []string{c.deps.PromptsInProgressDir})
	if err != nil {
		return nil, err
	}
	for _, path := range inProgressPaths {
		pf, err := c.deps.PromptManager.Load(ctx, path)
		if err != nil {
			continue
		}
		if !isAllowedPromptStatus(pf.Frontmatter.Status, inProgressAllowed) {
			promptStem := strings.TrimSuffix(filepath.Base(path), ".md")
			findings = append(findings, Finding{
				Category:    CategoryStatusDirMismatch,
				TargetPaths: []string{pf.Path},
				SpecID:      "",
				Detail:      "prompt in prompts/in-progress/ has status " + pf.Frontmatter.Status + " but only statuses {idea, draft, approved, executing, failed, in_review, pending_verification, committing} are allowed in that directory",
				FixCommand:  "dark-factory prompt move " + promptStem,
			})
		}
	}

	// prompts/completed/ allowed: completed or rejected.
	completedAllowed := []prompt.PromptStatus{
		prompt.CompletedPromptStatus,
		prompt.RejectedPromptStatus,
	}
	completedPaths, err := scanDirsForPrompts(ctx, []string{c.deps.PromptsCompletedDir})
	if err != nil {
		return nil, err
	}
	for _, path := range completedPaths {
		pf, err := c.deps.PromptManager.Load(ctx, path)
		if err != nil {
			continue
		}
		if !isAllowedPromptStatus(pf.Frontmatter.Status, completedAllowed) {
			promptStem := strings.TrimSuffix(filepath.Base(path), ".md")
			findings = append(findings, Finding{
				Category:    CategoryStatusDirMismatch,
				TargetPaths: []string{pf.Path},
				SpecID:      "",
				Detail:      "prompt in prompts/completed/ has status " + pf.Frontmatter.Status + " but only statuses {completed, rejected} are allowed in that directory",
				FixCommand:  "dark-factory prompt move " + promptStem,
			})
		}
	}

	// prompts/cancelled/ allowed: cancelled.
	cancelledPaths, err := scanDirsForPrompts(ctx, []string{c.deps.PromptsCancelledDir})
	if err != nil {
		return nil, err
	}
	for _, path := range cancelledPaths {
		pf, err := c.deps.PromptManager.Load(ctx, path)
		if err != nil {
			continue
		}
		if pf.Frontmatter.Status != string(prompt.CancelledPromptStatus) {
			promptStem := strings.TrimSuffix(filepath.Base(path), ".md")
			findings = append(findings, Finding{
				Category:    CategoryStatusDirMismatch,
				TargetPaths: []string{pf.Path},
				SpecID:      "",
				Detail:      "prompt in prompts/cancelled/ has status " + pf.Frontmatter.Status + " but only status cancelled is allowed in that directory",
				FixCommand:  "dark-factory prompt move " + promptStem,
			})
		}
	}

	return findings, nil
}

func isAllowedSpecStatus(status string, allowed []spec.Status) bool {
	for _, s := range allowed {
		if status == string(s) {
			return true
		}
	}
	return false
}

func isAllowedPromptStatus(status string, allowed []prompt.PromptStatus) bool {
	for _, s := range allowed {
		if status == string(s) {
			return true
		}
	}
	return false
}
