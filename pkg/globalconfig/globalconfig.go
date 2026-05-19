// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package globalconfig

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"

	"github.com/bborbe/errors"
	"gopkg.in/yaml.v3"
)

// DefaultMaxContainers is the system-wide container limit when no config is set.
const DefaultMaxContainers = 3

// ModelPattern is the regex source string. Exported so callers can include
// it in error messages.
const ModelPattern = `^[a-zA-Z0-9._:/-]{1,256}$`

// ModelRegex validates model identifiers at every config layer (global, project, CLI arg).
// Permits Anthropic IDs (claude-opus-4-7), other-provider IDs (qwen3.6:35b-a3b),
// namespaced paths (local/qwen3.6:35b-a3b), and Docker image refs (docker.io/bborbe/claude-yolo:v0.6.1).
// Blocks shell metacharacters since model flows to container args.
// EXPORTED so pkg/config and main.go reuse the SAME compiled regex — do not duplicate the pattern.
var ModelRegex = regexp.MustCompile(ModelPattern)

// envKeyPattern is the required format for environment variable key names.
const envKeyPattern = `^[A-Z_][A-Z0-9_]*$`

// envKeyRegexp validates environment variable key names.
var envKeyRegexp = regexp.MustCompile(envKeyPattern)

// userHomeDir is a variable so tests can override the home directory lookup.
var userHomeDir = os.UserHomeDir

// GlobalConfig holds the user-level dark-factory configuration.
// It is loaded from ~/.dark-factory/config.yaml once at daemon startup.
// When the file does not exist or the field is omitted, defaults apply.
type GlobalConfig struct {
	MaxContainers      int               `yaml:"maxContainers"`
	HideGit            *bool             `yaml:"hideGit,omitempty"`
	AutoRelease        *bool             `yaml:"autoRelease,omitempty"`
	DirtyFileThreshold *int              `yaml:"dirtyFileThreshold,omitempty"`
	Model              *string           `yaml:"model,omitempty"`
	AutoApprovePrompts *bool             `yaml:"autoApprovePrompts,omitempty"`
	Env                map[string]string `yaml:"env,omitempty"`
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
	if g.DirtyFileThreshold != nil && *g.DirtyFileThreshold < 0 {
		return errors.Errorf(
			ctx,
			"globalconfig: dirtyFileThreshold must not be negative, got %d",
			*g.DirtyFileThreshold,
		)
	}
	if g.Model != nil {
		if *g.Model == "" {
			return errors.Errorf(ctx, "globalconfig: model must not be empty string when set")
		}
		if !ModelRegex.MatchString(*g.Model) {
			return errors.Errorf(
				ctx,
				"globalconfig: model %q does not match required pattern %s",
				*g.Model,
				ModelPattern,
			)
		}
	}
	for k := range g.Env {
		if !envKeyRegexp.MatchString(k) {
			return errors.Errorf(
				ctx,
				"globalconfig: env key %q does not match required pattern %s",
				k,
				envKeyPattern,
			)
		}
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

// FileExists reports whether the global config file (~/.dark-factory/config.yaml) exists.
// Callers use this only to distinguish "global file present" from "using built-in defaults"
// in diagnostic logs.
// - Config file missing → (false, nil)
// - Home dir lookup fails → (false, wrapped error)
// - Any other stat error → (false, wrapped error)
// - File present (any size) → (true, nil)
func FileExists(ctx context.Context) (bool, error) {
	home, err := userHomeDir()
	if err != nil {
		return false, errors.Wrap(ctx, err, "globalconfig: get home directory")
	}
	configPath := filepath.Join(home, ".dark-factory", "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.Wrap(ctx, err, "globalconfig: stat config file")
	}
	return true, nil
}

// Load reads ~/.dark-factory/config.yaml, merges with defaults, validates, and returns the config.
// If the file does not exist or is empty, defaults are returned without error.
func (l *fileLoader) Load(ctx context.Context) (GlobalConfig, error) {
	cfg := defaults()

	home, err := userHomeDir()
	if err != nil {
		return GlobalConfig{}, errors.Wrap(ctx, err, "globalconfig: get home directory")
	}

	configPath := filepath.Join(home, ".dark-factory", "config.yaml")

	// Best-effort permission check — skip silently on stat failure.
	if info, statErr := os.Stat(configPath); statErr == nil {
		if perm := info.Mode().Perm(); perm&0066 != 0 {
			slog.Warn(
				"global config has group or world read/write permissions; consider: chmod 600",
				"path", configPath,
			)
		}
	}

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
		MaxContainers      *int              `yaml:"maxContainers"`
		HideGit            *bool             `yaml:"hideGit"`
		AutoRelease        *bool             `yaml:"autoRelease"`
		DirtyFileThreshold *int              `yaml:"dirtyFileThreshold"`
		Model              *string           `yaml:"model"`
		AutoApprovePrompts *bool             `yaml:"autoApprovePrompts"`
		Env                map[string]string `yaml:"env,omitempty"`
	}
	if err := yaml.Unmarshal(data, &partial); err != nil {
		return GlobalConfig{}, errors.Wrap(ctx, err, "globalconfig: parse config file")
	}

	if partial.MaxContainers != nil {
		cfg.MaxContainers = *partial.MaxContainers
	}
	if partial.HideGit != nil {
		cfg.HideGit = partial.HideGit
	}
	if partial.AutoRelease != nil {
		cfg.AutoRelease = partial.AutoRelease
	}
	if partial.DirtyFileThreshold != nil {
		cfg.DirtyFileThreshold = partial.DirtyFileThreshold
	}
	if partial.Model != nil {
		cfg.Model = partial.Model
	}
	if partial.AutoApprovePrompts != nil {
		cfg.AutoApprovePrompts = partial.AutoApprovePrompts
	}
	if partial.Env != nil {
		cfg.Env = partial.Env
	}

	if err := cfg.Validate(ctx); err != nil {
		return GlobalConfig{}, errors.Wrap(ctx, err, "globalconfig: validate")
	}

	return cfg, nil
}
