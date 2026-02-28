// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompt

import (
	"bufio"
	"bytes"
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bborbe/errors"
	"gopkg.in/yaml.v3"
)

// ErrEmptyPrompt is returned when a prompt file is empty or contains only whitespace.
var ErrEmptyPrompt = stderrors.New("prompt file is empty")

// Prompt represents a prompt file with YAML frontmatter.
type Prompt struct {
	Path   string
	Status string
}

// Frontmatter represents the YAML frontmatter in a prompt file.
type Frontmatter struct {
	Status string `yaml:"status"`
}

// ListQueued scans a directory for .md files that should be picked up.
// Files are picked up UNLESS they have an explicit skip status (executing, completed, failed).
// Sorted alphabetically by filename.
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
			// Skip files with read errors
			continue
		}

		// Pick up files UNLESS they have an explicit skip status
		if fm.Status != "executing" && fm.Status != "completed" && fm.Status != "failed" {
			// Normalize status to "queued" for consistency
			status := fm.Status
			if status == "" {
				status = "queued"
			}
			queued = append(queued, Prompt{
				Path:   path,
				Status: status,
			})
		}
	}

	// Sort alphabetically by filename
	sort.Slice(queued, func(i, j int) bool {
		return filepath.Base(queued[i].Path) < filepath.Base(queued[j].Path)
	})

	return queued, nil
}

// ResetExecuting resets any prompts with status "executing" back to "queued".
// This handles prompts that got stuck from a previous crash.
func ResetExecuting(ctx context.Context, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return errors.Wrap(ctx, err, "read directory")
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		fm, err := readFrontmatter(ctx, path)
		if err != nil {
			continue
		}

		if fm.Status == "executing" {
			if err := SetStatus(ctx, path, "queued"); err != nil {
				return errors.Wrap(ctx, err, "reset executing prompt")
			}
		}
	}

	return nil
}

// SetStatus updates the status field in a prompt file's frontmatter.
// If the file has no frontmatter, adds frontmatter with the status field.
func SetStatus(ctx context.Context, path string, status string) error {
	// #nosec G304 -- path is from ListQueued which scans prompts directory
	content, err := os.ReadFile(path)
	if err != nil {
		return errors.Wrap(ctx, err, "read file")
	}

	// Split frontmatter from content
	yamlBytes, body, hasFM := splitFrontmatter(content)

	var updated []byte
	if !hasFM {
		updated, err = addFrontmatterWithStatus(ctx, content, status)
	} else {
		updated, err = updateExistingFrontmatter(ctx, yamlBytes, body, status)
	}

	if err != nil {
		return err
	}

	// Write back
	if err := os.WriteFile(path, updated, 0600); err != nil {
		return errors.Wrap(ctx, err, "write file")
	}

	return nil
}

// addFrontmatterWithStatus adds frontmatter with status to a file that has none.
func addFrontmatterWithStatus(
	ctx context.Context,
	content []byte,
	status string,
) ([]byte, error) {
	fm := Frontmatter{Status: status}
	yamlData, err := yaml.Marshal(&fm)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "marshal frontmatter")
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(yamlData)
	buf.WriteString("---\n")
	buf.Write(content)
	return buf.Bytes(), nil
}

// updateExistingFrontmatter updates status in existing frontmatter.
func updateExistingFrontmatter(
	ctx context.Context,
	yamlBytes []byte,
	body []byte,
	status string,
) ([]byte, error) {
	var fm Frontmatter
	if err := yaml.Unmarshal(yamlBytes, &fm); err != nil {
		return nil, errors.Wrap(ctx, err, "parse frontmatter")
	}

	fm.Status = status

	yamlData, err := yaml.Marshal(&fm)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "marshal frontmatter")
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(yamlData)
	buf.WriteString("---\n")
	buf.Write(body)
	return buf.Bytes(), nil
}

// Title extracts the first # heading from a prompt file.
// Handles files with or without frontmatter.
// If no heading is found, returns the filename without extension.
func Title(ctx context.Context, path string) (string, error) {
	// #nosec G304 -- path is from ListQueued which scans prompts directory
	content, err := os.ReadFile(path)
	if err != nil {
		return "", errors.Wrap(ctx, err, "read file")
	}

	// Skip frontmatter if present
	_, contentToScan, _ := splitFrontmatter(content)

	// Find first # heading
	scanner := bufio.NewScanner(bytes.NewReader(contentToScan))
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

	// No heading found - use filename without extension
	filename := filepath.Base(path)
	return strings.TrimSuffix(filename, ".md"), nil
}

// Content returns the prompt content (without frontmatter) for passing to Docker.
// Returns ErrEmptyPrompt if the file is empty or contains only whitespace.
func Content(ctx context.Context, path string) (string, error) {
	// #nosec G304 -- path is from ListQueued which scans prompts directory
	content, err := os.ReadFile(path)
	if err != nil {
		return "", errors.Wrap(ctx, err, "read file")
	}

	// Strip frontmatter — only pass the body to the executor
	_, body, hasFM := splitFrontmatter(content)
	var result string
	if hasFM {
		result = string(body)
	} else {
		result = string(content)
	}

	// Check if content is empty or only whitespace
	if len(strings.TrimSpace(result)) == 0 {
		return "", ErrEmptyPrompt
	}

	return result, nil
}

// MoveToCompleted sets status to "completed" and moves a prompt file to the completed/ subdirectory.
// This ensures files in completed/ always have the correct status.
func MoveToCompleted(ctx context.Context, path string) error {
	// Set status to completed before moving
	if err := SetStatus(ctx, path, "completed"); err != nil {
		return errors.Wrap(ctx, err, "set completed status")
	}

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

// splitFrontmatter splits file content into frontmatter YAML and body.
// Returns (yamlBytes, body, hasFrontmatter).
// Frontmatter must start with "---\n" at the very beginning of the file
// and end with "\n---\n" on its own line.
func splitFrontmatter(content []byte) ([]byte, []byte, bool) {
	if !bytes.HasPrefix(content, []byte("---\n")) {
		return nil, content, false
	}

	rest := content[4:] // skip opening "---\n"
	idx := bytes.Index(rest, []byte("\n---\n"))
	if idx >= 0 {
		return rest[:idx], rest[idx+4:], true
	}

	// Check for "---" at end of file (no trailing newline)
	if bytes.HasSuffix(rest, []byte("\n---")) {
		return rest[:len(rest)-4], nil, true
	}

	return nil, content, false
}

// readFrontmatter is a helper to read frontmatter from a file.
// Returns empty Frontmatter if file has no frontmatter delimiters.
func readFrontmatter(ctx context.Context, path string) (*Frontmatter, error) {
	// #nosec G304 -- path is from ListQueued which scans prompts directory
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "read file")
	}

	yamlBytes, _, hasFM := splitFrontmatter(content)
	if !hasFM {
		return &Frontmatter{}, nil
	}

	var fm Frontmatter
	if err := yaml.Unmarshal(yamlBytes, &fm); err != nil {
		return nil, errors.Wrap(ctx, err, "parse frontmatter")
	}

	return &fm, nil
}
