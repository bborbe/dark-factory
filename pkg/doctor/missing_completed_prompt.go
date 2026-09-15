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
// Two shapes are covered: a number whose file sits in in-progress/ (the
// 186/187 shape) and a number whose file exists nowhere at all (the
// never-created gap, which no directory listing can reveal).
func (c *checker) detectMissingCompletedPrompt(ctx context.Context) ([]Finding, error) {
	allPaths, err := scanDirsForPrompts(ctx, []string{
		c.deps.PromptsInboxDir,
		c.deps.PromptsInProgressDir,
		c.deps.PromptsCompletedDir,
		c.deps.PromptsCancelledDir,
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
			FixCommand: missingCompletedFixCommand(n, pathsByNumber[n]),
		})
	}

	return findings, nil
}

// missingCompletedFixCommand picks the remedy that fits the gap's shape.
// A number whose file exists somewhere is requeued; a number with no file at
// all cannot be requeued and needs the blocking prompt renumbered instead.
func missingCompletedFixCommand(n int, paths []string) string {
	if len(paths) > 0 {
		return "dark-factory prompt requeue " + strconv.Itoa(n)
	}
	return "no file for prompt number " + strconv.Itoa(n) +
		"; renumber the blocking prompt into a free slot below it, then re-run dark-factory doctor"
}
