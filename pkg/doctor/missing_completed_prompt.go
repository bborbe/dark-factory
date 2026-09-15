// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"context"
	"path/filepath"
	"strconv"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

// detectMissingCompletedPrompt reports every prompt number below the highest
// number present anywhere in prompts/ that has no file in prompts/completed/.
//
// This mirrors the rule the execution guard applies via
// prompt.Manager.FindMissingCompleted: a spec-less prompt below such a gap is
// refused with reason=previous-prompt-not-completed. The highest number present
// is never reported — it is the newest, in-flight prompt and is legitimately
// not completed yet.
//
// Three shapes are covered: a number whose file sits in in-progress/ (the
// 186/187 shape), a number whose only file is in rejected/ (terminal, and not
// requeueable), and a number whose file exists nowhere at all (the never-created
// gap, which no directory listing can reveal).
//
// Scope: like every doctor detector this reads the WORKING TREE and takes no git
// ref, so a finding describes the tree it ran in — not origin/master. A stale
// checkout yields stale findings, and callers reasoning about master must check
// out master first. A 2026-09-15 fleet sweep found 72 of 89 checkouts behind
// origin/master, so this is the common case rather than the edge case.
func (c *checker) detectMissingCompletedPrompt(ctx context.Context) ([]Finding, error) {
	allPaths, err := scanDirsForPrompts(ctx, []string{
		c.deps.PromptsInboxDir,
		c.deps.PromptsInProgressDir,
		c.deps.PromptsCompletedDir,
		c.deps.PromptsCancelledDir,
		c.deps.PromptsRejectedDir,
	})
	if err != nil {
		return nil, errors.Wrap(ctx, err, "scan prompt dirs")
	}

	completedPaths, err := scanDirsForPrompts(ctx, []string{c.deps.PromptsCompletedDir})
	if err != nil {
		return nil, errors.Wrap(ctx, err, "scan completed dir")
	}

	pathsByNumber := make(map[int][]string)
	for _, path := range allPaths {
		n := prompt.NumberFromFilename(filepath.Base(path))
		if n < 0 {
			continue
		}
		pathsByNumber[n] = append(pathsByNumber[n], path)
	}

	completedNumbers := make(map[int]bool)
	for _, path := range completedPaths {
		n := prompt.NumberFromFilename(filepath.Base(path))
		if n < 0 {
			continue
		}
		completedNumbers[n] = true
	}

	if len(pathsByNumber) == 0 {
		return nil, nil
	}

	lo, hi := 0, 0
	first := true
	for n := range pathsByNumber {
		if first {
			lo, hi = n, n
			first = false
			continue
		}
		if n < lo {
			lo = n
		}
		if n > hi {
			hi = n
		}
	}

	var findings []Finding
	for n := lo; n < hi; n++ {
		if completedNumbers[n] {
			continue
		}
		findings = append(findings, Finding{
			Category:    CategoryMissingCompletedPrompt,
			TargetPaths: pathsByNumber[n],
			SpecID:      "",
			Detail: "prompt number " + strconv.Itoa(
				n,
			) + " is missing from " + c.deps.PromptsCompletedDir,
			FixCommand: c.missingCompletedFixCommand(n, pathsByNumber[n]),
		})
	}

	return findings, nil
}

// missingCompletedFixCommand picks the remedy that fits the gap's shape.
//
// A gap is a bookkeeping fact, NOT a claim that the work is unfinished, and the
// remedy must not assume it is. Observed on bborbe/vault-cli: prompts 186 and 187
// sat outside completed/ with status failed/approved, yet both had shipped —
// c70d170 and 79a6949 carry the code, and the repair commit was named "reconcile
// prompts 186/187 to shipped reality". Leading with `prompt requeue` there would
// have told an operator to re-execute work already in the tree, at the moment
// they are blocked and looking for the fastest way out.
//
// So every branch tells the reader to establish whether the work shipped first,
// and offers reconcile-or-requeue rather than a single confident command. The
// rejected branch exists because prompts/rejected/ is terminal and not
// requeueable — `prompt requeue` reads the in-progress dir alone.
func (c *checker) missingCompletedFixCommand(n int, paths []string) string {
	num := strconv.Itoa(n)
	if len(paths) == 0 {
		return "no file for prompt number " + num +
			"; if its work shipped, this number is a permanent gap — renumber the blocking prompt " +
			"into a free slot below it, then re-run dark-factory doctor"
	}
	if rejected := c.rejectedPaths(paths); len(rejected) > 0 && len(rejected) == len(paths) {
		return "prompt number " + num + " is rejected (" + rejected[0] +
			"); check whether its work already shipped — if so move the file to " +
			c.deps.PromptsCompletedDir + ", otherwise move it back to " +
			c.deps.PromptsInProgressDir + " and run dark-factory prompt requeue " + num
	}
	return "check whether prompt " + num + " already shipped (git log for its changes) — " +
		"if so move its file to " + c.deps.PromptsCompletedDir +
		" to record that; only run dark-factory prompt requeue " + num + " if it genuinely never ran"
}

// rejectedPaths returns the subset of paths that live under the rejected dir.
func (c *checker) rejectedPaths(paths []string) []string {
	if c.deps.PromptsRejectedDir == "" {
		return nil
	}
	var rejected []string
	for _, p := range paths {
		if filepath.Dir(p) == filepath.Clean(c.deps.PromptsRejectedDir) {
			rejected = append(rejected, p)
		}
	}
	return rejected
}
