// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package review

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/bborbe/errors"
)

var xmlTagPattern = regexp.MustCompile(`<[^>]{0,100}>`)

// SanitizeReviewBody strips XML/HTML-like tags from the review body to prevent prompt injection.
func SanitizeReviewBody(body string) string {
	return xmlTagPattern.ReplaceAllStringFunc(body, func(match string) string {
		return "&lt;" + match[1:len(match)-1] + "&gt;"
	})
}

//counterfeiter:generate -o ../../mocks/fix_prompt_generator.go --fake-name FixPromptGenerator . FixPromptGenerator

// FixPromptGenerator generates a fix prompt file in the inbox directory when a PR receives a request-changes review.
type FixPromptGenerator interface {
	Generate(ctx context.Context, opts GenerateOpts) error
}

// GenerateOpts holds the options for generating a fix prompt.
type GenerateOpts struct {
	InboxDir           string
	OriginalPromptName string // filename without path, used to derive fix prompt name
	Branch             string
	PRURL              string
	RetryCount         int
	ReviewBody         string
}

// fixPromptGenerator implements FixPromptGenerator.
type fixPromptGenerator struct{}

// NewFixPromptGenerator creates a new FixPromptGenerator.
func NewFixPromptGenerator() FixPromptGenerator {
	return &fixPromptGenerator{}
}

// Generate writes a fix prompt to inboxDir. It is idempotent: if the file already exists, it returns nil.
func (g *fixPromptGenerator) Generate(ctx context.Context, opts GenerateOpts) error {
	filename := fmt.Sprintf("fix-%s-retry-%d.md", opts.OriginalPromptName, opts.RetryCount)
	destPath := filepath.Join(opts.InboxDir, filename)

	if _, err := os.Stat(destPath); err == nil {
		// File already exists — idempotent, skip
		return nil
	}

	content := "<objective>\n" +
		"Fix the issues raised in the code review for PR " + opts.PRURL + ".\n" +
		"</objective>\n\n" +
		"<context>\n" +
		"Read CLAUDE.md for project conventions.\n" +
		"This is a follow-up fix for branch " + opts.Branch + ".\n" +
		"</context>\n\n" +
		"<requirements>\n" +
		"Fix all issues raised in the review feedback below. Do not change unrelated code.\n" +
		"</requirements>\n\n" +
		"<review_feedback>\n" +
		SanitizeReviewBody(opts.ReviewBody) + "\n" +
		"</review_feedback>\n\n" +
		"<constraints>\n" +
		"- Do NOT commit — dark-factory handles git\n" +
		"- Run make precommit to verify\n" +
		"</constraints>\n\n" +
		"<verification>\n" +
		"Run `make precommit` — must pass.\n" +
		"</verification>\n"

	// #nosec G306 -- prompt files are not sensitive
	if err := os.WriteFile(destPath, []byte(content), 0600); err != nil {
		return errors.Wrap(ctx, err, "write fix prompt")
	}

	return nil
}
