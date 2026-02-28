// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompt

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bborbe/errors"
	"gopkg.in/yaml.v3"
)

// Prompt represents a prompt file with YAML frontmatter.
type Prompt struct {
	Path   string
	Status string
}

// Frontmatter represents the YAML frontmatter in a prompt file.
type Frontmatter struct {
	Status string `yaml:"status"`
}

// ListQueued scans a directory for .md files with status: queued in frontmatter,
// sorted alphabetically by filename.
func ListQueued(ctx context.Context, dir string) ([]Prompt, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "read directory")
	}

	var queued []Prompt
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		fm, err := readFrontmatter(ctx, path)
		if err != nil {
			// Skip files with invalid frontmatter
			continue
		}

		if fm.Status == "queued" {
			queued = append(queued, Prompt{
				Path:   path,
				Status: fm.Status,
			})
		}
	}

	// Sort alphabetically by filename
	sort.Slice(queued, func(i, j int) bool {
		return filepath.Base(queued[i].Path) < filepath.Base(queued[j].Path)
	})

	return queued, nil
}

// SetStatus updates the status field in a prompt file's frontmatter.
func SetStatus(ctx context.Context, path string, status string) error {
	// #nosec G304 -- path is from ListQueued which scans prompts directory
	content, err := os.ReadFile(path)
	if err != nil {
		return errors.Wrap(ctx, err, "read file")
	}

	// Split frontmatter from content
	parts := bytes.SplitN(content, []byte("---"), 3)
	if len(parts) < 3 {
		return errors.Errorf(ctx, "invalid frontmatter format")
	}

	// Parse frontmatter
	var fm Frontmatter
	if err := yaml.Unmarshal(parts[1], &fm); err != nil {
		return errors.Wrap(ctx, err, "parse frontmatter")
	}

	// Update status
	fm.Status = status

	// Marshal back to YAML
	updated, err := yaml.Marshal(&fm)
	if err != nil {
		return errors.Wrap(ctx, err, "marshal frontmatter")
	}

	// Reconstruct file
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(updated)
	buf.WriteString("---")
	buf.Write(parts[2])

	// Write back
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		return errors.Wrap(ctx, err, "write file")
	}

	return nil
}

// Title extracts the first # heading from a prompt file (below frontmatter).
func Title(ctx context.Context, path string) (string, error) {
	// #nosec G304 -- path is from ListQueued which scans prompts directory
	content, err := os.ReadFile(path)
	if err != nil {
		return "", errors.Wrap(ctx, err, "read file")
	}

	// Skip frontmatter
	parts := bytes.SplitN(content, []byte("---"), 3)
	if len(parts) < 3 {
		return "", errors.Errorf(ctx, "invalid frontmatter format")
	}

	// Find first # heading
	scanner := bufio.NewScanner(bytes.NewReader(parts[2]))
	headingRe := regexp.MustCompile(`^#\s+(.+)$`)

	for scanner.Scan() {
		line := scanner.Text()
		if matches := headingRe.FindStringSubmatch(line); matches != nil {
			return strings.TrimSpace(matches[1]), nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", errors.Wrap(ctx, err, "scan content")
	}

	return "", errors.Errorf(ctx, "no heading found")
}

// Content returns the full file content for passing to Docker.
func Content(ctx context.Context, path string) (string, error) {
	// #nosec G304 -- path is from ListQueued which scans prompts directory
	content, err := os.ReadFile(path)
	if err != nil {
		return "", errors.Wrap(ctx, err, "read file")
	}
	return string(content), nil
}

// MoveToCompleted moves a prompt file to the completed/ subdirectory.
func MoveToCompleted(ctx context.Context, path string) error {
	dir := filepath.Dir(path)
	completedDir := filepath.Join(dir, "completed")

	// Ensure completed/ directory exists
	if err := os.MkdirAll(completedDir, 0750); err != nil {
		return errors.Wrap(ctx, err, "create completed directory")
	}

	// Move file
	filename := filepath.Base(path)
	dest := filepath.Join(completedDir, filename)

	if err := os.Rename(path, dest); err != nil {
		return errors.Wrap(ctx, err, "move file")
	}

	return nil
}

// ReadFrontmatter reads frontmatter from a file.
func ReadFrontmatter(ctx context.Context, path string) (*Frontmatter, error) {
	return readFrontmatter(ctx, path)
}

// readFrontmatter is a helper to read frontmatter from a file.
func readFrontmatter(ctx context.Context, path string) (*Frontmatter, error) {
	// #nosec G304 -- path is from ListQueued which scans prompts directory
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "read file")
	}

	parts := bytes.SplitN(content, []byte("---"), 3)
	if len(parts) < 3 {
		return nil, errors.Errorf(ctx, "invalid frontmatter format")
	}

	var fm Frontmatter
	if err := yaml.Unmarshal(parts[1], &fm); err != nil {
		return nil, errors.Wrap(ctx, err, "parse frontmatter")
	}

	return &fm, nil
}
