// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package globalconfig

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	"github.com/bborbe/errors"
	"gopkg.in/yaml.v3"
)

// DefaultMaxContainers is the system-wide container limit when no config is set.
const DefaultMaxContainers = 3

// userHomeDir is a variable so tests can override the home directory lookup.
var userHomeDir = os.UserHomeDir

// GlobalConfig holds the user-level dark-factory configuration.
// It is loaded from ~/.dark-factory/config.yaml once at daemon startup.
// When the file does not exist or the field is omitted, defaults apply.
type GlobalConfig struct {
	MaxContainers int `yaml:"maxContainers"`
}

// Validate validates the GlobalConfig fields.
func (g GlobalConfig) Validate(ctx context.Context) error {
	if g.MaxContainers < 1 {
		return errors.Errorf(
			ctx,
			"globalconfig: maxContainers must be >= 1, got %d",
			g.MaxContainers,
		)
	}
	return nil
}

// defaults returns a GlobalConfig with all default values.
func defaults() GlobalConfig {
	return GlobalConfig{
		MaxContainers: DefaultMaxContainers,
	}
}

//counterfeiter:generate -o ../../mocks/global-config-loader.go --fake-name GlobalConfigLoader . Loader

// Loader loads the global dark-factory configuration.
type Loader interface {
	Load(ctx context.Context) (GlobalConfig, error)
}

// NewLoader creates a Loader that reads from ~/.dark-factory/config.yaml.
func NewLoader() Loader {
	return &fileLoader{}
}

// fileLoader implements Loader by reading ~/.dark-factory/config.yaml.
type fileLoader struct{}

// Load reads ~/.dark-factory/config.yaml, merges with defaults, validates, and returns the config.
// If the file does not exist or is empty, defaults are returned without error.
func (l *fileLoader) Load(ctx context.Context) (GlobalConfig, error) {
	cfg := defaults()

	home, err := userHomeDir()
	if err != nil {
		return GlobalConfig{}, errors.Wrap(ctx, err, "globalconfig: get home directory")
	}

	configPath := filepath.Join(home, ".dark-factory", "config.yaml")

	// #nosec G304 -- configPath is derived from user home dir, not user input
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return GlobalConfig{}, errors.Wrap(ctx, err, "globalconfig: read config file")
	}

	// Empty file → return defaults
	if len(bytes.TrimSpace(data)) == 0 {
		return cfg, nil
	}

	// partial struct to detect which fields were set (vs omitted)
	var partial struct {
		MaxContainers *int `yaml:"maxContainers"`
	}
	if err := yaml.Unmarshal(data, &partial); err != nil {
		return GlobalConfig{}, errors.Wrap(ctx, err, "globalconfig: parse config file")
	}

	if partial.MaxContainers != nil {
		cfg.MaxContainers = *partial.MaxContainers
	}

	if err := cfg.Validate(ctx); err != nil {
		return GlobalConfig{}, errors.Wrap(ctx, err, "globalconfig: validate")
	}

	return cfg, nil
}
