// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/spec"
)

//counterfeiter:generate -o ../../mocks/spec-approve-command.go --fake-name SpecApproveCommand . SpecApproveCommand

// SpecApproveCommand executes the spec approve subcommand.
type SpecApproveCommand interface {
	Run(ctx context.Context, args []string) error
}

// specApproveCommand implements SpecApproveCommand.
type specApproveCommand struct {
	specsDir string
}

// NewSpecApproveCommand creates a new SpecApproveCommand.
func NewSpecApproveCommand(specsDir string) SpecApproveCommand {
	return &specApproveCommand{specsDir: specsDir}
}

// Run executes the spec approve command.
func (s *specApproveCommand) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.Errorf(ctx, "spec identifier required")
	}

	id := args[0]
	path, err := s.findSpec(ctx, id)
	if err != nil {
		return err
	}

	sf, err := spec.Load(ctx, path)
	if err != nil {
		return errors.Wrap(ctx, err, "load spec")
	}

	if sf.Frontmatter.Status == string(spec.StatusApproved) {
		return errors.Errorf(ctx, "spec is already approved")
	}

	sf.SetStatus(string(spec.StatusApproved))
	if err := sf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save spec")
	}

	fmt.Printf("approved: %s\n", filepath.Base(path))
	return nil
}

// findSpec finds a spec by exact filename or numeric prefix match.
func (s *specApproveCommand) findSpec(ctx context.Context, id string) (string, error) {
	// Try exact match with .md extension
	if strings.HasSuffix(id, ".md") {
		path := filepath.Join(s.specsDir, id)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	} else {
		// Try as filename without extension
		path := filepath.Join(s.specsDir, id+".md")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// Try prefix match (e.g. "001" matches "001-my-spec.md")
	entries, err := os.ReadDir(s.specsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.Errorf(ctx, "spec not found: %s", id)
		}
		return "", errors.Wrap(ctx, err, "read specs directory")
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, id+"-") ||
			strings.HasPrefix(name, id) && strings.HasSuffix(name, ".md") {
			return filepath.Join(s.specsDir, name), nil
		}
	}

	return "", errors.Errorf(ctx, "spec not found: %s", id)
}
