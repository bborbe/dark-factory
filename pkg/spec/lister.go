// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package spec

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"
)

// Summary holds counts of specs grouped by status.
type Summary struct {
	Draft                  int
	Approved               int
	Prompted               int
	Verifying              int
	Completed              int
	Total                  int
	LinkedPromptsCompleted int `json:"linked_prompts_completed,omitempty"`
	LinkedPromptsTotal     int `json:"linked_prompts_total,omitempty"`
}

//counterfeiter:generate -o ../../mocks/spec-lister.go --fake-name Lister . Lister

// Lister lists spec files from a directory.
type Lister interface {
	List(ctx context.Context) ([]*SpecFile, error)
	Summary(ctx context.Context) (*Summary, error)
}

// lister implements Lister.
type lister struct {
	dirs []string
}

// NewLister creates a new Lister that scans the given directories.
func NewLister(dirs ...string) Lister {
	return &lister{dirs: dirs}
}

// List returns all spec files found across all configured directories.
func (l *lister) List(ctx context.Context) ([]*SpecFile, error) {
	var all []*SpecFile
	for _, dir := range l.dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, errors.Wrap(ctx, err, "read specs directory")
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			sf, err := Load(ctx, path, libtime.NewCurrentDateTime())
			if err != nil {
				return nil, errors.Wrap(ctx, err, "load spec file")
			}
			all = append(all, sf)
		}
	}
	return all, nil
}

// Summary returns counts of specs grouped by status.
func (l *lister) Summary(ctx context.Context) (*Summary, error) {
	specs, err := l.List(ctx)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "list specs")
	}

	s := &Summary{Total: len(specs)}
	for _, sf := range specs {
		switch Status(sf.Frontmatter.Status) {
		case StatusDraft:
			s.Draft++
		case StatusApproved:
			s.Approved++
		case StatusPrompted:
			s.Prompted++
		case StatusVerifying:
			s.Verifying++
		case StatusCompleted:
			s.Completed++
		}
	}

	return s, nil
}
