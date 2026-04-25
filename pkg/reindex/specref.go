// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reindex

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/bborbe/errors"

	"github.com/bborbe/dark-factory/pkg/specnum"
)

var specFilenamePatternRegexp = regexp.MustCompile(`spec-(\d{3})`)

// UpdateSpecRefs propagates spec file renames to prompt files.
//
// For each spec rename (old→new), it:
//  1. Updates the frontmatter `spec:` field in any prompt file that references the old spec number.
//  2. Renames any prompt file whose filename contains `spec-NNN` where NNN matches the old spec number.
//
// promptDirs are scanned for .md files. mover is used for file renames.
// pm is used to load prompt files.
// Returns the list of prompt file renames performed.
func UpdateSpecRefs(
	ctx context.Context,
	specRenames []Rename,
	promptDirs []string,
	mover FileMover,
	pm PromptManager,
) ([]Rename, error) {
	oldNumToNew := buildOldNumToNew(specRenames)
	if len(oldNumToNew) == 0 {
		return nil, nil
	}

	mdFiles, err := collectMDFiles(promptDirs)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "collect md files")
	}

	var renames []Rename
	for _, entry := range mdFiles {
		rename, err := processPromptFileForSpecRefs(
			ctx,
			entry.dir,
			entry.name,
			oldNumToNew,
			mover,
			pm,
		)
		if err != nil {
			return nil, err
		}
		if rename != nil {
			renames = append(renames, *rename)
		}
	}
	return renames, nil
}

func buildOldNumToNew(specRenames []Rename) map[int]int {
	oldNumToNew := make(map[int]int)
	for _, rename := range specRenames {
		oldNum := specnum.Parse(strings.TrimSuffix(filepath.Base(rename.OldPath), ".md"))
		newNum := specnum.Parse(strings.TrimSuffix(filepath.Base(rename.NewPath), ".md"))
		if oldNum >= 0 && newNum >= 0 {
			oldNumToNew[oldNum] = newNum
		}
	}
	return oldNumToNew
}

type mdFileEntry struct {
	dir  string
	name string
}

func collectMDFiles(dirs []string) ([]mdFileEntry, error) {
	var entries []mdFileEntry
	for _, dir := range dirs {
		dirEntries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, de := range dirEntries {
			if de.IsDir() || !hasmdSuffix(de.Name()) {
				continue
			}
			entries = append(entries, mdFileEntry{dir: dir, name: de.Name()})
		}
	}
	return entries, nil
}

func processPromptFileForSpecRefs(
	ctx context.Context,
	dir string,
	name string,
	oldNumToNew map[int]int,
	mover FileMover,
	pm PromptManager,
) (*Rename, error) {
	path := filepath.Join(dir, name)

	if err := updateFrontmatterSpecRefs(ctx, path, oldNumToNew, pm); err != nil {
		return nil, err
	}

	return renameFileWithSpecPattern(ctx, dir, name, oldNumToNew, mover)
}

func updateFrontmatterSpecRefs(
	ctx context.Context,
	path string,
	oldNumToNew map[int]int,
	pm PromptManager,
) error {
	pf, err := pm.Load(ctx, path)
	if err != nil {
		slog.Warn(
			"reindex: failed to load prompt for spec ref update",
			"file",
			filepath.Base(path),
			"error",
			err,
		)
		return nil
	}
	if pf == nil {
		return nil
	}

	changed := false
	for i, s := range pf.Frontmatter.Specs {
		n := specnum.Parse(s)
		if n < 0 {
			continue
		}
		newNum, ok := oldNumToNew[n]
		if !ok {
			continue
		}
		slog.Info(
			"reindex: updated spec ref in prompt",
			"file",
			filepath.Base(path),
			"old",
			n,
			"new",
			newNum,
		)
		pf.Frontmatter.Specs[i] = fmt.Sprintf("%03d", newNum)
		changed = true
	}

	if !changed {
		return nil
	}
	return errors.Wrapf(ctx, pf.Save(ctx), "save spec refs in %s", filepath.Base(path))
}

func renameFileWithSpecPattern(
	ctx context.Context,
	dir string,
	name string,
	oldNumToNew map[int]int,
	mover FileMover,
) (*Rename, error) {
	match := specFilenamePatternRegexp.FindStringSubmatch(name)
	if match == nil {
		return nil, nil
	}

	specNum, _ := strconv.Atoi(match[1])
	if _, ok := oldNumToNew[specNum]; !ok {
		return nil, nil
	}

	newName := replaceSpecNumbers(name, oldNumToNew)
	if newName == name {
		return nil, nil
	}

	oldPath := filepath.Join(dir, name)
	newPath := filepath.Join(dir, newName)
	if err := mover.MoveFile(ctx, oldPath, newPath); err != nil {
		return nil, errors.Wrapf(ctx, err, "rename prompt file %s → %s", name, newName)
	}
	slog.Info("reindex: renamed prompt file with spec ref", "from", name, "to", newName)
	return &Rename{OldPath: oldPath, NewPath: newPath}, nil
}

func replaceSpecNumbers(name string, oldNumToNew map[int]int) string {
	return specFilenamePatternRegexp.ReplaceAllStringFunc(name, func(s string) string {
		inner := specFilenamePatternRegexp.FindStringSubmatch(s)
		n, _ := strconv.Atoi(inner[1])
		if nn, exists := oldNumToNew[n]; exists {
			return fmt.Sprintf("spec-%03d", nn)
		}
		return s
	})
}
