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
)

// Summary holds counts of specs grouped by status.
type Summary struct {
	Draft                  int
	Approved               int
	Prompted               int
	Completed              int
	Total                  int
	LinkedPromptsCompleted int `json:"linked_prompts_completed,omitempty"`
	LinkedPromptsTotal     int `json:"linked_prompts_total,omitempty"`
}

// Lister lists spec files from a directory.
//
//counterfeiter:generate -o ../../mocks/spec-lister.go --fake-name Lister . Lister
type Lister interface {
	List(ctx context.Context) ([]*SpecFile, error)
	Summary(ctx context.Context) (*Summary, error)
}

// lister implements Lister.
type lister struct {
	specsDir string
}

// NewLister creates a new Lister that scans the given directory.
func NewLister(specsDir string) Lister {
	return &lister{specsDir: specsDir}
}

// List returns all spec files found in the specs directory.
func (l *lister) List(ctx context.Context) ([]*SpecFile, error) {
	entries, err := os.ReadDir(l.specsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrap(ctx, err, "read specs directory")
	}

	specs := make([]*SpecFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(l.specsDir, entry.Name())
		sf, err := Load(ctx, path)
		if err != nil {
			return nil, errors.Wrap(ctx, err, "load spec file")
		}
		specs = append(specs, sf)
	}

	return specs, nil
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
		case StatusCompleted:
			s.Completed++
		}
	}

	return s, nil
}
