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
	f, err := c.checkPromptDir(
		ctx,
		c.deps.PromptsInProgressDir,
		inProgressAllowed,
		"prompts/in-progress/",
		"{idea, draft, approved, executing, failed, in_review, pending_verification, committing}",
		true,
	)
	if err != nil {
		return nil, err
	}
	findings = append(findings, f...)

	completedAllowed := []prompt.PromptStatus{
		prompt.CompletedPromptStatus,
		prompt.RejectedPromptStatus,
	}
	f, err = c.checkPromptDir(
		ctx,
		c.deps.PromptsCompletedDir,
		completedAllowed,
		"prompts/completed/",
		"{completed, rejected}",
		true,
	)
	if err != nil {
		return nil, err
	}
	findings = append(findings, f...)

	f, err = c.checkPromptDir(
		ctx,
		c.deps.PromptsCancelledDir,
		[]prompt.PromptStatus{prompt.CancelledPromptStatus},
		"prompts/cancelled/",
		"cancelled",
		false,
	)
	if err != nil {
		return nil, err
	}
	findings = append(findings, f...)

	return findings, nil
}

func (c *checker) checkPromptDir(
	ctx context.Context,
	dir string,
	allowed []prompt.PromptStatus,
	dirLabel string,
	allowedLabel string,
	plural bool,
) ([]Finding, error) {
	paths, err := scanDirsForPrompts(ctx, []string{dir})
	if err != nil {
		return nil, err
	}
	var findings []Finding
	allowedSuffix := "only status " + allowedLabel + " is allowed in that directory"
	if plural {
		allowedSuffix = "only statuses " + allowedLabel + " are allowed in that directory"
	}
	for _, path := range paths {
		pf, err := c.deps.PromptManager.Load(ctx, path)
		if err != nil {
			continue
		}
		if isAllowedPromptStatus(pf.Frontmatter.Status, allowed) {
			continue
		}
		promptStem := strings.TrimSuffix(filepath.Base(path), ".md")
		findings = append(findings, Finding{
			Category:    CategoryStatusDirMismatch,
			TargetPaths: []string{pf.Path},
			SpecID:      "",
			Detail:      "prompt in " + dirLabel + " has status " + pf.Frontmatter.Status + " but " + allowedSuffix,
			FixCommand:  "dark-factory prompt move " + promptStem,
		})
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
