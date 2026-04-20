// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scenario

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/specnum"
)

// Summary holds counts of scenarios grouped by status.
type Summary struct {
	Idea     int
	Draft    int
	Active   int
	Outdated int
	Unknown  int
	Total    int
}

//counterfeiter:generate -o ../../mocks/scenario-lister.go --fake-name ScenarioLister . Lister

// Lister lists scenario files from a directory.
type Lister interface {
	List(ctx context.Context) ([]*ScenarioFile, error)
	Summary(ctx context.Context) (*Summary, error)
	Find(ctx context.Context, id string) ([]*ScenarioFile, error)
}

// NewLister creates a Lister that scans the given directory.
func NewLister(dir string) Lister {
	return &lister{dir: dir}
}

type lister struct {
	dir string
}

// List returns all scenario files in l.dir whose names match NNN-*.md, sorted by Number ascending.
// Returns an empty slice (no error) when the directory does not exist.
func (l *lister) List(ctx context.Context) ([]*ScenarioFile, error) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrap(ctx, err, "read scenarios directory")
	}

	var files []*ScenarioFile
	for _, entry := range entries {
		if entry.IsDir() || !filenameRe.MatchString(entry.Name()) {
			continue
		}
		path := filepath.Join(l.dir, entry.Name())
		sf, err := Load(ctx, path)
		if err != nil {
			slog.Warn("skipping scenario file", "file", entry.Name(), "error", err)
			continue
		}
		files = append(files, sf)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Number < files[j].Number
	})
	return files, nil
}

// Summary returns counts of scenario files grouped by status.
func (l *lister) Summary(ctx context.Context) (*Summary, error) {
	files, err := l.List(ctx)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "list scenarios")
	}
	s := &Summary{Total: len(files)}
	for _, sf := range files {
		switch Status(sf.Frontmatter.Status) {
		case StatusIdea:
			s.Idea++
		case StatusDraft:
			s.Draft++
		case StatusActive:
			s.Active++
		case StatusOutdated:
			s.Outdated++
		default:
			s.Unknown++
		}
	}
	return s, nil
}

// Find returns all scenarios in l.dir whose number or name matches id.
//
// Matching rules:
//   - If id parses as a number (specnum.Parse(id) >= 0), return files with that Number.
//   - Otherwise, return files whose Name contains id as a substring (case-sensitive).
//
// Returns an empty slice (not an error) when nothing matches.
func (l *lister) Find(ctx context.Context, id string) ([]*ScenarioFile, error) {
	files, err := l.List(ctx)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "list scenarios for find")
	}

	num := specnum.Parse(id)
	var matches []*ScenarioFile
	for _, sf := range files {
		if num >= 0 {
			if sf.Number == num {
				matches = append(matches, sf)
			}
		} else {
			if strings.Contains(sf.Name, id) {
				matches = append(matches, sf)
			}
		}
	}
	return matches, nil
}
