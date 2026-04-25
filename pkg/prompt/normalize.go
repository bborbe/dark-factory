// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompt

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bborbe/errors"
)

var (
	validPatternRegexp   = regexp.MustCompile(`^(\d{3})-(.+)\.md$`)
	numericPatternRegexp = regexp.MustCompile(`^(\d+)-(.+)\.md$`)
)

// fileInfo represents information about a prompt file.
type fileInfo struct {
	name   string
	number int
	slug   string
}

// normalizeFilenames scans a directory for .md files and ensures they follow the NNN-slug.md naming convention.
// Files are renamed if they:
// - Have no numeric prefix (gets next available number)
// - Have a duplicate number (later file gets next available number)
// - Have wrong format (e.g., 9-foo.md instead of 009-foo.md)
// Returns list of renames performed.
func normalizeFilenames(
	ctx context.Context,
	dir string,
	completedDir string,
	mover FileMover,
) ([]Rename, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "read directory")
	}

	files, usedNumbers := scanPromptFiles(entries)

	// Also collect numbers used in completed/ so we don't assign duplicates.
	completedEntries, err := os.ReadDir(completedDir)
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
	files := make([]fileInfo, 0, len(entries))
	usedNumbers := make(map[int]bool)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		info := parseFilename(entry.Name(), validPatternRegexp, numericPatternRegexp)
		files = append(files, info)

		// Only claim the number if the file is already properly formatted (NNN-slug.md).
		// Wrong-format files (e.g. 01-foo.md) have a parsed number but haven't earned it yet.
		if info.number != -1 && validPatternRegexp.MatchString(entry.Name()) {
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
		if usedNumbers[f.number] {
			newNum := findNextAvailableNumber(usedNumbers)
			usedNumbers[newNum] = true
			return newNum, true
		}
		usedNumbers[f.number] = true
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

	slog.Debug("normalizing filename", "from", f.name, "to", newName, "number", newNumber)

	if err := mover.MoveFile(ctx, oldPath, newPath); err != nil {
		return Rename{}, errors.Wrap(ctx, err, "rename file")
	}

	return Rename{OldPath: oldPath, NewPath: newPath}, nil
}
