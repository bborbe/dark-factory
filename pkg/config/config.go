// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"context"
	"log/slog"
	"os"
	"regexp"

	"github.com/bborbe/errors"
	"github.com/bborbe/validation"
)

// GitHubConfig holds GitHub-specific configuration.
type GitHubConfig struct {
	Token string `yaml:"token"`
}

// PromptsConfig holds directories for the prompt lifecycle.
type PromptsConfig struct {
	InboxDir      string `yaml:"inboxDir"`
	InProgressDir string `yaml:"inProgressDir"`
	CompletedDir  string `yaml:"completedDir"`
	LogDir        string `yaml:"logDir"`
}

// SpecsConfig holds directories for the spec lifecycle.
type SpecsConfig struct {
	InboxDir      string `yaml:"inboxDir"`
	InProgressDir string `yaml:"inProgressDir"`
	CompletedDir  string `yaml:"completedDir"`
	LogDir        string `yaml:"logDir"`
}

// Config holds the dark-factory configuration.
type Config struct {
	ProjectName      string        `yaml:"projectName"`
	Workflow         Workflow      `yaml:"workflow"`
	Prompts          PromptsConfig `yaml:"prompts"`
	Specs            SpecsConfig   `yaml:"specs"`
	ContainerImage   string        `yaml:"containerImage"`
	Model            string        `yaml:"model"`
	DebounceMs       int           `yaml:"debounceMs"`
	ServerPort       int           `yaml:"serverPort"`
	AutoMerge        bool          `yaml:"autoMerge"`
	AutoRelease      bool          `yaml:"autoRelease"`
	AutoReview       bool          `yaml:"autoReview"`
	MaxReviewRetries int           `yaml:"maxReviewRetries"`
	AllowedReviewers []string      `yaml:"allowedReviewers,omitempty"`
	UseCollaborators bool          `yaml:"useCollaborators"`
	PollIntervalSec  int           `yaml:"pollIntervalSec"`
	GitHub           GitHubConfig  `yaml:"github"`
}

// Defaults returns a Config with all default values.
func Defaults() Config {
	return Config{
		Workflow: WorkflowDirect,
		Prompts: PromptsConfig{
			InboxDir:      "prompts",
			InProgressDir: "prompts/in-progress",
			CompletedDir:  "prompts/completed",
			LogDir:        "prompts/log",
		},
		Specs: SpecsConfig{
			InboxDir:      "specs",
			InProgressDir: "specs/in-progress",
			CompletedDir:  "specs/completed",
			LogDir:        "specs/log",
		},
		ContainerImage:   "docker.io/bborbe/claude-yolo:v0.2.5",
		Model:            "claude-sonnet-4-6",
		DebounceMs:       500,
		ServerPort:       0,
		AutoMerge:        false,
		AutoRelease:      false,
		AutoReview:       false,
		MaxReviewRetries: 3,
		PollIntervalSec:  60,
		UseCollaborators: false,
		GitHub: GitHubConfig{
			Token: "${DARK_FACTORY_GITHUB_TOKEN}",
		}, // #nosec G101 -- env var reference, not a credential
	}
}

// Validate validates the config fields.
func (c Config) Validate(ctx context.Context) error {
	return validation.All{
		validation.Name("workflow", c.Workflow),
		validation.Name("inboxDir", validation.NotEmptyString(c.Prompts.InboxDir)),
		validation.Name("inProgressDir", validation.NotEmptyString(c.Prompts.InProgressDir)),
		validation.Name("completedDir", validation.NotEmptyString(c.Prompts.CompletedDir)),
		validation.Name("logDir", validation.NotEmptyString(c.Prompts.LogDir)),
		validation.Name("containerImage", validation.NotEmptyString(c.ContainerImage)),
		validation.Name("model", validation.NotEmptyString(c.Model)),
		validation.Name("debounceMs", validation.HasValidationFunc(func(ctx context.Context) error {
			if c.DebounceMs <= 0 {
				return errors.Errorf(ctx, "debounceMs must be positive, got %d", c.DebounceMs)
			}
			return nil
		})),
		validation.Name("serverPort", validation.HasValidationFunc(func(ctx context.Context) error {
			if c.ServerPort < 0 || c.ServerPort > 65535 {
				return errors.Errorf(
					ctx,
					"serverPort must be 0 (disabled) or 1-65535, got %d",
					c.ServerPort,
				)
			}
			return nil
		})),
		validation.Name(
			"completedDir",
			validation.HasValidationFunc(func(ctx context.Context) error {
				if c.Prompts.CompletedDir == c.Prompts.InProgressDir {
					return errors.Errorf(ctx, "completedDir cannot equal inProgressDir")
				}
				if c.Prompts.CompletedDir == c.Prompts.InboxDir {
					return errors.Errorf(ctx, "completedDir cannot equal inboxDir")
				}
				return nil
			}),
		),
		validation.Name("autoMerge", validation.HasValidationFunc(func(ctx context.Context) error {
			if c.AutoMerge && c.Workflow != WorkflowPR && c.Workflow != WorkflowWorktree {
				return errors.Errorf(ctx, "autoMerge requires workflow 'pr' or 'worktree'")
			}
			return nil
		})),
		validation.Name(
			"autoRelease",
			validation.HasValidationFunc(func(ctx context.Context) error {
				if c.AutoRelease && !c.AutoMerge {
					return errors.Errorf(ctx, "autoRelease requires autoMerge")
				}
				return nil
			}),
		),
		validation.Name("autoReview", validation.HasValidationFunc(c.validateAutoReview)),
	}.Validate(ctx)
}

// validateAutoReview validates the autoReview configuration.
func (c Config) validateAutoReview(ctx context.Context) error {
	if !c.AutoReview {
		return nil
	}
	if c.Workflow != WorkflowPR && c.Workflow != WorkflowWorktree {
		return errors.Errorf(ctx, "autoReview requires workflow 'pr' or 'worktree'")
	}
	if !c.AutoMerge {
		return errors.Errorf(ctx, "autoReview requires autoMerge")
	}
	if len(c.AllowedReviewers) == 0 && !c.UseCollaborators {
		return errors.Errorf(ctx, "autoReview requires allowedReviewers or useCollaborators: true")
	}
	return nil
}

var envVarPattern = regexp.MustCompile(`^\$\{([A-Z_][A-Z0-9_]*)\}$`)

// resolveEnvVar resolves environment variable references in the form ${VAR_NAME}.
// If the value matches the pattern, it returns the environment variable value.
// Otherwise, it returns the value as-is.
func resolveEnvVar(value string) string {
	matches := envVarPattern.FindStringSubmatch(value)
	if len(matches) == 2 {
		return os.Getenv(matches[1])
	}
	return value
}

// DefaultGitHubTokenRef is the default env var reference for the GitHub token.
const DefaultGitHubTokenRef = "${DARK_FACTORY_GITHUB_TOKEN}" // #nosec G101 -- env var reference, not a credential

// ResolvedGitHubToken returns the GitHub token with environment variables resolved.
// Logs a warning if a non-default token is configured but the env var is empty.
func (c Config) ResolvedGitHubToken() string {
	token := resolveEnvVar(c.GitHub.Token)
	if c.GitHub.Token != "" && c.GitHub.Token != DefaultGitHubTokenRef && token == "" {
		slog.Warn("github.token configured but env var is empty, using default gh auth")
	}
	return token
}
