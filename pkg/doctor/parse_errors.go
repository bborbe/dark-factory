// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package doctor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/frontmatter"
	"github.com/bborbe/errors"
	"gopkg.in/yaml.v3"
)

// scanDirsForSpecs reads all .md files from spec dirs and returns their paths.
func scanDirsForSpecs(ctx context.Context, specDirs []string) ([]string, error) {
	var paths []string
	for _, dir := range specDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, errors.Wrap(ctx, err, "read spec directory: "+dir)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	return paths, nil
}

// scanDirsForPrompts reads all .md files from prompt dirs and returns their paths.
func scanDirsForPrompts(ctx context.Context, promptDirs []string) ([]string, error) {
	var paths []string
	for _, dir := range promptDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, errors.Wrap(ctx, err, "read prompt directory: "+dir)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	return paths, nil
}

func (c *checker) scanParseErrors(ctx context.Context) ([]Finding, error) {
	specDirs := []string{
		c.deps.SpecsInboxDir,
		c.deps.SpecsInProgressDir,
		c.deps.SpecsCompletedDir,
		c.deps.SpecsRejectedDir,
	}
	promptDirs := []string{
		c.deps.PromptsInboxDir,
		c.deps.PromptsInProgressDir,
		c.deps.PromptsCompletedDir,
		c.deps.PromptsCancelledDir,
	}

	var findings []Finding

	for _, dir := range append(specDirs, promptDirs...) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, errors.Wrap(ctx, err, "read directory: "+dir)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if err := checkFileParseError(path); err != nil {
				findings = append(findings, Finding{
					Category:    CategoryParseError,
					TargetPaths: []string{path},
					SpecID:      "",
					Detail:      "YAML parse error: " + err.Error(),
					FixCommand:  "Fix the YAML by hand, then re-run `dark-factory doctor`",
				})
			}
		}
	}

	return findings, nil
}

func checkFileParseError(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	yamlV3Format := frontmatter.NewFormat("---", "---", yaml.Unmarshal)
	var fm struct{}
	_, err = frontmatter.Parse(bytes.NewReader(content), &fm, yamlV3Format)
	return err
}
