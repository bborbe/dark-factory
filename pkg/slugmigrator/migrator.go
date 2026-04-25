// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package slugmigrator provides migration of bare spec number references in prompt files
// to full spec slugs (e.g. "036" → "036-full-slug-spec-references").
package slugmigrator

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/specnum"
)

//counterfeiter:generate -o ../../mocks/spec-slug-migrator.go --fake-name SpecSlugMigrator . Migrator

// Migrator scans prompt files and replaces bare spec number references with full spec slugs.
type Migrator interface {
	// MigrateDirs scans all .md files in each dir and replaces bare spec number
	// references with full slugs. Skips files that cannot be parsed.
	MigrateDirs(ctx context.Context, promptDirs []string) error
}

// specSlugMigrator implements Migrator.
type specSlugMigrator struct {
	specsDirs     []string
	promptManager PromptManager
}

// NewMigrator creates a new Migrator that resolves bare spec number refs using specs found in specsDirs.
func NewMigrator(
	specsDirs []string,
	promptManager PromptManager,
) Migrator {
	return &specSlugMigrator{
		specsDirs:     specsDirs,
		promptManager: promptManager,
	}
}

type slugEntry struct {
	slug  string
	count int
}

// buildSlugMap scans specsDirs for .md files and returns a map from spec number to full slug.
// If a number appears in more than one file across all dirs, it is omitted (ambiguous).
func buildSlugMap(ctx context.Context, specsDirs []string) map[int]string {
	raw := make(map[int]*slugEntry)
	for _, dir := range specsDirs {
		scanSpecDir(ctx, dir, raw)
	}
	return filterUnambiguous(raw)
}

// scanSpecDir reads one directory and adds spec name entries to raw.
func scanSpecDir(ctx context.Context, dir string, raw map[int]*slugEntry) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.WarnContext(ctx, "failed to read specs dir", "dir", dir, "error", err)
		}
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		recordSpecEntry(ctx, strings.TrimSuffix(e.Name(), ".md"), raw)
	}
}

// recordSpecEntry adds or marks-ambiguous a spec name in raw.
func recordSpecEntry(ctx context.Context, name string, raw map[int]*slugEntry) {
	num := specnum.Parse(name)
	if num < 0 {
		return
	}
	if existing, ok := raw[num]; ok {
		existing.count++
		slog.WarnContext(ctx, "ambiguous spec number found in multiple files",
			"number", num,
			"existing", existing.slug,
			"new", name,
		)
		return
	}
	raw[num] = &slugEntry{slug: name, count: 1}
}

// filterUnambiguous returns a map containing only entries with count == 1.
func filterUnambiguous(raw map[int]*slugEntry) map[int]string {
	result := make(map[int]string, len(raw))
	for num, e := range raw {
		if e.count == 1 {
			result[num] = e.slug
		}
	}
	return result
}

// MigrateDirs scans all .md files in each promptDir and replaces bare spec number references
// with full slugs. Individual file failures are logged and skipped, not returned as errors.
func (m *specSlugMigrator) MigrateDirs(ctx context.Context, promptDirs []string) error {
	slugMap := buildSlugMap(ctx, m.specsDirs)

	for _, dir := range promptDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			slog.WarnContext(ctx, "failed to read prompt dir", "dir", dir, "error", err)
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			if err := m.migrateFile(ctx, path, slugMap); err != nil {
				slog.WarnContext(ctx, "failed to migrate prompt file", "file", path, "error", err)
			}
		}
	}
	return nil
}

// migrateFile loads a prompt file and replaces bare spec number refs with full slugs.
// Returns nil if nothing changed or the file has no spec refs.
func (m *specSlugMigrator) migrateFile(
	ctx context.Context,
	path string,
	slugMap map[int]string,
) error {
	pf, err := m.promptManager.Load(ctx, path)
	if err != nil {
		slog.WarnContext(ctx, "skipping prompt file: failed to load", "file", path, "error", err)
		return nil
	}

	if len(pf.Frontmatter.Specs) == 0 {
		return nil
	}

	updated := make(prompt.SpecList, len(pf.Frontmatter.Specs))
	changedCount := 0

	for i, ref := range pf.Frontmatter.Specs {
		// A bare number ref: parses as a valid number AND has no "-" (no slug suffix)
		num := specnum.Parse(ref)
		if num >= 0 && !strings.Contains(ref, "-") {
			if slug, ok := slugMap[num]; ok {
				updated[i] = slug
				changedCount++
			} else {
				slog.WarnContext(ctx, "spec slug not found, leaving bare ref", "ref", ref, "file", path)
				updated[i] = ref
			}
		} else {
			updated[i] = ref
		}
	}

	if changedCount == 0 {
		return nil
	}

	pf.Frontmatter.Specs = updated
	if err := pf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save prompt file")
	}

	slog.InfoContext(ctx, "migrated spec refs to full slugs",
		"file", filepath.Base(path),
		"updated", changedCount,
	)
	return nil
}
