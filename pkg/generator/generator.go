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

// dockerSpecGenerator implements SpecGenerator using the Docker executor.
type dockerSpecGenerator struct {
	executor              executor.Executor
	inboxDir              string
	completedDir          string
	specsDir              string
	logDir                string
	currentDateTimeGetter libtime.CurrentDateTimeGetter
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

	// e. Snapshot .md files in inboxDir before execution
	beforeFiles, err := listMDFiles(g.inboxDir)
	if err != nil {
		return errors.Wrap(ctx, err, "count inbox files before")
	}

	// d. Execute via executor
	if err := g.executor.Execute(ctx, promptContent, logFile, containerName); err != nil {
		return errors.Wrap(ctx, err, "execute spec generator")
	}

	// f. Snapshot .md files in inboxDir after execution
	afterFiles, err := listMDFiles(g.inboxDir)
	if err != nil {
		return errors.Wrap(ctx, err, "count inbox files after")
	}

	// g. Determine new files
	newFiles := diffFiles(beforeFiles, afterFiles)

	// g. Verify new files were created
	if len(newFiles) == 0 {
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

	// h. Load spec, inherit metadata to prompts, set status to prompted, save
	sf, err := spec.Load(ctx, specPath, g.currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load spec file")
	}

	specBranch := sf.Frontmatter.Branch
	specIssue := sf.Frontmatter.Issue
	if specBranch != "" || specIssue != "" {
		if err := inheritFromSpec(ctx, newFiles, specBranch, specIssue, g.currentDateTimeGetter); err != nil {
			return errors.Wrap(ctx, err, "inherit spec metadata to prompts")
		}
	}

	sf.SetStatus(string(spec.StatusPrompted))
	if err := sf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save spec file")
	}

	return nil
}

// inheritFromSpec copies branch and issue from the spec into each newly created prompt file,
// without overwriting existing values.
func inheritFromSpec(
	ctx context.Context,
	paths []string,
	specBranch string,
	specIssue string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	for _, p := range paths {
		pf, err := prompt.Load(ctx, p, currentDateTimeGetter)
		if err != nil {
			return errors.Wrap(ctx, err, "load prompt for inheritance")
		}
		pf.SetBranchIfEmpty(specBranch)
		pf.SetIssueIfEmpty(specIssue)
		if err := pf.Save(ctx); err != nil {
			return errors.Wrap(ctx, err, "save prompt after inheritance")
		}
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

// listMDFiles returns a set of full .md file paths in the given directory.
// Returns an empty map (not error) when the directory does not exist.
func listMDFiles(dir string) (map[string]struct{}, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]struct{}{}, nil
		}
		return nil, err
	}
	files := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			files[filepath.Join(dir, entry.Name())] = struct{}{}
		}
	}
	return files, nil
}

// diffFiles returns full paths of files present in after but not in before.
func diffFiles(before, after map[string]struct{}) []string {
	var newFiles []string
	for path := range after {
		if _, exists := before[path]; !exists {
			newFiles = append(newFiles, path)
		}
	}
	return newFiles
}
