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
//
// Square brackets are allowed for provider-side variant suffixes
// (e.g. `claude-sonnet-4-5[1m]` for the 1M context window variant,
// `deepseek-v4-flash[1m]`). They are NOT shell metacharacters in the
// docker-run context — model flows as a single argv element via
// `exec.Command(name, args...)`, never through `sh -c`, so the shell
// glob meaning never applies.
const ModelPattern = `^[a-zA-Z0-9._:/[\]-]{1,256}$`

// ModelRegex validates model identifiers at every config layer (global, project, CLI arg).
// Permits Anthropic IDs (claude-opus-4-7), variant suffixes (claude-sonnet-4-5[1m]),
// other-provider IDs (qwen3.6:35b-a3b, deepseek-v4-flash[1m]), namespaced paths
// (local/qwen3.6:35b-a3b), and Docker image refs (docker.io/bborbe/claude-yolo:v0.6.1).
// Blocks true shell metacharacters (`;`, `&`, `|`, `$`, backticks, quotes, etc.).
// EXPORTED so pkg/config and main.go reuse the SAME compiled regex — do not duplicate the pattern.
var ModelRegex = regexp.MustCompile(ModelPattern)

// envKeyPattern is the required format for environment variable key names.
const envKeyPattern = `^[A-Z_][A-Z0-9_]*$`

// envKeyRegexp validates environment variable key names.
var envKeyRegexp = regexp.MustCompile(envKeyPattern)

// userHomeDir is a variable so tests can override the home directory lookup.
var userHomeDir = os.UserHomeDir

// GlobalConfig holds the user-level dark-factory configuration.
// It is loaded from ~/.config/dark-factory/config.yaml (XDG) with fallback to
// ~/.dark-factory/config.yaml (legacy) once at daemon startup.
// When the file does not exist or the field is omitted, defaults apply.
type GlobalConfig struct {
	MaxContainers       int               `yaml:"maxContainers"`
	HideGit             *bool             `yaml:"hideGit,omitempty"`
	AutoRelease         *bool             `yaml:"autoRelease,omitempty"`
	DirtyFileThreshold  *int              `yaml:"dirtyFileThreshold,omitempty"`
	Model               *string           `yaml:"model,omitempty"`
	AutoApprovePrompts  *bool             `yaml:"autoApprovePrompts,omitempty"`
	AutoGeneratePrompts *bool             `yaml:"autoGeneratePrompts,omitempty"`
	Backend             *string           `yaml:"backend,omitempty"`
	Env                 map[string]string `yaml:"env,omitempty"`
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
	if g.Backend != nil {
		if *g.Backend != "docker" && *g.Backend != "local" {
			return errors.Errorf(
				ctx,
				"globalconfig: backend %q invalid, valid values: docker, local",
				*g.Backend,
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

// NewLoader creates a Loader that reads from the XDG-first global config path
// (see FindConfigDir for the exact semantics).
func NewLoader() Loader {
	return &fileLoader{}
}

// fileLoader implements Loader by reading the XDG-first global config path.
// See FindConfigDir for the path resolution logic.
type fileLoader struct{}

// FindConfigDir returns the absolute path to the configuration directory for the given
// tool name, following the XDG Base Directory Specification with legacy fallback.
//
// Resolution order:
//   - If ~/.config/<tool>/ exists (stat success) → returns ~/.config/<tool/>
//   - If ~/.<tool>/ exists (stat success)  → returns ~/.<tool>/ (legacy path)
//   - Neither exists                        → returns ~/.config/<tool>/ (XDG default; not created)
//
// Returns an error only if the home directory lookup fails.
// Stat failures on missing directories are swallowed (they trigger fallthrough, not error).
func FindConfigDir(ctx context.Context, toolName string) (string, error) {
	home, err := userHomeDir()
	if err != nil {
		return "", errors.Wrap(ctx, err, "globalconfig: get home directory")
	}

	xdgPath := filepath.Join(home, ".config", toolName)
	if _, err := os.Stat(xdgPath); err == nil {
		return xdgPath, nil
	}

	legacyPath := filepath.Join(home, "."+toolName)
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath, nil
	}

	// Neither exists — return XDG default path without creating it.
	return xdgPath, nil
}

// FileExists reports whether the global config file exists.
// Callers use this only to distinguish "global file present" from "using built-in defaults"
// in diagnostic logs.
// - Config file missing → (false, nil)
// - Config dir lookup fails → (false, wrapped error)
// - Any other stat error → (false, wrapped error)
// - File present (any size) → (true, nil)
func FileExists(ctx context.Context) (bool, error) {
	dir, err := FindConfigDir(ctx, "dark-factory")
	if err != nil {
		return false, errors.Wrap(ctx, err, "globalconfig: find config directory")
	}
	configPath := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.Wrap(ctx, err, "globalconfig: stat config file")
	}
	return true, nil
}

// Load reads the global config file, merges with defaults, validates, and returns the config.
// The path is resolved via FindConfigDir (XDG-first with legacy fallback).
// If the file does not exist or is empty, defaults are returned without error.
func (l *fileLoader) Load(ctx context.Context) (GlobalConfig, error) {
	cfg := defaults()

	dir, err := FindConfigDir(ctx, "dark-factory")
	if err != nil {
		return GlobalConfig{}, errors.Wrap(ctx, err, "globalconfig: find config directory")
	}

	configPath := filepath.Join(dir, "config.yaml")

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
		MaxContainers       *int              `yaml:"maxContainers"`
		HideGit             *bool             `yaml:"hideGit"`
		AutoRelease         *bool             `yaml:"autoRelease"`
		DirtyFileThreshold  *int              `yaml:"dirtyFileThreshold"`
		Model               *string           `yaml:"model"`
		AutoApprovePrompts  *bool             `yaml:"autoApprovePrompts"`
		AutoGeneratePrompts *bool             `yaml:"autoGeneratePrompts"`
		Backend             *string           `yaml:"backend"`
		Env                 map[string]string `yaml:"env,omitempty"`
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
	if partial.AutoGeneratePrompts != nil {
		cfg.AutoGeneratePrompts = partial.AutoGeneratePrompts
	}
	if partial.Backend != nil {
		cfg.Backend = partial.Backend
	}
	if partial.Env != nil {
		cfg.Env = partial.Env
	}

	if err := cfg.Validate(ctx); err != nil {
		return GlobalConfig{}, errors.Wrap(ctx, err, "globalconfig: validate")
	}

	return cfg, nil
}
