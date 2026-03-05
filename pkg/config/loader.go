// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"context"
	"log/slog"
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

// partialConfig is used for YAML unmarshaling to distinguish between
// explicitly set zero values and missing fields.
type partialConfig struct {
	Workflow       *Workflow     `yaml:"workflow"`
	InboxDir       *string       `yaml:"inboxDir"`
	QueueDir       *string       `yaml:"queueDir"`
	CompletedDir   *string       `yaml:"completedDir"`
	LogDir         *string       `yaml:"logDir"`
	ContainerImage *string       `yaml:"containerImage"`
	DebounceMs     *int          `yaml:"debounceMs"`
	ServerPort     *int          `yaml:"serverPort"`
	AutoMerge      *bool         `yaml:"autoMerge"`
	AutoRelease    *bool         `yaml:"autoRelease"`
	GitHub         *GitHubConfig `yaml:"github"`
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

	// Check file permissions
	fileInfo, statErr := os.Stat(l.configPath)
	if statErr != nil {
		slog.Warn("failed to stat config file", "path", l.configPath, "error", statErr)
	} else if fileInfo.Mode()&0004 != 0 {
		slog.Warn("config file is world-readable, consider: chmod 600", "path", l.configPath)
	}

	// Parse YAML into partial config to preserve defaults for missing fields
	var partial partialConfig
	if err := yaml.Unmarshal(data, &partial); err != nil {
		return Config{}, errors.Wrap(ctx, err, "parse config file")
	}

	// Merge non-nil values onto defaults
	if partial.Workflow != nil {
		cfg.Workflow = *partial.Workflow
	}
	if partial.InboxDir != nil {
		cfg.InboxDir = *partial.InboxDir
	}
	if partial.QueueDir != nil {
		cfg.QueueDir = *partial.QueueDir
	}
	if partial.CompletedDir != nil {
		cfg.CompletedDir = *partial.CompletedDir
	}
	if partial.LogDir != nil {
		cfg.LogDir = *partial.LogDir
	}
	if partial.ContainerImage != nil {
		cfg.ContainerImage = *partial.ContainerImage
	}
	if partial.DebounceMs != nil {
		cfg.DebounceMs = *partial.DebounceMs
	}
	if partial.ServerPort != nil {
		cfg.ServerPort = *partial.ServerPort
	}
	if partial.AutoMerge != nil {
		cfg.AutoMerge = *partial.AutoMerge
	}
	if partial.AutoRelease != nil {
		cfg.AutoRelease = *partial.AutoRelease
	}
	if partial.GitHub != nil {
		cfg.GitHub = *partial.GitHub
	}

	// Validate merged config
	if err := cfg.Validate(ctx); err != nil {
		return Config{}, errors.Wrap(ctx, err, "validate config")
	}

	return cfg, nil
}
