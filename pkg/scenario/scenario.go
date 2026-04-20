// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package scenario

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/adrg/frontmatter"
	"github.com/bborbe/errors"
	"gopkg.in/yaml.v3"

	"github.com/bborbe/dark-factory/pkg/specnum"
)

// Status represents the lifecycle state of a scenario.
type Status string

const (
	// StatusIdea indicates a rough concept not yet ready for execution.
	StatusIdea Status = "idea"
	// StatusDraft indicates a scenario that is written but not yet exercised.
	StatusDraft Status = "draft"
	// StatusActive indicates a scenario that is currently in use for regression coverage.
	StatusActive Status = "active"
	// StatusOutdated indicates a scenario that no longer reflects the current behavior.
	StatusOutdated Status = "outdated"
)

// KnownStatuses is the set of valid scenario status values.
var KnownStatuses = []Status{StatusIdea, StatusDraft, StatusActive, StatusOutdated}

// IsKnown returns true if s is one of the four defined scenario statuses.
func IsKnown(s Status) bool {
	for _, k := range KnownStatuses {
		if s == k {
			return true
		}
	}
	return false
}

// Frontmatter represents the YAML frontmatter in a scenario file.
type Frontmatter struct {
	Status string `yaml:"status"`
}

// ScenarioFile represents a loaded scenario file.
type ScenarioFile struct {
	Path        string
	Name        string // filename without .md extension, e.g. "001-workflow-direct"
	Number      int    // numeric prefix, -1 if none
	Frontmatter Frontmatter
	Title       string // text from the first "# " heading after frontmatter, empty if absent
	RawContent  []byte // full file bytes (used by show command to print entire file)
}

// filenameRe matches files that follow the NNN-*.md convention (one or more leading digits).
var filenameRe = regexp.MustCompile(`^\d+-.*\.md$`)

// Load reads one scenario file from disk. On frontmatter parse failure the file is still
// returned with an empty Frontmatter — callers treat an empty/unrecognized status as unknown.
// Returns an error only if the file cannot be read at all.
func Load(ctx context.Context, path string) (*ScenarioFile, error) {
	// #nosec G304 -- path is from caller who controls the scenarios directory
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "read scenario file")
	}

	name := strings.TrimSuffix(filepath.Base(path), ".md")
	number := specnum.Parse(name)

	var fm Frontmatter
	yamlV3Format := frontmatter.NewFormat("---", "---", yaml.Unmarshal)
	body, fmErr := frontmatter.Parse(bytes.NewReader(content), &fm, yamlV3Format)
	if fmErr != nil {
		// Frontmatter missing or malformed — leave fm empty, body = full content
		body = content
		fm = Frontmatter{}
	}

	return &ScenarioFile{
		Path:        path,
		Name:        name,
		Number:      number,
		Frontmatter: fm,
		Title:       extractTitle(body),
		RawContent:  content,
	}, nil
}

// extractTitle returns the text of the first "# " heading found in body, or "" if none.
func extractTitle(body []byte) string {
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}
