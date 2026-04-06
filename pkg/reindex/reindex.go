// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reindex

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/adrg/frontmatter"
	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"
	"gopkg.in/yaml.v3"

	"github.com/bborbe/dark-factory/pkg/specnum"
)

var validPatternRegexp = regexp.MustCompile(`^(\d{3})-(.+)\.md$`)

// Rename represents a file rename performed by the reindexer.
type Rename struct {
	OldPath string
	NewPath string
}

//counterfeiter:generate -o ../../mocks/reindex-file-mover.go --fake-name ReindexFileMover . FileMover

// FileMover handles file move operations with git awareness.
type FileMover interface {
	MoveFile(ctx context.Context, oldPath string, newPath string) error
}

//counterfeiter:generate -o ../../mocks/reindexer.go --fake-name Reindexer . Reindexer

// Reindexer detects and resolves duplicate numeric prefixes across multiple directories.
type Reindexer interface {
	Reindex(ctx context.Context) ([]Rename, error)
}

// NewReindexer creates a Reindexer that scans the given dirs for NNN-slug.md files
// with duplicate numeric prefixes and renames conflicting files.
func NewReindexer(dirs []string, mover FileMover) Reindexer {
	return &reindexer{dirs: dirs, mover: mover}
}

type reindexer struct {
	dirs  []string
	mover FileMover
}

type fileEntry struct {
	dir     string
	name    string
	number  int
	slug    string
	created string
}

func (r *reindexer) Reindex(ctx context.Context) ([]Rename, error) {
	entries, err := collectEntries(r.dirs)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "collect entries")
	}

	usedNumbers := buildUsedNumbers(entries)
	groups := groupByNumber(entries)

	var renames []Rename
	for _, group := range groups {
		if len(group) <= 1 {
			continue
		}
		groupRenames, err := r.resolveGroup(ctx, group, usedNumbers)
		if err != nil {
			return nil, err
		}
		renames = append(renames, groupRenames...)
	}
	return renames, nil
}

func (r *reindexer) resolveGroup(
	ctx context.Context,
	group []fileEntry,
	usedNumbers map[int]bool,
) ([]Rename, error) {
	for i := range group {
		group[i].created = readCreated(filepath.Join(group[i].dir, group[i].name))
	}
	sortGroup(group)

	var renames []Rename
	for _, loser := range group[1:] {
		rename, err := r.renameLoser(ctx, loser, usedNumbers)
		if err != nil {
			return nil, err
		}
		renames = append(renames, rename)
	}
	return renames, nil
}

func (r *reindexer) renameLoser(
	ctx context.Context,
	loser fileEntry,
	usedNumbers map[int]bool,
) (Rename, error) {
	newNum := findNextAvailableNumber(usedNumbers)
	usedNumbers[newNum] = true

	newName := fmt.Sprintf("%03d-%s.md", newNum, loser.slug)
	oldPath := filepath.Join(loser.dir, loser.name)
	newPath := filepath.Join(loser.dir, newName)

	if err := r.mover.MoveFile(ctx, oldPath, newPath); err != nil {
		return Rename{}, errors.Wrapf(ctx, err, "reindex rename %s → %s", loser.name, newName)
	}

	slog.Info("reindex: renamed file", "from", loser.name, "to", newName)
	return Rename{OldPath: oldPath, NewPath: newPath}, nil
}

func buildUsedNumbers(entries []fileEntry) map[int]bool {
	usedNumbers := make(map[int]bool)
	for _, e := range entries {
		if validPatternRegexp.MatchString(e.name) {
			usedNumbers[e.number] = true
		}
	}
	return usedNumbers
}

func groupByNumber(entries []fileEntry) map[int][]fileEntry {
	groups := make(map[int][]fileEntry)
	for _, e := range entries {
		if validPatternRegexp.MatchString(e.name) {
			groups[e.number] = append(groups[e.number], e)
		}
	}
	return groups
}

func sortGroup(group []fileEntry) {
	sort.SliceStable(group, func(i, j int) bool {
		return entriesLess(group[i], group[j])
	})
}

func entriesLess(a, b fileEntry) bool {
	ta, oka := parseCreated(a.created)
	tb, okb := parseCreated(b.created)
	if oka && okb {
		if time.Time(ta).Equal(time.Time(tb)) {
			return a.name < b.name
		}
		return time.Time(ta).Before(time.Time(tb))
	}
	if oka {
		return true
	}
	if okb {
		return false
	}
	return a.name < b.name
}

func collectEntries(dirs []string) ([]fileEntry, error) {
	var entries []fileEntry
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
			name := de.Name()
			stem := name[:len(name)-3] // trim ".md"
			number := specnum.Parse(stem)

			var slug string
			if matches := validPatternRegexp.FindStringSubmatch(name); matches != nil {
				slug = matches[2]
			} else {
				slug = stem
			}

			entries = append(entries, fileEntry{
				dir:    dir,
				name:   name,
				number: number,
				slug:   slug,
			})
		}
	}
	return entries, nil
}

func hasmdSuffix(name string) bool {
	return len(name) > 3 && name[len(name)-3:] == ".md"
}

func findNextAvailableNumber(usedNumbers map[int]bool) int {
	for i := 1; ; i++ {
		if !usedNumbers[i] {
			return i
		}
	}
}

func readCreated(path string) string {
	data, err := os.ReadFile(
		path,
	) // #nosec G304 -- path is constructed from dirs passed by caller, not user input
	if err != nil {
		return ""
	}
	var fm struct {
		Created string `yaml:"created"`
	}
	yamlV3Format := frontmatter.NewFormat("---", "---", yaml.Unmarshal)
	_, _ = frontmatter.Parse(bytes.NewReader(data), &fm, yamlV3Format)
	return fm.Created
}

func parseCreated(s string) (libtime.DateTime, bool) {
	if s == "" {
		return libtime.DateTime{}, false
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return libtime.DateTime(t), true
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return libtime.DateTime(t), true
	}
	return libtime.DateTime{}, false
}
