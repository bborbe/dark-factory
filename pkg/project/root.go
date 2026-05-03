// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package project

import (
	"context"
	"os"
	"path/filepath"

	"github.com/bborbe/errors"
)

const darkFactoryYAML = ".dark-factory.yaml"

// FindRoot walks up the directory tree from the current working directory,
// looking for a .dark-factory.yaml file. It stops at $HOME and never
// ascends above it. Returns the directory that contains .dark-factory.yaml.
//
// If no .dark-factory.yaml is found in any ancestor up to $HOME, it returns:
//
//	"not a dark-factory project: no .dark-factory.yaml in <cwd> or any parent directory"
func FindRoot(ctx context.Context) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Wrap(ctx, err, "get home directory")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", errors.Wrap(ctx, err, "get working directory")
	}

	dir := cwd
	for {
		yamlPath := filepath.Join(dir, darkFactoryYAML)
		if _, err := os.Stat(yamlPath); err == nil {
			return dir, nil
		}

		// Stop at $HOME — do not ascend above it.
		if dir == home {
			break
		}

		parent := filepath.Dir(dir)
		// Stop at filesystem root (guard against infinite loop on non-standard FS layouts).
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", errors.Errorf(
		ctx,
		"not a dark-factory project: no %s in %s or any parent directory",
		darkFactoryYAML,
		cwd,
	)
}
