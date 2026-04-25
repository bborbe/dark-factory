// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package promptenricher

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/dark-factory/pkg/git"
	"github.com/bborbe/dark-factory/pkg/report"
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
) Enricher {
	return &enricher{
		releaser:                 releaser,
		additionalInstructions:   additionalInstructions,
		testCommand:              testCommand,
		validationCommand:        validationCommand,
		validationPromptCriteria: validationPromptCriteria,
	}
}

type enricher struct {
	releaser                 git.Releaser
	additionalInstructions   string
	testCommand              string
	validationCommand        string
	validationPromptCriteria string
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
	if criteria, ok := resolveValidationPrompt(ctx, e.validationPromptCriteria); ok {
		content = content + report.ValidationPromptSuffix(criteria)
	}
	return content
}

// resolveValidationPrompt resolves the validationPrompt config value.
// If value is a relative path to an existing file, the file contents are returned.
// If value is non-empty but the file does not exist, ("", false) is returned (caller logs warning).
// If value is empty, ("", false) is returned silently.
// The resolved result is the criteria text to inject, or empty string to skip injection.
func resolveValidationPrompt(ctx context.Context, value string) (string, bool) {
	if value == "" {
		return "", false
	}
	// Check if value is a path to an existing file
	if _, err := os.Stat(value); err == nil {
		data, readErr := os.ReadFile(
			value,
		) // #nosec G304 -- path is validated by config (no absolute path, no .. traversal)
		if readErr != nil {
			slog.WarnContext(
				ctx,
				"failed to read validationPrompt file",
				"path",
				value,
				"error",
				readErr,
			)
			return "", false
		}
		return string(data), true
	}
	// Check if value looks like a file path (contains path separator or .md extension)
	// and the file doesn't exist — log a warning
	if strings.Contains(value, string(filepath.Separator)) || strings.HasSuffix(value, ".md") {
		slog.WarnContext(
			ctx,
			"validationPrompt file not found, skipping criteria evaluation",
			"path",
			value,
		)
		return "", false
	}
	// Value is inline criteria text
	return value, true
}
