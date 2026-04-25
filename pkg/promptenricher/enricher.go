// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package promptenricher

import (
	"context"
	"log/slog"

	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/report"
	"github.com/bborbe/dark-factory/pkg/validationprompt"
)

//counterfeiter:generate -o ../../mocks/prompt-enricher.go --fake-name PromptEnricher . Enricher

// Enricher prepends additionalInstructions and appends machine-parseable suffixes
// (completion report, changelog hint, test command, validation command, validation criteria).
type Enricher interface {
	Enrich(ctx context.Context, content string) string
}

// NewEnricher creates a new prompt Enricher.
// Uses primitive types in the constructor signature to avoid an import cycle
// (pkg/processor imports promptenricher; promptenricher cannot import processor).
func NewEnricher(
	releaser git.Releaser,
	additionalInstructions string,
	testCommand string,
	validationCommand string,
	validationPromptCriteria string,
	validationPromptResolver validationprompt.Resolver,
) Enricher {
	return &enricher{
		releaser:                 releaser,
		additionalInstructions:   additionalInstructions,
		testCommand:              testCommand,
		validationCommand:        validationCommand,
		validationPromptCriteria: validationPromptCriteria,
		validationPromptResolver: validationPromptResolver,
	}
}

type enricher struct {
	releaser                 git.Releaser
	additionalInstructions   string
	testCommand              string
	validationCommand        string
	validationPromptCriteria string
	validationPromptResolver validationprompt.Resolver
}

// Enrich prepends additionalInstructions and appends machine-parseable suffixes.
func (e *enricher) Enrich(ctx context.Context, content string) string {
	if e.additionalInstructions != "" {
		content = e.additionalInstructions + "\n\n" + content
	}
	// Append completion report suffix to make output machine-parseable
	content = content + report.Suffix()
	// Append changelog instructions when the project has a CHANGELOG.md
	if e.releaser.HasChangelog(ctx) {
		content = content + report.ChangelogSuffix()
	}
	// Inject project-level test command for fast iteration feedback
	if e.testCommand != "" {
		content = content + report.TestCommandSuffix(e.testCommand)
	}
	// Inject project-level validation command (overrides prompt-level <verification>)
	if e.validationCommand != "" {
		content = content + report.ValidationSuffix(e.validationCommand)
	}
	// Inject project-level validation prompt criteria (AI-judged, runs after validationCommand)
	criteria, ok, err := e.validationPromptResolver.Resolve(ctx, e.validationPromptCriteria)
	if err != nil {
		slog.WarnContext(ctx, "failed to resolve validationPrompt",
			"value", e.validationPromptCriteria, "error", err)
	} else if ok {
		content = content + report.ValidationPromptSuffix(criteria)
	}
	return content
}
