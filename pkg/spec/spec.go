// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package spec

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/adrg/frontmatter"
	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"
	"gopkg.in/yaml.v3"

	"github.com/bborbe/dark-factory/pkg/notifier"
	"github.com/bborbe/dark-factory/pkg/prompt"
	"github.com/bborbe/dark-factory/pkg/specnum"
)

// Status represents the lifecycle state of a spec.
type Status string

const (
	// StatusDraft indicates the spec has been written but not yet reviewed.
	StatusDraft Status = "draft"
	// StatusApproved indicates the spec has been reviewed and approved.
	StatusApproved Status = "approved"
	// StatusPrompted indicates prompts have been generated from the spec.
	StatusPrompted Status = "prompted"
	// StatusVerifying indicates all prompts completed, awaiting human verification.
	StatusVerifying Status = "verifying"
	// StatusCompleted indicates human verified all acceptance criteria.
	StatusCompleted Status = "completed"
)

// Frontmatter represents the YAML frontmatter in a spec file.
type Frontmatter struct {
	Status    string   `yaml:"status"`
	Tags      []string `yaml:"tags,omitempty"`
	Approved  string   `yaml:"approved,omitempty"`
	Prompted  string   `yaml:"prompted,omitempty"`
	Verifying string   `yaml:"verifying,omitempty"`
	Completed string   `yaml:"completed,omitempty"`
	Branch    string   `yaml:"branch,omitempty"`
	Issue     string   `yaml:"issue,omitempty"`
}

// SpecFile represents a loaded spec file with frontmatter and body.
//
//nolint:revive // SpecFile is the intended name per requirements
type SpecFile struct {
	Path                  string
	Frontmatter           Frontmatter
	Name                  string // filename without extension
	Body                  []byte
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

func (s *SpecFile) now() time.Time {
	return time.Time(s.currentDateTimeGetter.Now())
}

// SpecNumber returns the numeric prefix of the spec file name.
// Returns -1 if the name has no numeric prefix.
func (s *SpecFile) SpecNumber() int {
	return specnum.Parse(s.Name)
}

// stampOnce sets *field to the current UTC RFC3339 timestamp only if *field is empty.
func (s *SpecFile) stampOnce(field *string) {
	if *field == "" {
		*field = s.now().UTC().Format(time.RFC3339)
	}
}

// Load reads a spec file from disk, parsing frontmatter and body.
func Load(
	ctx context.Context,
	path string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (*SpecFile, error) {
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
			Path:                  path,
			Name:                  name,
			Body:                  content,
			currentDateTimeGetter: currentDateTimeGetter,
		}, nil
	}

	return &SpecFile{
		Path:                  path,
		Frontmatter:           fm,
		Name:                  name,
		Body:                  body,
		currentDateTimeGetter: currentDateTimeGetter,
	}, nil
}

// SetStatus sets the status field in the frontmatter and stamps the matching timestamp once.
func (s *SpecFile) SetStatus(status string) {
	s.Frontmatter.Status = status
	switch Status(status) {
	case StatusApproved:
		s.stampOnce(&s.Frontmatter.Approved)
	case StatusPrompted:
		s.stampOnce(&s.Frontmatter.Prompted)
	case StatusVerifying:
		s.stampOnce(&s.Frontmatter.Verifying)
	case StatusCompleted:
		s.stampOnce(&s.Frontmatter.Completed)
	}
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
	s.SetStatus(string(StatusCompleted))
}

// SetBranchIfEmpty sets the branch in the frontmatter only if it is not already set.
func (s *SpecFile) SetBranchIfEmpty(branch string) {
	if s.Frontmatter.Branch == "" {
		s.Frontmatter.Branch = branch
	}
}

var unsafeBranchPattern = regexp.MustCompile(`\.\.|^-|[\s` + "`" + `$()|\&;<>!]`)

// ValidateBranchName returns an error if branch contains characters unsafe for git checkout.
// An empty branch is always valid (the field is optional).
func ValidateBranchName(ctx context.Context, branch string) error {
	if branch == "" {
		return nil
	}
	if unsafeBranchPattern.MatchString(branch) {
		return errors.Errorf(ctx, "branch name %q contains invalid characters", branch)
	}
	return nil
}

var safeBranchCharPattern = regexp.MustCompile(`[^a-zA-Z0-9\-_/]`)

// AutoBranchName generates a canonical branch name from a spec file's basename (without .md extension).
// If the name has a numeric prefix, it returns "dark-factory/spec-NNN".
// Otherwise it returns "dark-factory/" + sanitized name.
func AutoBranchName(name string) string {
	num := specnum.Parse(name)
	if num >= 0 {
		return fmt.Sprintf("dark-factory/spec-%03d", num)
	}
	safe := safeBranchCharPattern.ReplaceAllString(name, "-")
	return "dark-factory/" + safe
}

// MarkVerifying sets the spec status to verifying.
func (s *SpecFile) MarkVerifying() {
	s.SetStatus(string(StatusVerifying))
}

//counterfeiter:generate -o ../../mocks/spec-auto-completer.go --fake-name AutoCompleter . AutoCompleter

// AutoCompleter checks if all linked prompts are completed and marks the spec as completed.
type AutoCompleter interface {
	CheckAndComplete(ctx context.Context, specID string) error
}

// NewAutoCompleter creates a new AutoCompleter.
func NewAutoCompleter(
	queueDir, completedDir, specsInboxDir, specsInProgressDir, specsCompletedDir string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
	projectName string,
	n notifier.Notifier,
) AutoCompleter {
	return &autoCompleter{
		queueDir:              queueDir,
		completedDir:          completedDir,
		specsInboxDir:         specsInboxDir,
		specsInProgressDir:    specsInProgressDir,
		specsCompletedDir:     specsCompletedDir,
		currentDateTimeGetter: currentDateTimeGetter,
		projectName:           projectName,
		notifier:              n,
	}
}

// autoCompleter implements AutoCompleter.
type autoCompleter struct {
	queueDir              string
	completedDir          string
	specsInboxDir         string
	specsInProgressDir    string
	specsCompletedDir     string
	currentDateTimeGetter libtime.CurrentDateTimeGetter
	projectName           string
	notifier              notifier.Notifier
}

// findSpecFile searches dirs for a spec file matching specID.
// Matches by numeric prefix first ("019" finds "019-review-fix-loop.md"),
// falling back to exact "specID.md" lookup for non-numeric IDs.
func findSpecFile(dirs []string, specID string) string {
	specNum := specnum.Parse(specID)
	for _, dir := range dirs {
		if p := findSpecFileInDir(dir, specID, specNum); p != "" {
			return p
		}
	}
	return ""
}

// findSpecFileInDir searches a single directory for a spec file matching specID.
func findSpecFileInDir(dir, specID string, specNum int) string {
	if specNum >= 0 {
		if p := findByNumericPrefix(dir, specNum); p != "" {
			return p
		}
	}
	candidate := filepath.Join(dir, specID+".md")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

// findByNumericPrefix scans dir for a .md file whose name numeric prefix equals specNum.
func findByNumericPrefix(dir string, specNum int) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if specnum.Parse(strings.TrimSuffix(e.Name(), ".md")) == specNum {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
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

	// All linked prompts are completed — find the spec file in any of the three spec dirs.
	dirs := []string{a.specsInboxDir, a.specsInProgressDir, a.specsCompletedDir}
	specPath := findSpecFile(dirs, specID)
	if specPath == "" {
		slog.Warn("spec file not found in any spec dir", "specID", specID)
		return nil
	}

	sf, err := Load(ctx, specPath, a.currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load spec file")
	}

	if sf.Frontmatter.Status == string(StatusCompleted) ||
		sf.Frontmatter.Status == string(StatusVerifying) {
		// Already completed or awaiting verification — no-op
		return nil
	}

	sf.MarkVerifying()
	if err := sf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save spec file")
	}

	_ = a.notifier.Notify(ctx, notifier.Event{
		ProjectName: a.projectName,
		EventType:   "spec_verifying",
		PromptName:  sf.Name,
	})

	slog.Info("spec awaiting verification", "spec", specID)
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
		pf, err := prompt.Load(ctx, path, a.currentDateTimeGetter)
		if err != nil {
			slog.Warn("skipping prompt during spec scan", "file", entry.Name(), "error", err)
			continue
		}

		if !pf.Frontmatter.HasSpec(specID) {
			continue
		}

		found = true
		if pf.Frontmatter.Status != string(prompt.CompletedPromptStatus) {
			return false, true, nil
		}
	}

	return true, found, nil
}
