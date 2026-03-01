// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompt

import (
	"bufio"
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/bborbe/errors"
	"github.com/bborbe/validation"
	"gopkg.in/yaml.v3"
)

// ErrEmptyPrompt is returned when a prompt file is empty or contains only whitespace.
var ErrEmptyPrompt = stderrors.New("prompt file is empty")

// Status represents the current state of a prompt.
type Status string

const (
	StatusQueued    Status = "queued"
	StatusExecuting Status = "executing"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

// Validate validates the Status value.
func (s Status) Validate(ctx context.Context) error {
	for _, valid := range []Status{StatusQueued, StatusExecuting, StatusCompleted, StatusFailed} {
		if s == valid {
			return nil
		}
	}
	return errors.Wrapf(ctx, validation.Error, "status(%s) is invalid", s)
}

// Rename represents a file rename operation.
type Rename struct {
	OldPath string
	NewPath string
}

// Prompt represents a prompt file with YAML frontmatter.
type Prompt struct {
	Path   string
	Status Status
}

// Validate validates the Prompt struct.
func (p Prompt) Validate(ctx context.Context) error {
	return validation.All{
		validation.Name("path", validation.NotEmptyString(p.Path)),
		validation.Name("status", p.Status),
		validation.Name("filename", validation.HasValidationFunc(func(ctx context.Context) error {
			if !hasNumberPrefix(filepath.Base(p.Path)) {
				return errors.Errorf(ctx, "missing NNN- prefix: %s", filepath.Base(p.Path))
			}
			return nil
		})),
	}.Validate(ctx)
}

// ValidateForExecution validates that a prompt is ready to execute.
func (p Prompt) ValidateForExecution(ctx context.Context) error {
	return validation.All{
		validation.Name("prompt", p),
		validation.Name("status", validation.HasValidationFunc(func(ctx context.Context) error {
			if p.Status != StatusQueued {
				return errors.Errorf(ctx, "expected status queued, got %s", p.Status)
			}
			return nil
		})),
	}.Validate(ctx)
}

// Number extracts the numeric prefix from the prompt filename.
// Returns -1 if the filename has no numeric prefix.
func (p Prompt) Number() int {
	return extractNumberFromFilename(filepath.Base(p.Path))
}

// Frontmatter represents the YAML frontmatter in a prompt file.
type Frontmatter struct {
	Status             string `yaml:"status"`
	Container          string `yaml:"container,omitempty"`
	DarkFactoryVersion string `yaml:"dark-factory-version,omitempty"`
}

// FileMover handles file move operations with git awareness.
//
//counterfeiter:generate -o ../../mocks/file-mover.go --fake-name FileMover . FileMover
type FileMover interface {
	MoveFile(ctx context.Context, oldPath string, newPath string) error
}

// Manager manages prompt file operations.
//
//counterfeiter:generate -o ../../mocks/prompt-manager.go --fake-name Manager . Manager
type Manager interface {
	ResetExecuting(ctx context.Context) error
	HasExecuting(ctx context.Context) bool
	ListQueued(ctx context.Context) ([]Prompt, error)
	ReadFrontmatter(ctx context.Context, path string) (*Frontmatter, error)
	SetStatus(ctx context.Context, path string, status string) error
	SetContainer(ctx context.Context, path string, name string) error
	SetVersion(ctx context.Context, path string, version string) error
	Content(ctx context.Context, path string) (string, error)
	Title(ctx context.Context, path string) (string, error)
	MoveToCompleted(ctx context.Context, path string) error
	NormalizeFilenames(ctx context.Context) ([]Rename, error)
	AllPreviousCompleted(ctx context.Context, n int) bool
}

// manager implements Manager.
type manager struct {
	dir   string
	mover FileMover
}

// NewManager creates a new Manager.
func NewManager(dir string, mover FileMover) Manager {
	return &manager{
		dir:   dir,
		mover: mover,
	}
}

// ResetExecuting resets any prompts with status "executing" back to "queued".
func (pm *manager) ResetExecuting(ctx context.Context) error {
	return ResetExecuting(ctx, pm.dir)
}

// HasExecuting returns true if any prompt in dir has status "executing".
func (pm *manager) HasExecuting(ctx context.Context) bool {
	return HasExecuting(ctx, pm.dir)
}

// ListQueued scans a directory for .md files that should be picked up.
func (pm *manager) ListQueued(ctx context.Context) ([]Prompt, error) {
	return ListQueued(ctx, pm.dir)
}

// ReadFrontmatter reads frontmatter from a file.
func (pm *manager) ReadFrontmatter(ctx context.Context, path string) (*Frontmatter, error) {
	return ReadFrontmatter(ctx, path)
}

// SetStatus updates the status field in a prompt file's frontmatter.
func (pm *manager) SetStatus(ctx context.Context, path string, status string) error {
	return SetStatus(ctx, path, status)
}

// SetContainer updates the container field in a prompt file's frontmatter.
func (pm *manager) SetContainer(ctx context.Context, path string, name string) error {
	return SetContainer(ctx, path, name)
}

// SetVersion updates the dark-factory-version field in a prompt file's frontmatter.
func (pm *manager) SetVersion(ctx context.Context, path string, version string) error {
	return SetVersion(ctx, path, version)
}

// Content returns the prompt content (without frontmatter) for passing to Docker.
func (pm *manager) Content(ctx context.Context, path string) (string, error) {
	return Content(ctx, path)
}

// Title extracts the first # heading from a prompt file.
func (pm *manager) Title(ctx context.Context, path string) (string, error) {
	return Title(ctx, path)
}

// MoveToCompleted sets status to "completed" and moves a prompt file to the completed/ subdirectory.
func (pm *manager) MoveToCompleted(ctx context.Context, path string) error {
	return MoveToCompleted(ctx, path, pm.mover)
}

// NormalizeFilenames scans a directory for .md files and ensures they follow the NNN-slug.md naming convention.
func (pm *manager) NormalizeFilenames(ctx context.Context) ([]Rename, error) {
	return NormalizeFilenames(ctx, pm.dir, pm.mover)
}

// AllPreviousCompleted checks if all prompts with numbers less than n are in completed/.
func (pm *manager) AllPreviousCompleted(ctx context.Context, n int) bool {
	return AllPreviousCompleted(ctx, pm.dir, n)
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
		if fm.Status != string(StatusExecuting) && fm.Status != string(StatusCompleted) &&
			fm.Status != string(StatusFailed) {
			// Normalize status to "queued" for consistency
			status := Status(fm.Status)
			if fm.Status == "" {
				status = StatusQueued
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

		if fm.Status == string(StatusExecuting) {
			if err := SetStatus(ctx, path, string(StatusQueued)); err != nil {
				return errors.Wrap(ctx, err, "reset executing prompt")
			}
		}
	}

	return nil
}

// SetStatus updates the status field in a prompt file's frontmatter.
// If the file has no frontmatter, adds frontmatter with the status field.
func SetStatus(ctx context.Context, path string, status string) error {
	return setField(ctx, path, func(fm *Frontmatter) {
		fm.Status = status
	})
}

// SetContainer updates the container field in a prompt file's frontmatter.
// If the file has no frontmatter, adds frontmatter with the container field.
func SetContainer(ctx context.Context, path string, container string) error {
	return setField(ctx, path, func(fm *Frontmatter) {
		fm.Container = container
	})
}

// SetVersion updates the dark-factory-version field in a prompt file's frontmatter.
// If the file has no frontmatter, adds frontmatter with the version field.
func SetVersion(ctx context.Context, path string, version string) error {
	return setField(ctx, path, func(fm *Frontmatter) {
		fm.DarkFactoryVersion = version
	})
}

// setField updates a field in a prompt file's frontmatter using the provided setter function.
// If the file has no frontmatter, adds frontmatter with the field.
func setField(ctx context.Context, path string, setter func(*Frontmatter)) error {
	// #nosec G304 -- path is from ListQueued which scans prompts directory
	content, err := os.ReadFile(path)
	if err != nil {
		return errors.Wrap(ctx, err, "read file")
	}

	// Split frontmatter from content
	yamlBytes, body, hasFM := splitFrontmatter(content)

	var updated []byte
	if !hasFM {
		updated, err = addFrontmatterWithSetter(ctx, content, setter)
	} else {
		updated, err = updateExistingFrontmatterWithSetter(ctx, yamlBytes, body, setter)
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

// addFrontmatterWithSetter adds frontmatter to a file that has none, using setter to populate fields.
func addFrontmatterWithSetter(
	ctx context.Context,
	content []byte,
	setter func(*Frontmatter),
) ([]byte, error) {
	fm := Frontmatter{}
	setter(&fm)
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

// updateExistingFrontmatterWithSetter updates existing frontmatter using setter to modify fields.
func updateExistingFrontmatterWithSetter(
	ctx context.Context,
	yamlBytes []byte,
	body []byte,
	setter func(*Frontmatter),
) ([]byte, error) {
	var fm Frontmatter
	if err := yaml.Unmarshal(yamlBytes, &fm); err != nil {
		return nil, errors.Wrap(ctx, err, "parse frontmatter")
	}

	setter(&fm)

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
func MoveToCompleted(ctx context.Context, path string, mover FileMover) error {
	// Set status to completed before moving
	if err := SetStatus(ctx, path, string(StatusCompleted)); err != nil {
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

	if err := mover.MoveFile(ctx, path, dest); err != nil {
		return errors.Wrap(ctx, err, "move file")
	}

	return nil
}

// ReadFrontmatter reads frontmatter from a file.
func ReadFrontmatter(ctx context.Context, path string) (*Frontmatter, error) {
	return readFrontmatter(ctx, path)
}

// HasExecuting returns true if any prompt in dir has status "executing".
func HasExecuting(ctx context.Context, dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		fm, err := readFrontmatter(ctx, filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		if fm.Status == string(StatusExecuting) {
			return true
		}
	}
	return false
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

// fileInfo represents information about a prompt file.
type fileInfo struct {
	name   string
	number int
	slug   string
}

// NormalizeFilenames scans a directory for .md files and ensures they follow the NNN-slug.md naming convention.
// Files are renamed if they:
// - Have no numeric prefix (gets next available number)
// - Have a duplicate number (later file gets next available number)
// - Have wrong format (e.g., 9-foo.md instead of 009-foo.md)
// Returns list of renames performed.
func NormalizeFilenames(ctx context.Context, dir string, mover FileMover) ([]Rename, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "read directory")
	}

	files, usedNumbers := scanPromptFiles(entries)

	// Also collect numbers used in completed/ so we don't assign duplicates.
	completedEntries, err := os.ReadDir(filepath.Join(dir, "completed"))
	if err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrap(ctx, err, "read completed directory")
	}
	_, completedNumbers := scanPromptFiles(completedEntries)
	for n := range completedNumbers {
		usedNumbers[n] = true
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].name < files[j].name
	})

	return renameInvalidFiles(ctx, dir, files, usedNumbers, mover)
}

// scanPromptFiles scans directory entries and extracts file information.
func scanPromptFiles(entries []os.DirEntry) ([]fileInfo, map[int]bool) {
	validPattern := regexp.MustCompile(`^(\d{3})-(.+)\.md$`)
	numericPattern := regexp.MustCompile(`^(\d+)-(.+)\.md$`)

	files := make([]fileInfo, 0, len(entries))
	usedNumbers := make(map[int]bool)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		info := parseFilename(entry.Name(), validPattern, numericPattern)
		files = append(files, info)

		if info.number != -1 {
			usedNumbers[info.number] = true
		}
	}

	return files, usedNumbers
}

// parseFilename extracts number and slug from a filename.
func parseFilename(
	name string,
	validPattern *regexp.Regexp,
	numericPattern *regexp.Regexp,
) fileInfo {
	// Check if file matches valid pattern (3-digit)
	if matches := validPattern.FindStringSubmatch(name); matches != nil {
		num := 0
		_, _ = fmt.Sscanf(matches[1], "%d", &num)
		return fileInfo{name: name, number: num, slug: matches[2]}
	}

	// Check if file has any numeric prefix (wrong format or needs normalization)
	if matches := numericPattern.FindStringSubmatch(name); matches != nil {
		num := 0
		_, _ = fmt.Sscanf(matches[1], "%d", &num)
		return fileInfo{name: name, number: num, slug: matches[2]}
	}

	// No numeric prefix - assign -1 as placeholder
	slug := strings.TrimSuffix(name, ".md")
	return fileInfo{name: name, number: -1, slug: slug}
}

// renameInvalidFiles processes files and renames those that don't meet the naming convention.
func renameInvalidFiles(
	ctx context.Context,
	dir string,
	files []fileInfo,
	usedNumbers map[int]bool,
	mover FileMover,
) ([]Rename, error) {
	var renames []Rename
	seenNumbers := make(map[int]string)

	for _, f := range files {
		newNumber, needsRename := determineRename(f, seenNumbers, usedNumbers)

		if needsRename {
			rename, err := performRename(ctx, dir, f, newNumber, mover)
			if err != nil {
				return nil, err
			}
			renames = append(renames, rename)
			seenNumbers[newNumber] = rename.NewPath
		} else {
			seenNumbers[f.number] = f.name
		}
	}

	return renames, nil
}

// determineRename checks if a file needs to be renamed and returns the new number.
func determineRename(
	f fileInfo,
	seenNumbers map[int]string,
	usedNumbers map[int]bool,
) (int, bool) {
	// Case 1: No numeric prefix
	if f.number == -1 {
		newNum := findNextAvailableNumber(usedNumbers)
		usedNumbers[newNum] = true
		return newNum, true
	}

	// Case 2: Duplicate number
	if _, exists := seenNumbers[f.number]; exists {
		newNum := findNextAvailableNumber(usedNumbers)
		usedNumbers[newNum] = true
		return newNum, true
	}

	// Case 3: Wrong format
	expectedName := fmt.Sprintf("%03d-%s.md", f.number, f.slug)
	if f.name != expectedName {
		return f.number, true
	}

	return f.number, false
}

// findNextAvailableNumber finds the next unused number.
func findNextAvailableNumber(usedNumbers map[int]bool) int {
	for i := 1; ; i++ {
		if !usedNumbers[i] {
			return i
		}
	}
}

// performRename renames a file to match the naming convention.
func performRename(
	ctx context.Context,
	dir string,
	f fileInfo,
	newNumber int,
	mover FileMover,
) (Rename, error) {
	oldPath := filepath.Join(dir, f.name)
	newName := fmt.Sprintf("%03d-%s.md", newNumber, f.slug)
	newPath := filepath.Join(dir, newName)

	if err := mover.MoveFile(ctx, oldPath, newPath); err != nil {
		return Rename{}, errors.Wrap(ctx, err, "rename file")
	}

	return Rename{OldPath: oldPath, NewPath: newPath}, nil
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

// hasNumberPrefix checks if a filename has a numeric prefix (NNN-).
func hasNumberPrefix(filename string) bool {
	pattern := regexp.MustCompile(`^\d{3}-`)
	return pattern.MatchString(filename)
}

// extractNumberFromFilename extracts the numeric prefix from a filename.
// Returns -1 if the filename has no numeric prefix.
func extractNumberFromFilename(filename string) int {
	pattern := regexp.MustCompile(`^(\d{3})-`)
	matches := pattern.FindStringSubmatch(filename)
	if matches == nil {
		return -1
	}
	num, err := strconv.Atoi(matches[1])
	if err != nil {
		return -1
	}
	return num
}

// AllPreviousCompleted checks if all prompts with numbers less than n are in completed/.
func AllPreviousCompleted(ctx context.Context, dir string, n int) bool {
	if n <= 1 {
		return true // No previous prompts to check
	}

	completedDir := filepath.Join(dir, "completed")
	completedEntries, err := os.ReadDir(completedDir)
	if err != nil {
		return false // completed/ doesn't exist or can't be read
	}

	// Collect all completed numbers
	completedNumbers := make(map[int]bool)
	for _, entry := range completedEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		num := extractNumberFromFilename(entry.Name())
		if num != -1 {
			completedNumbers[num] = true
		}
	}

	// Check that all numbers 1 through n-1 are completed
	for i := 1; i < n; i++ {
		if !completedNumbers[i] {
			return false
		}
	}

	return true
}
