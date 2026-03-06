// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package spec

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/frontmatter"
	"github.com/bborbe/errors"
	"gopkg.in/yaml.v3"

	"github.com/bborbe/dark-factory/pkg/prompt"
)

// Status represents the lifecycle state of a spec.
type Status string

const (
	StatusDraft     Status = "draft"
	StatusApproved  Status = "approved"
	StatusPrompted  Status = "prompted"
	StatusCompleted Status = "completed"
)

// Frontmatter represents the YAML frontmatter in a spec file.
type Frontmatter struct {
	Status string   `yaml:"status"`
	Tags   []string `yaml:"tags,omitempty"`
}

// SpecFile represents a loaded spec file with frontmatter and body.
//
//nolint:revive // SpecFile is the intended name per requirements
type SpecFile struct {
	Path        string
	Frontmatter Frontmatter
	Name        string // filename without extension
	Body        []byte
}

// Load reads a spec file from disk, parsing frontmatter and body.
func Load(ctx context.Context, path string) (*SpecFile, error) {
	// #nosec G304 -- path is from caller who controls the specs directory
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "read spec file")
	}

	name := strings.TrimSuffix(filepath.Base(path), ".md")

	var fm Frontmatter
	body, err := frontmatter.Parse(bytes.NewReader(content), &fm)
	if err != nil {
		return &SpecFile{
			Path: path,
			Name: name,
			Body: content,
		}, nil
	}

	return &SpecFile{
		Path:        path,
		Frontmatter: fm,
		Name:        name,
		Body:        body,
	}, nil
}

// SetStatus sets the status field in the frontmatter.
func (s *SpecFile) SetStatus(status string) {
	s.Frontmatter.Status = status
}

// Save writes the spec file back to disk.
func (s *SpecFile) Save(ctx context.Context) error {
	fm, err := yaml.Marshal(&s.Frontmatter)
	if err != nil {
		return errors.Wrap(ctx, err, "marshal frontmatter")
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(fm)
	buf.WriteString("---\n")
	buf.Write(s.Body)

	if err := os.WriteFile(s.Path, buf.Bytes(), 0600); err != nil {
		return errors.Wrap(ctx, err, "write spec file")
	}

	return nil
}

// MarkCompleted sets the spec status to completed.
func (s *SpecFile) MarkCompleted() {
	s.Frontmatter.Status = string(StatusCompleted)
}

// AutoCompleter checks if all linked prompts are completed and marks the spec as completed.
//
//counterfeiter:generate -o ../../mocks/spec-auto-completer.go --fake-name AutoCompleter . AutoCompleter
type AutoCompleter interface {
	CheckAndComplete(ctx context.Context, specID string) error
}

// autoCompleter implements AutoCompleter.
type autoCompleter struct {
	queueDir     string
	completedDir string
	specsDir     string
}

// NewAutoCompleter creates a new AutoCompleter.
func NewAutoCompleter(queueDir, completedDir, specsDir string) AutoCompleter {
	return &autoCompleter{
		queueDir:     queueDir,
		completedDir: completedDir,
		specsDir:     specsDir,
	}
}

// CheckAndComplete checks if all prompts linked to specID are completed.
// If so, it marks the spec file as completed.
// No-op if specID is empty or if the spec is already completed.
func (a *autoCompleter) CheckAndComplete(ctx context.Context, specID string) error {
	if specID == "" {
		return nil
	}

	// Scan both queue and completed dirs for prompts matching this spec
	allCompleted, found, err := a.allLinkedPromptsCompleted(ctx, specID)
	if err != nil {
		return errors.Wrap(ctx, err, "scan prompts for spec")
	}

	if !found {
		// No prompts reference this spec — nothing to do
		return nil
	}

	if !allCompleted {
		return nil
	}

	// All linked prompts are completed — mark the spec as completed
	specPath := filepath.Join(a.specsDir, specID+".md")
	sf, err := Load(ctx, specPath)
	if err != nil {
		return errors.Wrap(ctx, err, "load spec file")
	}

	if sf.Frontmatter.Status == string(StatusCompleted) {
		// Already completed — no-op
		return nil
	}

	sf.MarkCompleted()
	if err := sf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save spec file")
	}

	slog.Info("spec auto-completed", "spec", specID)
	return nil
}

// allLinkedPromptsCompleted scans both queue and completed directories for prompts
// with the given spec field. Returns (allCompleted, found, error).
func (a *autoCompleter) allLinkedPromptsCompleted(
	ctx context.Context,
	specID string,
) (bool, bool, error) {
	found := false

	for _, dir := range []string{a.queueDir, a.completedDir} {
		allDone, dirFound, err := a.scanDirForSpec(ctx, dir, specID)
		if err != nil {
			return false, false, err
		}
		if dirFound {
			found = true
		}
		if !allDone {
			return false, true, nil
		}
	}

	return true, found, nil
}

// scanDirForSpec scans a single directory for prompts linked to specID.
// Returns (allCompleted, found, error).
func (a *autoCompleter) scanDirForSpec(
	ctx context.Context,
	dir string,
	specID string,
) (bool, bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return true, false, nil
		}
		return false, false, errors.Wrap(ctx, err, "read directory")
	}

	found := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		pf, err := prompt.Load(ctx, path)
		if err != nil {
			slog.Warn("skipping prompt during spec scan", "file", entry.Name(), "error", err)
			continue
		}

		if pf.Frontmatter.Spec != specID {
			continue
		}

		found = true
		if pf.Frontmatter.Status != string(prompt.StatusCompleted) {
			return false, true, nil
		}
	}

	return true, found, nil
}
