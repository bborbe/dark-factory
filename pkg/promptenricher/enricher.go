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

const hideGitGuidanceFragment = `hideGit=true active: /workspace/.git appears as a character device when the dark-factory hideGit mode is active. This is intentional behavior of the dark-factory YOLO container - it is not a broken or corrupt repository. The mask prevents the agent from accidentally using git commands that would conflict with dark-factory's workflow management.

When hideGit=true active, GOFLAGS=-buildvcs=false is typically already set in the container environment, so go test, errcheck, gosec, golangci-lint, and other static analysis tools work normally without git version control metadata. You do not need to work around this.

Regardless of how /workspace/.git appears (as a directory, character device, or any other file type), run the project's precommit gate (make precommit or the equivalent validation command) to validate your changes before reporting completion. Do not skip or bypass the validation because of .git's appearance.`

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
	hideGit bool,
) Enricher {
	return &enricher{
		releaser:                 releaser,
		additionalInstructions:   additionalInstructions,
		testCommand:              testCommand,
		validationCommand:        validationCommand,
		validationPromptCriteria: validationPromptCriteria,
		validationPromptResolver: validationPromptResolver,
		hideGit:                  hideGit,
	}
}

type enricher struct {
	releaser                 git.Releaser
	additionalInstructions   string
	testCommand              string
	validationCommand        string
	validationPromptCriteria string
	validationPromptResolver validationprompt.Resolver
	hideGit                  bool
}

// Enrich prepends additionalInstructions and appends machine-parseable suffixes.
func (e *enricher) Enrich(ctx context.Context, content string) string {
	prefix := ""
	if e.additionalInstructions != "" {
		prefix = e.additionalInstructions + "\n\n"
	}
	if e.hideGit {
		prefix = prefix + hideGitGuidanceFragment + "\n\n"
	}
	content = prefix + content
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
