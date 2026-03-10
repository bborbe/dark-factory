// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"context"
	"log/slog"
	"os"
	"regexp"
	"strings"

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
	ProjectName       string            `yaml:"projectName"`
	Workflow          Workflow          `yaml:"workflow"`
	DefaultBranch     string            `yaml:"defaultBranch"`
	Prompts           PromptsConfig     `yaml:"prompts"`
	Specs             SpecsConfig       `yaml:"specs"`
	ContainerImage    string            `yaml:"containerImage"`
	NetrcFile         string            `yaml:"netrcFile"`
	GitconfigFile     string            `yaml:"gitconfigFile"`
	Model             string            `yaml:"model"`
	ValidationCommand string            `yaml:"validationCommand"`
	DebounceMs        int               `yaml:"debounceMs"`
	ServerPort        int               `yaml:"serverPort"`
	AutoMerge         bool              `yaml:"autoMerge"`
	AutoRelease       bool              `yaml:"autoRelease"`
	VerificationGate  bool              `yaml:"verificationGate"`
	AutoReview        bool              `yaml:"autoReview"`
	MaxReviewRetries  int               `yaml:"maxReviewRetries"`
	AllowedReviewers  []string          `yaml:"allowedReviewers,omitempty"`
	UseCollaborators  bool              `yaml:"useCollaborators"`
	PollIntervalSec   int               `yaml:"pollIntervalSec"`
	GitHub            GitHubConfig      `yaml:"github"`
	Env               map[string]string `yaml:"env,omitempty"`
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
		ContainerImage:    "docker.io/bborbe/claude-yolo:v0.2.8",
		Model:             "claude-sonnet-4-6",
		ValidationCommand: "make precommit",
		DebounceMs:        500,
		ServerPort:        0,
		AutoMerge:         false,
		AutoRelease:       false,
		AutoReview:        false,
		MaxReviewRetries:  3,
		PollIntervalSec:   60,
		UseCollaborators:  false,
		GitHub:            GitHubConfig{},
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
			if c.AutoMerge && c.Workflow != WorkflowPR {
				return errors.Errorf(ctx, "autoMerge requires workflow 'pr'")
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
		validation.Name("netrcFile", validation.HasValidationFunc(c.validateNetrcFile)),
		validation.Name("gitconfigFile", validation.HasValidationFunc(c.validateGitconfigFile)),
		validation.Name("env", validation.HasValidationFunc(c.validateEnv)),
	}.Validate(ctx)
}

// validateAutoReview validates the autoReview configuration.
func (c Config) validateAutoReview(ctx context.Context) error {
	if !c.AutoReview {
		return nil
	}
	if c.Workflow != WorkflowPR {
		return errors.Errorf(ctx, "autoReview requires workflow 'pr'")
	}
	if !c.AutoMerge {
		return errors.Errorf(ctx, "autoReview requires autoMerge")
	}
	if len(c.AllowedReviewers) == 0 && !c.UseCollaborators {
		return errors.Errorf(ctx, "autoReview requires allowedReviewers or useCollaborators: true")
	}
	return nil
}

// validateNetrcFile validates the netrcFile configuration.
func (c Config) validateNetrcFile(ctx context.Context) error {
	if c.NetrcFile == "" {
		return nil
	}
	resolved := resolveFilePath(c.NetrcFile)
	if _, err := os.Stat(resolved); err != nil {
		return errors.Errorf(ctx, "netrcFile %q does not exist: %v", resolved, err)
	}
	return nil
}

// validateGitconfigFile validates the gitconfigFile configuration.
func (c Config) validateGitconfigFile(ctx context.Context) error {
	if c.GitconfigFile == "" {
		return nil
	}
	resolved := resolveFilePath(c.GitconfigFile)
	if _, err := os.Stat(resolved); err != nil {
		return errors.Errorf(ctx, "gitconfigFile %q does not exist: %v", resolved, err)
	}
	return nil
}

// reservedEnvKeys are env var names set internally by the executor and cannot be overridden.
var reservedEnvKeys = []string{"YOLO_PROMPT_FILE", "ANTHROPIC_MODEL"}

// validateEnv validates the env map keys.
func (c Config) validateEnv(ctx context.Context) error {
	for k := range c.Env {
		if k == "" {
			return errors.Errorf(ctx, "env key must not be empty")
		}
		for _, reserved := range reservedEnvKeys {
			if k == reserved {
				return errors.Errorf(ctx, "env key %q is reserved and cannot be overridden", k)
			}
		}
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

// resolveFilePath resolves a file path by expanding ${VAR} env vars and leading ~/ to home dir.
func resolveFilePath(value string) string {
	value = resolveEnvVar(value)
	if strings.HasPrefix(value, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			value = home + value[1:]
		}
	}
	return value
}

// ResolvedGitHubToken returns the GitHub token with environment variables resolved.
// Returns empty string when not configured, letting gh use its own auth.
func (c Config) ResolvedGitHubToken() string {
	if c.GitHub.Token == "" {
		return ""
	}
	token := resolveEnvVar(c.GitHub.Token)
	if token == "" {
		slog.Warn("github.token configured but env var is empty, using default gh auth")
	}
	return token
}
