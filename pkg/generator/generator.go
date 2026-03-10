// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package generator

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"

	"github.com/bborbe/dark-factory/pkg/executor"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/spec"
)

//counterfeiter:generate -o ../../mocks/spec-generator.go --fake-name SpecGenerator . SpecGenerator

// SpecGenerator generates prompt files from a spec file.
type SpecGenerator interface {
	Generate(ctx context.Context, specPath string) error
}

// dockerSpecGenerator implements SpecGenerator using the Docker executor.
type dockerSpecGenerator struct {
	executor              executor.Executor
	inboxDir              string
	completedDir          string
	specsDir              string
	logDir                string
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// NewSpecGenerator creates a new SpecGenerator that runs the /generate-prompts-for-spec command.
func NewSpecGenerator(
	executor executor.Executor,
	inboxDir string,
	completedDir string,
	specsDir string,
	logDir string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) SpecGenerator {
	return &dockerSpecGenerator{
		executor:              executor,
		inboxDir:              inboxDir,
		completedDir:          completedDir,
		specsDir:              specsDir,
		logDir:                logDir,
		currentDateTimeGetter: currentDateTimeGetter,
	}
}

// Generate runs the /generate-prompts-for-spec slash command for the given spec file,
// then transitions the spec status to prompted if new prompt files were created.
func (g *dockerSpecGenerator) Generate(ctx context.Context, specPath string) error {
	// a. Build prompt content
	promptContent := "/generate-prompts-for-spec " + specPath

	// b. Derive container name from spec filename
	specBasename := strings.TrimSuffix(filepath.Base(specPath), ".md")
	containerName := "dark-factory-gen-" + specBasename

	// c. Derive log file path
	logFile := filepath.Join(g.logDir, "gen-"+specBasename+".log")

	// e. Count .md files in inboxDir before execution
	before, err := countMDFiles(g.inboxDir)
	if err != nil {
		return errors.Wrap(ctx, err, "count inbox files before")
	}

	// d. Execute via executor
	if err := g.executor.Execute(ctx, promptContent, logFile, containerName); err != nil {
		return errors.Wrap(ctx, err, "execute spec generator")
	}

	// f. Count .md files in inboxDir after execution
	after, err := countMDFiles(g.inboxDir)
	if err != nil {
		return errors.Wrap(ctx, err, "count inbox files after")
	}

	// g. Verify new files were created
	if after <= before {
		count, err := countCompletedPromptsForSpec(ctx, g.completedDir, specBasename)
		if err != nil {
			return errors.Wrap(ctx, err, "count completed prompts for spec")
		}
		if count > 0 {
			slog.Info(
				"spec already has completed prompts, skipping generation",
				"spec",
				specBasename,
				"count",
				count,
			)
			return nil
		}
		return errors.New(ctx, "generation produced no prompt files")
	}

	// h. Load spec, set status to prompted, save
	sf, err := spec.Load(ctx, specPath, g.currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load spec file")
	}

	sf.SetStatus(string(spec.StatusPrompted))
	if err := sf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save spec file")
	}

	return nil
}

// countCompletedPromptsForSpec counts prompts in completedDir that have specID in their spec field.
func countCompletedPromptsForSpec(
	ctx context.Context,
	completedDir string,
	specID string,
) (int, error) {
	entries, err := os.ReadDir(completedDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(completedDir, entry.Name())
		pf, err := prompt.Load(ctx, path, libtime.NewCurrentDateTime())
		if err != nil {
			slog.Warn("skipping prompt during spec scan", "file", entry.Name(), "error", err)
			continue
		}
		if pf.Frontmatter.HasSpec(specID) {
			count++
		}
	}
	return count, nil
}

// countMDFiles counts the number of .md files in the given directory.
func countMDFiles(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			count++
		}
	}
	return count, nil
}
