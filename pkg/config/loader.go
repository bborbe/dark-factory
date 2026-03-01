// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"context"
	"os"

	"github.com/bborbe/errors"
	"gopkg.in/yaml.v3"
)

// Loader loads configuration from a file.
//
//counterfeiter:generate -o ../../mocks/config-loader.go --fake-name Loader . Loader
type Loader interface {
	Load(ctx context.Context) (Config, error)
}

// fileLoader implements Loader by reading from a file.
type fileLoader struct {
	configPath string
}

// NewLoader creates a Loader that reads from .dark-factory.yaml in the current directory.
func NewLoader() Loader {
	return &fileLoader{
		configPath: ".dark-factory.yaml",
	}
}

// Load reads the config file, merges with defaults, validates, and returns the config.
func (l *fileLoader) Load(ctx context.Context) (Config, error) {
	// Start with defaults
	cfg := Defaults()

	// Try to read config file
	// #nosec G304 -- configPath is hardcoded, not user input
	data, err := os.ReadFile(l.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist - return defaults
			return cfg, nil
		}
		return Config{}, errors.Wrap(ctx, err, "read config file")
	}

	// Parse YAML and merge with defaults
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, errors.Wrap(ctx, err, "parse config file")
	}

	// Validate merged config
	if err := cfg.Validate(ctx); err != nil {
		return Config{}, errors.Wrap(ctx, err, "validate config")
	}

	return cfg, nil
}
