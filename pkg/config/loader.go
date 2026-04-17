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

//counterfeiter:generate -o ../../mocks/config-loader.go --fake-name Loader . Loader

// Loader loads configuration from a file.
type Loader interface {
	Load(ctx context.Context) (Config, error)
}

// NewLoader creates a Loader that reads from .dark-factory.yaml in the current directory.
func NewLoader() Loader {
	return &fileLoader{
		configPath: ".dark-factory.yaml",
	}
}

// fileLoader implements Loader by reading from a file.
type fileLoader struct {
	configPath string
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
	Workflow               *Workflow             `yaml:"workflow"`
	PR                     *bool                 `yaml:"pr"`
	Worktree               *bool                 `yaml:"worktree"`
	ProjectName            *string               `yaml:"projectName"`
	DefaultBranch          *string               `yaml:"defaultBranch"`
	Prompts                *partialPromptsConfig `yaml:"prompts"`
	Specs                  *partialSpecsConfig   `yaml:"specs"`
	ContainerImage         *string               `yaml:"containerImage"`
	NetrcFile              *string               `yaml:"netrcFile"`
	GitconfigFile          *string               `yaml:"gitconfigFile"`
	Model                  *string               `yaml:"model"`
	ValidationCommand      *string               `yaml:"validationCommand"`
	ValidationPrompt       *string               `yaml:"validationPrompt"`
	TestCommand            *string               `yaml:"testCommand"`
	DebounceMs             *int                  `yaml:"debounceMs"`
	ServerPort             *int                  `yaml:"serverPort"`
	AutoMerge              *bool                 `yaml:"autoMerge"`
	AutoRelease            *bool                 `yaml:"autoRelease"`
	VerificationGate       *bool                 `yaml:"verificationGate"`
	AutoReview             *bool                 `yaml:"autoReview"`
	MaxReviewRetries       *int                  `yaml:"maxReviewRetries"`
	AllowedReviewers       []string              `yaml:"allowedReviewers,omitempty"`
	UseCollaborators       *bool                 `yaml:"useCollaborators"`
	PollIntervalSec        *int                  `yaml:"pollIntervalSec"`
	GitHub                 *GitHubConfig         `yaml:"github"`
	Provider               *Provider             `yaml:"provider"`
	Bitbucket              *BitbucketConfig      `yaml:"bitbucket"`
	Notifications          *NotificationsConfig  `yaml:"notifications"`
	Env                    map[string]string     `yaml:"env,omitempty"`
	ExtraMounts            []ExtraMount          `yaml:"extraMounts,omitempty"`
	ClaudeDir              *string               `yaml:"claudeDir"`
	GenerateCommand        *string               `yaml:"generateCommand"`
	AdditionalInstructions *string               `yaml:"additionalInstructions,omitempty"`
	MaxContainers          *int                  `yaml:"maxContainers,omitempty"`
	DirtyFileThreshold     *int                  `yaml:"dirtyFileThreshold,omitempty"`
	MaxPromptDuration      *string               `yaml:"maxPromptDuration"`
	AutoRetryLimit         *int                  `yaml:"autoRetryLimit"`
	HideGit                *bool                 `yaml:"hideGit"`
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

	// Step A — workflow: pr legacy enum mapping
	if partial.Workflow != nil && *partial.Workflow == WorkflowPR {
		cfg.Workflow = WorkflowClone
		cfg.PR = true
		slog.Info(
			"'workflow: pr' is deprecated; use 'workflow: clone' with 'pr: true' instead",
			"resolved", "workflow: clone, pr: true",
		)
	} else if partial.Workflow != nil && partial.Worktree != nil {
		// Step B — new workflow value alongside legacy worktree: bool
		slog.Warn(
			"'worktree' is ignored when 'workflow' is set; remove 'worktree' from .dark-factory.yaml",
			"workflow", cfg.Workflow,
		)
	} else if partial.Workflow == nil && partial.Worktree != nil {
		// Step C — legacy worktree: bool mapping (no workflow field)
		switch {
		case !cfg.Worktree && !cfg.PR:
			cfg.Workflow = WorkflowDirect
			cfg.PR = false
			slog.Info(
				"'worktree' is deprecated in .dark-factory.yaml; use 'workflow' instead",
				"resolved_workflow", cfg.Workflow, "resolved_pr", cfg.PR,
			)
		case !cfg.Worktree && cfg.PR:
			cfg.Workflow = WorkflowBranch
			cfg.PR = true
			slog.Info(
				"'worktree' is deprecated in .dark-factory.yaml; use 'workflow' instead",
				"resolved_workflow", cfg.Workflow, "resolved_pr", cfg.PR,
			)
		case cfg.Worktree && cfg.PR:
			cfg.Workflow = WorkflowClone
			cfg.PR = true
			slog.Info(
				"'worktree' is deprecated in .dark-factory.yaml; use 'workflow' instead",
				"resolved_workflow", cfg.Workflow, "resolved_pr", cfg.PR,
			)
		case cfg.Worktree && !cfg.PR:
			cfg.Workflow = WorkflowClone
			cfg.PR = true
			slog.Info(
				"'worktree' is deprecated in .dark-factory.yaml; use 'workflow' instead",
				"resolved_workflow", cfg.Workflow, "resolved_pr", cfg.PR,
			)
			slog.Warn(
				"'worktree: true, pr: false' overrides pr to true for compatibility; set 'pr: true' explicitly to silence this warning",
			)
		}
	}
	// Step D — zero out cfg.Worktree unconditionally
	cfg.Worktree = false

	// Validate merged config
	if err := cfg.Validate(ctx); err != nil {
		return Config{}, errors.Wrap(ctx, err, "validate config")
	}

	return cfg, nil
}

// mergePartial applies non-nil fields from partial onto cfg.
func mergePartial(cfg *Config, partial *partialConfig) {
	mergePartialWorkflow(cfg, partial)
	mergePartialContainer(cfg, partial)
	mergePartialReview(cfg, partial)
	mergePartialProviders(cfg, partial)
	mergePartialLimits(cfg, partial)
}

// mergePartialWorkflow merges workflow/PR/branch fields.
func mergePartialWorkflow(cfg *Config, partial *partialConfig) {
	if partial.Workflow != nil {
		cfg.Workflow = *partial.Workflow
	}
	if partial.PR != nil {
		cfg.PR = *partial.PR
	}
	if partial.Worktree != nil {
		cfg.Worktree = *partial.Worktree
	}
	if partial.ProjectName != nil {
		cfg.ProjectName = *partial.ProjectName
	}
	if partial.DefaultBranch != nil {
		cfg.DefaultBranch = *partial.DefaultBranch
	}
	mergePartialPrompts(&cfg.Prompts, partial.Prompts)
	mergePartialSpecs(&cfg.Specs, partial.Specs)
}

// mergePartialContainer merges container image, files, commands, and runtime settings.
func mergePartialContainer(cfg *Config, partial *partialConfig) {
	if partial.ContainerImage != nil {
		cfg.ContainerImage = *partial.ContainerImage
	}
	if partial.NetrcFile != nil {
		cfg.NetrcFile = *partial.NetrcFile
	}
	if partial.GitconfigFile != nil {
		cfg.GitconfigFile = *partial.GitconfigFile
	}
	if partial.Model != nil {
		cfg.Model = *partial.Model
	}
	if partial.ValidationCommand != nil {
		cfg.ValidationCommand = *partial.ValidationCommand
	}
	if partial.ValidationPrompt != nil {
		cfg.ValidationPrompt = *partial.ValidationPrompt
	}
	if partial.TestCommand != nil {
		cfg.TestCommand = *partial.TestCommand
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
	if partial.ClaudeDir != nil {
		cfg.ClaudeDir = *partial.ClaudeDir
	}
	if partial.GenerateCommand != nil {
		cfg.GenerateCommand = *partial.GenerateCommand
	}
	if partial.AdditionalInstructions != nil {
		cfg.AdditionalInstructions = *partial.AdditionalInstructions
	}
	if partial.Env != nil {
		cfg.Env = partial.Env
	}
	if partial.ExtraMounts != nil {
		cfg.ExtraMounts = partial.ExtraMounts
	}
	if partial.HideGit != nil {
		cfg.HideGit = *partial.HideGit
	}
}

// mergePartialReview merges auto-review and reviewer settings.
func mergePartialReview(cfg *Config, partial *partialConfig) {
	if partial.AutoReview != nil {
		cfg.AutoReview = *partial.AutoReview
	}
	if partial.MaxReviewRetries != nil {
		cfg.MaxReviewRetries = *partial.MaxReviewRetries
	}
	if partial.AllowedReviewers != nil {
		cfg.AllowedReviewers = partial.AllowedReviewers
	}
	if partial.UseCollaborators != nil {
		cfg.UseCollaborators = *partial.UseCollaborators
	}
	if partial.PollIntervalSec != nil {
		cfg.PollIntervalSec = *partial.PollIntervalSec
	}
}

// mergePartialProviders merges provider and notification settings.
func mergePartialProviders(cfg *Config, partial *partialConfig) {
	if partial.GitHub != nil {
		cfg.GitHub = *partial.GitHub
	}
	if partial.Provider != nil {
		cfg.Provider = *partial.Provider
	}
	if partial.Bitbucket != nil {
		cfg.Bitbucket = *partial.Bitbucket
	}
	if partial.Notifications != nil {
		cfg.Notifications = *partial.Notifications
	}
}

// mergePartialLimits merges resource limit and duration settings.
func mergePartialLimits(cfg *Config, partial *partialConfig) {
	if partial.MaxContainers != nil {
		cfg.MaxContainers = *partial.MaxContainers
	}
	if partial.DirtyFileThreshold != nil {
		cfg.DirtyFileThreshold = *partial.DirtyFileThreshold
	}
	if partial.MaxPromptDuration != nil {
		cfg.MaxPromptDuration = *partial.MaxPromptDuration
	}
	if partial.AutoRetryLimit != nil {
		cfg.AutoRetryLimit = *partial.AutoRetryLimit
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
