// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"context"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/spec"
)

// detectGeneratingWithoutPrompt reports specs parked in generating status with
// no prompt referencing them anywhere.
//
// generating is a transient state: the generator holds it only while a
// container produces prompt files. Nothing times it out, so a generation that
// dies — container OOM-killed, host rebooted, the operator approving prompts
// out from under a still-running generator — leaves the spec in generating
// forever with nothing on disk to show for it. No other detector sees this:
// verifying-stale keys on verifying, prompted-but-not-swept keys on prompted,
// and status-dir-mismatch explicitly lists generating as legal in
// specs/in-progress/. The spec is simply parked, silently, and the pipeline
// reports itself clean.
//
// A spec with prompts is deliberately NOT reported even when still marked
// generating. That is a status-bookkeeping drift with the work visible on
// disk, and calling it "nothing was generated" would be wrong; the conservative
// reading keeps this detector's one claim true.
func (c *checker) detectGeneratingWithoutPrompt(ctx context.Context) ([]Finding, error) {
	specPaths, err := scanDirsForSpecs(ctx, []string{c.deps.SpecsInProgressDir})
	if err != nil {
		return nil, errors.Wrap(ctx, err, "scan specs in-progress dir")
	}

	promptPaths, err := scanDirsForPrompts(ctx, []string{
		c.deps.PromptsInboxDir,
		c.deps.PromptsInProgressDir,
		c.deps.PromptsCompletedDir,
		c.deps.PromptsCancelledDir,
		c.deps.PromptsRejectedDir,
	})
	if err != nil {
		return nil, errors.Wrap(ctx, err, "scan prompt dirs")
	}

	var findings []Finding
	for _, path := range specPaths {
		sf, err := spec.Load(ctx, path, c.deps.CurrentDateTimeGetter)
		if err != nil {
			continue
		}
		if sf.Frontmatter.Status != string(spec.StatusGenerating) {
			continue
		}
		if c.promptReferencesSpec(ctx, sf.Name, promptPaths) {
			continue
		}
		findings = append(findings, Finding{
			Category:    CategoryGeneratingWithoutPrompt,
			TargetPaths: []string{sf.Path},
			SpecID:      sf.Name,
			Detail: "spec is in generating status but no prompt references it — " +
				"generation produced nothing, and nothing will retry it",
			FixCommand: c.generatingWithoutPromptFixCommand(sf.Name),
		})
	}

	return findings, nil
}

// promptReferencesSpec reports whether any prompt links to the given spec.
func (c *checker) promptReferencesSpec(
	ctx context.Context,
	specName string,
	promptPaths []string,
) bool {
	for _, path := range promptPaths {
		pf, err := c.deps.PromptManager.Load(ctx, path)
		if err != nil {
			continue
		}
		for _, specRef := range pf.Frontmatter.Specs {
			if c.specExists(specRef, []string{specName + ".md"}) {
				return true
			}
		}
	}
	return false
}

// generatingWithoutPromptFixCommand names both real exits without guessing
// which applies.
//
// Both commands below were verified to exist before being emitted here — this
// package has previously shipped fix commands for subcommands that do not
// exist (`prompt unlink`, `spec renumber`), which send an already-stuck
// operator to a second dead end.
//
// The two exits are genuinely different decisions and the detector cannot tell
// them apart from disk: re-run generation, or record that the prompts exist
// under a name this check could not match. Naming both, in that order, keeps
// the operator making the call.
func (c *checker) generatingWithoutPromptFixCommand(specName string) string {
	return "re-run generation with dark-factory spec unapprove " + specName +
		" then dark-factory spec approve " + specName +
		"; if prompts for it do exist under another spec reference, " +
		"record that instead with dark-factory spec mark-prompted " + specName
}
