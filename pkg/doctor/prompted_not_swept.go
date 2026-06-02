// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"context"

	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
)

// promptTerminalStatuses contains statuses that indicate a prompt is done.
var promptTerminalStatuses = []prompt.PromptStatus{
	prompt.CompletedPromptStatus,
	prompt.CancelledPromptStatus,
	prompt.RejectedPromptStatus,
}

func (c *checker) detectPromptedNotSwept(ctx context.Context) ([]Finding, error) {
	specDirs := []string{
		c.deps.SpecsInboxDir,
		c.deps.SpecsInProgressDir,
		c.deps.SpecsCompletedDir,
	}

	// Collect all specs in prompted status across spec dirs.
	var promptedSpecs []*spec.SpecFile
	specPaths, err := scanDirsForSpecs(ctx, specDirs)
	if err != nil {
		return nil, err
	}
	for _, path := range specPaths {
		sf, err := spec.Load(ctx, path, c.deps.CurrentDateTimeGetter)
		if err != nil {
			continue
		}
		if sf.Frontmatter.Status == string(spec.StatusPrompted) {
			promptedSpecs = append(promptedSpecs, sf)
		}
	}

	var findings []Finding
	for _, sf := range promptedSpecs {
		allTerminal, total, err := c.linkedPromptsAllTerminal(ctx, sf.Name)
		if err != nil {
			return nil, err
		}
		if !allTerminal {
			continue
		}
		findings = append(findings, Finding{
			Category:    CategoryPromptedNotSwept,
			TargetPaths: []string{sf.Path},
			SpecID:      sf.Name,
			Detail: "spec status is prompted but all " + itoa(
				total,
			) + " linked prompt(s) are terminal",
			FixCommand: "dark-factory spec sweep " + sf.Name,
		})
	}
	return findings, nil
}

// linkedPromptsAllTerminal returns (allTerminal, total, error).
// It scans all four prompt dirs for prompts referencing specID.
func (c *checker) linkedPromptsAllTerminal(ctx context.Context, specID string) (bool, int, error) {
	promptDirs := []string{
		c.deps.PromptsInboxDir,
		c.deps.PromptsInProgressDir,
		c.deps.PromptsCompletedDir,
		c.deps.PromptsCancelledDir,
	}

	promptPaths, err := scanDirsForPrompts(ctx, promptDirs)
	if err != nil {
		return false, 0, err
	}

	total := 0
	for _, path := range promptPaths {
		pf, err := c.deps.PromptManager.Load(ctx, path)
		if err != nil {
			continue
		}
		if !pf.Frontmatter.HasSpec(specID) {
			continue
		}
		total++
		if !isTerminalStatus(prompt.PromptStatus(pf.Frontmatter.Status)) {
			return false, total, nil
		}
	}
	if total == 0 {
		return false, 0, nil
	}
	return true, total, nil
}

func isTerminalStatus(s prompt.PromptStatus) bool {
	for _, t := range promptTerminalStatuses {
		if s == t {
			return true
		}
	}
	return false
}
