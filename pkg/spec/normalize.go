// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package spec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/specnum"
)

var specNumericPrefixRegexp = regexp.MustCompile(`^\d+-`)

// NormalizeSpecFilename assigns a sequential numeric prefix to name if it does not already have one.
// Scans all given dirs for .md files, finds the highest existing numeric prefix across all dirs,
// and returns fmt.Sprintf("%03d-%s", highest+1, name) for unnumbered names.
// If name already has a valid numeric prefix, it is returned unchanged.
func NormalizeSpecFilename(ctx context.Context, name string, dirs ...string) (string, error) {
	if specnum.Parse(name) >= 0 {
		return name, nil
	}

	highest := 0
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", errors.Wrap(ctx, err, "read dir")
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			n := specnum.Parse(strings.TrimSuffix(e.Name(), ".md"))
			if n > highest {
				highest = n
			}
		}
	}

	return fmt.Sprintf("%03d-%s", highest+1, name), nil
}

// RenumberSpecsAfterRemoval renames all .md files in dir whose numeric prefix is greater
// than removedNum, decrementing each by 1 to close the gap left by the removed spec.
func RenumberSpecsAfterRemoval(ctx context.Context, dir string, removedNum int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrap(ctx, err, "read dir")
	}

	type specEntry struct {
		filename string
		num      int
	}
	var toRename []specEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		n := specnum.Parse(strings.TrimSuffix(e.Name(), ".md"))
		if n > removedNum {
			toRename = append(toRename, specEntry{filename: e.Name(), num: n})
		}
	}

	// Sort ascending so we rename the lowest-numbered spec first to avoid conflicts
	sort.Slice(toRename, func(i, j int) bool {
		return toRename[i].num < toRename[j].num
	})

	for _, se := range toRename {
		stripped := specNumericPrefixRegexp.ReplaceAllString(se.filename, "")
		newName := fmt.Sprintf("%03d-%s", se.num-1, stripped)
		oldPath := filepath.Join(dir, se.filename)
		newPath := filepath.Join(dir, newName)
		if err := os.Rename(oldPath, newPath); err != nil {
			return errors.Wrap(ctx, err, "rename spec file")
		}
	}
	return nil
}
