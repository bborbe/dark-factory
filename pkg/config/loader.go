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

// partialPromptsConfig is used for YAML unmarshaling of the prompts section.
type partialPromptsConfig struct {
	InboxDir      *string `yaml:"inboxDir"`
	InProgressDir *string `yaml:"inProgressDir"`
	CompletedDir  *string `yaml:"completedDir"`
	LogDir        *string `yaml:"logDir"`
}

// partialSpecsConfig is used for YAML unmarshaling of the specs section.
type partialSpecsConfig struct {
	InboxDir      *string `yaml:"inboxDir"`
	InProgressDir *string `yaml:"inProgressDir"`
	CompletedDir  *string `yaml:"completedDir"`
	LogDir        *string `yaml:"logDir"`
}

// partialConfig is used for YAML unmarshaling to distinguish between
// explicitly set zero values and missing fields.
type partialConfig struct {
	Workflow         *Workflow             `yaml:"workflow"`
	Prompts          *partialPromptsConfig `yaml:"prompts"`
	Specs            *partialSpecsConfig   `yaml:"specs"`
	ContainerImage   *string               `yaml:"containerImage"`
	DebounceMs       *int                  `yaml:"debounceMs"`
	ServerPort       *int                  `yaml:"serverPort"`
	AutoMerge        *bool                 `yaml:"autoMerge"`
	AutoRelease      *bool                 `yaml:"autoRelease"`
	VerificationGate *bool                 `yaml:"verificationGate"`
	GitHub           *GitHubConfig         `yaml:"github"`
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
	mergePartial(&cfg, &partial)

	// Validate merged config
	if err := cfg.Validate(ctx); err != nil {
		return Config{}, errors.Wrap(ctx, err, "validate config")
	}

	return cfg, nil
}

// mergePartial applies non-nil fields from partial onto cfg.
func mergePartial(cfg *Config, partial *partialConfig) {
	if partial.Workflow != nil {
		cfg.Workflow = *partial.Workflow
	}
	mergePartialPrompts(&cfg.Prompts, partial.Prompts)
	mergePartialSpecs(&cfg.Specs, partial.Specs)
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
	if partial.VerificationGate != nil {
		cfg.VerificationGate = *partial.VerificationGate
	}
	if partial.GitHub != nil {
		cfg.GitHub = *partial.GitHub
	}
}

// mergePartialPrompts applies non-nil fields from src onto dst.
func mergePartialPrompts(dst *PromptsConfig, src *partialPromptsConfig) {
	if src == nil {
		return
	}
	if src.InboxDir != nil {
		dst.InboxDir = *src.InboxDir
	}
	if src.InProgressDir != nil {
		dst.InProgressDir = *src.InProgressDir
	}
	if src.CompletedDir != nil {
		dst.CompletedDir = *src.CompletedDir
	}
	if src.LogDir != nil {
		dst.LogDir = *src.LogDir
	}
}

// mergePartialSpecs applies non-nil fields from src onto dst.
func mergePartialSpecs(dst *SpecsConfig, src *partialSpecsConfig) {
	if src == nil {
		return
	}
	if src.InboxDir != nil {
		dst.InboxDir = *src.InboxDir
	}
	if src.InProgressDir != nil {
		dst.InProgressDir = *src.InProgressDir
	}
	if src.CompletedDir != nil {
		dst.CompletedDir = *src.CompletedDir
	}
	if src.LogDir != nil {
		dst.LogDir = *src.LogDir
	}
}
